package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/relux-works/javacard-rpc/codegen"
)

const (
	exitCodeSuccess    = 0
	exitCodeValidation = 1
	exitCodeGeneration = 2
	exitCodeIO         = 3
)

type cliOptions struct {
	outDir       string
	javaPackage  string
	swiftModule  string
	all          bool
	validateOnly bool
	verbose      bool
	help         bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	opts := cliOptions{}
	fs := flag.NewFlagSet("jcrpc-gen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&opts.outDir, "out-dir", ".", "")
	fs.StringVar(&opts.javaPackage, "java", "", "")
	fs.StringVar(&opts.swiftModule, "swift", "", "")
	fs.BoolVar(&opts.all, "all", false, "")
	fs.BoolVar(&opts.validateOnly, "validate-only", false, "")
	fs.BoolVar(&opts.verbose, "verbose", false, "")
	fs.BoolVar(&opts.help, "help", false, "")
	fs.BoolVar(&opts.help, "h", false, "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stderr)
			return exitCodeSuccess
		}
		fmt.Fprintf(stderr, "error: %v\n\n", err)
		printUsage(stderr)
		return exitCodeGeneration
	}

	if opts.help {
		printUsage(stderr)
		return exitCodeSuccess
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: expected exactly one input TOML file")
		fmt.Fprintln(stderr)
		printUsage(stderr)
		return exitCodeGeneration
	}

	inputPath := fs.Arg(0)
	verbosef := func(format string, a ...any) {
		if opts.verbose {
			fmt.Fprintf(stderr, format+"\n", a...)
		}
	}

	verbosef("parsing %s", inputPath)
	schema, err := codegen.ParseFile(inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", inputPath, err)
		if isIOError(err) {
			return exitCodeIO
		}
		return exitCodeValidation
	}

	verbosef("validating schema")
	validationErrs := codegen.Validate(schema)
	if len(validationErrs) > 0 {
		printValidationErrors(stderr, inputPath, validationErrs)
		return exitCodeValidation
	}

	if opts.validateOnly {
		verbosef("validation successful")
		return exitCodeSuccess
	}

	javaPackage := strings.TrimSpace(opts.javaPackage)
	swiftModule := strings.TrimSpace(opts.swiftModule)
	if opts.all {
		if javaPackage == "" {
			javaPackage = defaultJavaPackage(schema.Applet.Name)
		}
		if swiftModule == "" {
			swiftModule = defaultSwiftModule(schema.Applet.Name)
		}
	}

	generateJava := javaPackage != ""
	generateSwift := swiftModule != ""
	if !generateJava && !generateSwift {
		fmt.Fprintln(stderr, "error: no outputs selected (use --java, --swift, or --all)")
		return exitCodeGeneration
	}

	appletStem := appletFileStem(schema.Applet.Name)
	appletLower := strings.ToLower(appletStem)
	generated := make([]string, 0, 4)

	if generateJava {
		verbosef("generating java package (package=%s)", javaPackage)
		javaResult, err := codegen.GenerateJavaSkeleton(schema, javaPackage)
		if err != nil {
			fmt.Fprintf(stderr, "generate java skeleton: %v\n", err)
			return exitCodeGeneration
		}

		// <out-dir>/<name>-server-javacard/
		pkgDir := filepath.Join(opts.outDir, appletLower+"-server-javacard")
		// src/main/java/<package/path>/
		pkgPath := strings.ReplaceAll(javaPackage, ".", string(filepath.Separator))
		srcDir := filepath.Join(pkgDir, "src", "main", "java", pkgPath)

		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "create java package dir %q: %v\n", srcDir, err)
			return exitCodeIO
		}

		// write settings.gradle
		settingsPath := filepath.Join(pkgDir, "settings.gradle")
		settingsContent := fmt.Sprintf("rootProject.name = '%s-server-javacard'\n", appletLower)
		if err := os.WriteFile(settingsPath, []byte(settingsContent), 0o644); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", settingsPath, err)
			return exitCodeIO
		}
		generated = append(generated, settingsPath)

		// write build.gradle
		gradlePath := filepath.Join(pkgDir, "build.gradle")
		gradleContent := generateBuildGradle(javaPackage, schema.Applet.Version)
		if err := os.WriteFile(gradlePath, []byte(gradleContent), 0o644); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", gradlePath, err)
			return exitCodeIO
		}
		generated = append(generated, gradlePath)

		// write transport interface
		transportPath := filepath.Join(srcDir, javaResult.TransportName+".java")
		if err := os.WriteFile(transportPath, javaResult.TransportSource, 0o644); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", transportPath, err)
			return exitCodeIO
		}
		generated = append(generated, transportPath)

		// write skeleton
		skeletonPath := filepath.Join(srcDir, javaResult.SkeletonName+".java")
		if err := os.WriteFile(skeletonPath, javaResult.SkeletonSource, 0o644); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", skeletonPath, err)
			return exitCodeIO
		}
		generated = append(generated, skeletonPath)
	}

	if generateSwift {
		verbosef("generating swift package (module=%s)", swiftModule)
		swiftSource, err := codegen.GenerateSwiftClient(schema, swiftModule)
		if err != nil {
			fmt.Fprintf(stderr, "generate swift client: %v\n", err)
			return exitCodeGeneration
		}

		clientName := appletStem + "Client"
		// <out-dir>/<name>-client-swift/
		pkgDir := filepath.Join(opts.outDir, appletLower+"-client-swift")
		srcDir := filepath.Join(pkgDir, "Sources", clientName)

		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			fmt.Fprintf(stderr, "create swift package dir %q: %v\n", srcDir, err)
			return exitCodeIO
		}

		// write Package.swift
		packageSwiftPath := filepath.Join(pkgDir, "Package.swift")
		packageSwiftContent := generatePackageSwift(appletLower, clientName)
		if err := os.WriteFile(packageSwiftPath, []byte(packageSwiftContent), 0o644); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", packageSwiftPath, err)
			return exitCodeIO
		}
		generated = append(generated, packageSwiftPath)

		// write client source
		swiftPath := filepath.Join(srcDir, clientName+".swift")
		if err := os.WriteFile(swiftPath, swiftSource, 0o644); err != nil {
			fmt.Fprintf(stderr, "write %s: %v\n", swiftPath, err)
			return exitCodeIO
		}
		generated = append(generated, swiftPath)
	}

	if opts.verbose {
		fmt.Fprintf(stderr, "generated %d file(s):\n", len(generated))
		for _, path := range generated {
			fmt.Fprintf(stderr, "  %s\n", path)
		}
	}

	return exitCodeSuccess
}

func generatePackageSwift(appletLower, clientName string) string {
	return fmt.Sprintf(`// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "%[1]s-client-swift",
    platforms: [
        .iOS(.v15),
        .macOS(.v12),
    ],
    products: [
        .library(name: "%[2]s", targets: ["%[2]s"]),
    ],
    targets: [
        .target(
            name: "%[2]s",
            path: "Sources/%[2]s"
        ),
    ]
)
`, appletLower, clientName)
}

func generateBuildGradle(javaPackage, version string) string {
	// extract group from package: io.jcrpc.counter.server -> io.jcrpc
	group := javaPackageGroup(javaPackage)
	return fmt.Sprintf(`plugins {
    id 'java-library'
}

group = '%s'
version = '%s'

java {
    sourceCompatibility = JavaVersion.VERSION_1_8
    targetCompatibility = JavaVersion.VERSION_1_8
}

tasks.withType(JavaCompile).configureEach {
    options.compilerArgs += ['-Xlint:-options']
}

repositories {
    mavenCentral()
}
`, group, version)
}

func javaPackageGroup(pkg string) string {
	parts := strings.Split(pkg, ".")
	if len(parts) <= 2 {
		return pkg
	}
	return strings.Join(parts[:2], ".")
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "jcrpc-gen [flags] <input.toml>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --out-dir string      Output directory (default \".\")")
	fmt.Fprintln(w, "  --java string         Generate Java skeleton with given package name")
	fmt.Fprintln(w, "  --swift string        Generate Swift client with given module name")
	fmt.Fprintln(w, "  --all                 Generate both Java and Swift (uses applet name for defaults)")
	fmt.Fprintln(w, "  --validate-only       Parse + validate only, no generation")
	fmt.Fprintln(w, "  --verbose             Print progress to stderr")
	fmt.Fprintln(w, "  -h, --help            Show help")
}

func printValidationErrors(w io.Writer, inputPath string, errs []codegen.ValidationError) {
	fmt.Fprintf(w, "%s: validation errors:\n", filepath.Base(inputPath))
	for _, err := range errs {
		fmt.Fprintf(w, "  %s: %s\n", err.Path, err.Message)
	}
}

func defaultJavaPackage(appletName string) string {
	return strings.ToLower(appletFileStem(appletName))
}

func defaultSwiftModule(appletName string) string {
	return appletFileStem(appletName) + "Client"
}

func appletFileStem(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Applet"
	}

	var b strings.Builder
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Applet"
	}
	return b.String()
}

func isIOError(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return true
	}

	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrExist)
}
