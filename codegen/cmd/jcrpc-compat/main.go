// jcrpc-compat prepares and checks local consumers of exact released runtimes.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/relux-works/javacard-rpc/codegen"
	"github.com/relux-works/javacard-rpc/codegen/internal/compat"
)

func main() { os.Exit(run(os.Args[1:])) }
func run(args []string) int {
	f := flag.NewFlagSet("jcrpc-compat", flag.ContinueOnError)
	repo := f.String("repo", "..", "facade checkout")
	manifest := f.String("manifest", "", "manifest (default compatibility/runtime-manifest.json)")
	root := f.String("root", "", "prepared local checkout directory")
	mode := f.String("mode", "check", "check, bootstrap, prepare, jvm, swift, offline, toolchain")
	offline := f.Bool("offline", false, "use prepared dependency caches only")
	if e := f.Parse(args); e != nil {
		return 2
	}
	if *mode == "offline" {
		return offlineRun(f.Args())
	}
	if f.NArg() != 0 {
		return fail(fmt.Errorf("unexpected positional arguments"))
	}
	if *mode == "toolchain" {
		if *root == "" {
			return fail(fmt.Errorf("root required"))
		}
		dir, e := filepath.Abs(*root)
		if e != nil {
			return fail(e)
		}
		if e = toolchain(dir); e != nil {
			return fail(e)
		}
		return 0
	}
	abs, e := filepath.Abs(*repo)
	if e != nil {
		return fail(e)
	}
	*repo = abs
	if *manifest == "" {
		*manifest = filepath.Join(*repo, "compatibility/runtime-manifest.json")
	}
	m, e := compat.Check(*repo, *manifest)
	if e != nil {
		return fail(e)
	}
	if *mode == "check" {
		fmt.Println("manifest and resolved backends agree")
		return 0
	}
	if *root == "" {
		return fail(fmt.Errorf("root required"))
	}
	*root, e = filepath.Abs(*root)
	if e != nil {
		return fail(e)
	}
	runtimes := filepath.Join(*root, "runtimes")
	if *mode == "bootstrap" {
		if e = os.MkdirAll(runtimes, 0755); e != nil {
			return fail(e)
		}
		for _, p := range m.Targets {
			dir := filepath.Join(runtimes, filepath.Base(p.GoModule))
			if _, e = os.Stat(dir); os.IsNotExist(e) {
				if e = command(*repo, "git", "clone", "--branch", p.Tag, "--depth", "1", p.Repository, dir); e != nil {
					return fail(e)
				}
			} else if e != nil {
				return fail(e)
			}
			if e = compat.VerifyCheckout(*repo, dir, p); e != nil {
				return fail(e)
			}
			fmt.Printf("%s: verified signed %s tag-object=%s commit=%s backend=%s native=%s\n", p.Target, p.Tag, p.TagObject, p.Commit, p.BackendVersion, p.Runtime)
		}
		return 0
	}
	for _, p := range m.Targets {
		if e = compat.VerifyCheckout(*repo, filepath.Join(runtimes, filepath.Base(p.GoModule)), p); e != nil {
			return fail(e)
		}
	}
	switch *mode {
	case "prepare":
		e = prepare(*repo, *root)
	case "jvm":
		gradleArgs := []string{"build", "--no-daemon", "--max-workers=2", "--console=plain"}
		if *offline {
			gradleArgs = append(gradleArgs, "--offline")
		}
		for _, dir := range []string{filepath.Join(runtimes, "javacard-rpc-server-javacard"), filepath.Join(runtimes, "javacard-rpc-client-kotlin"), filepath.Join(*root, "jvm")} {
			if e = command(dir, "gradle", gradleArgs...); e != nil {
				break
			}
		}
	case "swift":
		if e = command(filepath.Join(runtimes, "javacard-rpc-client-swift"), "swift", "test", "--disable-sandbox", "--disable-automatic-resolution"); e == nil {
			e = command(filepath.Join(*root, "swift"), "swift", "test", "--disable-sandbox", "--disable-automatic-resolution")
		}
	default:
		e = fmt.Errorf("unknown mode %s", *mode)
	}
	if e != nil {
		return fail(e)
	}
	return 0
}
func fail(e error) int { fmt.Fprintln(os.Stderr, e); return 1 }
func command(dir, name string, args ...string) error {
	fmt.Printf("%s: %s %s\n", dir, name, strings.Join(args, " "))
	c := exec.Command(name, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = append(os.Environ(), "GOWORK=off")
	if e := c.Run(); e != nil {
		return fmt.Errorf("%s: %w", name, e)
	}
	return nil
}
func write(path, content string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	return os.WriteFile(path, []byte(content), 0644)
}
func prepare(repo, root string) error {
	gen := filepath.Join(root, "generated")
	bin := filepath.Join(root, "jcrpc-gen")
	if e := command(filepath.Join(repo, "codegen"), "go", "build", "-o", bin, "./cmd/jcrpc-gen"); e != nil {
		return e
	}
	inputs := []string{}
	e := filepath.WalkDir(filepath.Join(repo, "examples"), func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() && (d.Name() == "generated" || d.Name() == ".build" || d.Name() == "build") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".toml") {
			inputs = append(inputs, p)
		}
		return nil
	})
	if e != nil {
		return e
	}
	inputs = append(inputs, filepath.Join(repo, "compatibility/inputs/bsim-auth-2d23abd.toml"))
	for i, input := range inputs {
		out := filepath.Join(gen, fmt.Sprint(i))
		// Explicit Java/Kotlin is applicable to all schemas; streams refuse Swift.
		if e = command(repo, bin, "--java", "io.jcrpc.compat", "--kotlin", "io.jcrpc.compat", "--out-dir", out, input); e != nil {
			return e
		}
		// Ordinary examples also exercise Swift. The bsim and stream inputs are
		// tested for refusal by the existing production CLI suite, not swallowed.
		schema, err := codegen.ParseFile(input)
		if err != nil {
			return err
		}
		streams := false
		for _, method := range schema.Methods {
			streams = streams || method.HasStream()
		}
		if !streams {
			if e = command(repo, bin, "--swift", "CompatClient", "--out-dir", out, input); e != nil {
				return e
			}
		}
	}
	// Composite settings live in the consumer root. Generated files stay frozen.
	settings := "rootProject.name = 'pinned-consumer'\n"
	for _, target := range []string{"javacard", "kotlin"} {
		coordinate := "javacard-rpc-server-javacard"
		if target == "kotlin" {
			coordinate = "javacard-rpc-client-kotlin"
		}
		settings += fmt.Sprintf("includeBuild('%s') { dependencySubstitution { substitute(module('io.jcrpc:%s')).using(project(':')) } }\n", filepath.ToSlash(filepath.Join(root, "runtimes", coordinate)), coordinate)
	}
	e = filepath.WalkDir(gen, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, "-server-javacard") || strings.HasSuffix(p, "-client-kotlin") {
			settings += fmt.Sprintf("includeBuild('%s')\n", filepath.ToSlash(p))
		}
		return nil
	})
	if e != nil {
		return e
	}
	// Included packages expose compile/test tasks through their preserved roots.
	build := "plugins { id 'org.jetbrains.kotlin.jvm' version '2.1.10' }\nrepositories { mavenCentral() }\nkotlin { jvmToolchain(17) }\ndependencies { implementation 'io.jcrpc:javacard-rpc-client-kotlin:0.3.0'; implementation 'io.jcrpc:javacard-rpc-server-javacard:0.5.0'; implementation 'io.jcrpc.compat:counter-client-kotlin:1.0.0'; implementation 'io.jcrpc:counter-server-javacard:1.0.0'; testImplementation 'org.jetbrains.kotlin:kotlin-test-junit5:2.1.10'; testRuntimeOnly 'org.junit.platform:junit-platform-launcher:1.11.4' }\ntest { useJUnitPlatform() }\ntasks.named('build') {\n"
	e = filepath.WalkDir(gen, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() && (strings.HasSuffix(p, "-server-javacard") || strings.HasSuffix(p, "-client-kotlin")) {
			build += fmt.Sprintf(" dependsOn gradle.includedBuild('%s').task(':build')\n", filepath.Base(p))
		}
		return nil
	})
	if e != nil {
		return e
	}
	build += "}\n"

	if e = write(filepath.Join(root, "jvm/settings.gradle"), settings); e != nil {
		return e
	}
	if e = write(filepath.Join(root, "jvm/build.gradle"), build); e != nil {
		return e
	}
	fixture, e := os.ReadFile(filepath.Join(repo, "compatibility/fixtures/PinnedConsumerTest.kt"))
	if e != nil {
		return e
	}
	if e = write(filepath.Join(root, "jvm/src/test/kotlin/PinnedConsumerTest.kt"), string(fixture)); e != nil {
		return e
	}
	// Swift's DI protocol requires a host adapter; both packages retain names.
	counter := filepath.Join(gen, "0/counter-client-swift")
	manifest := fmt.Sprintf(`// swift-tools-version: 6.2
import PackageDescription
let package = Package(name: "PinnedConsumer", platforms: [.iOS(.v15), .macOS(.v13)], dependencies: [.package(path: %q), .package(path: %q)], targets: [.testTarget(name: "ConsumerTests", dependencies: [.product(name: "CounterClient", package: "counter-client-swift"), .product(name: "JavaCardRPCClient", package: "javacard-rpc-client-swift")])])
`, counter, filepath.Join(root, "runtimes/javacard-rpc-client-swift"))
	if e = write(filepath.Join(root, "swift/Package.swift"), manifest); e != nil {
		return e
	}
	fixture, e = os.ReadFile(filepath.Join(repo, "compatibility/fixtures/PinnedConsumer.swift"))
	if e != nil {
		return e
	}
	return write(filepath.Join(root, "swift/Tests/ConsumerTests/Consumer.swift"), string(fixture))
}
