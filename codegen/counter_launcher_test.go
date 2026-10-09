package codegen

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/javacard-rpc/codegen/internal/wirecompat"
)

type counterLaunchFixture struct {
	root, script, jar, classpath, checksums, javaCalls, buildCalls string
	files                                                          []string
}

func launcherSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../examples/counter/run-bridge.sh")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newCounterLaunchFixture(t *testing.T, source string) *counterLaunchFixture {
	t.Helper()
	// A path containing spaces also exercises shell/classpath quoting.
	root := filepath.Join(t.TempDir(), "counter layout")
	f := &counterLaunchFixture{root: root}
	f.script = filepath.Join(root, "examples/counter/run-bridge.sh")
	f.jar = filepath.Join(root, "bridge/build/libs/jcrpc-bridge-7.8.9.jar")
	f.classpath = filepath.Join(root, "bridge/build/launch/classpath.txt")
	f.checksums = filepath.Join(root, "bridge/build/launch/checksums.sha256")
	f.javaCalls = filepath.Join(root, "java.calls")
	f.buildCalls = filepath.Join(root, "build.calls")
	put := func(path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	put(f.script, source, 0755)
	// Synthetic file bytes are used only for the no-process preflight tests;
	// canonical make e2e separately exercises the real Gradle archive and JVM.
	put(f.jar, "built archive fixture", 0644)
	dep := filepath.Join(root, "resolved/simulator-custom.jar")
	put(dep, "resolved dependency", 0644)
	config := filepath.Join(root, "bridge/build.gradle")
	put(config, "version = '7.8.9'", 0644)
	put(f.classpath, f.jar+"\n"+dep+"\n", 0644)
	f.files = []string{f.jar, dep, config, f.classpath}
	f.publishChecksums(t)
	put(filepath.Join(root, "bin/java"), "#!/bin/bash\nprintf '%s\\n' \"$@\" > \"$JAVA_CALLS\"\nexit 0\n", 0755)
	build := "#!/bin/bash\nprintf '%s\\n' \"$PWD $*\" >> \"$BUILD_CALLS\"\nexit \"${BUILD_EXIT:-0}\"\n"
	put(filepath.Join(root, "bridge/gradlew"), build, 0755)
	put(filepath.Join(root, "examples/counter/applet/gradlew"), build, 0755)
	return f
}

func (f *counterLaunchFixture) publishChecksums(t *testing.T) {
	t.Helper()
	var b strings.Builder
	for _, path := range f.files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%x  %s\n", sha256.Sum256(raw), path)
	}
	b.WriteString("# end of bridge build checksums\n")
	if err := os.WriteFile(f.checksums, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func (f *counterLaunchFixture) run(t *testing.T, skip, buildExit string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", f.script, "--port", "19425", "--card-provider", "example.Provider", "--card-scope", "shared")
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(f.root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "JCRPC_SKIP_BUILD="+skip, "JAVA_CALLS="+f.javaCalls, "BUILD_CALLS="+f.buildCalls, "BUILD_EXIT="+buildExit)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("launcher timeout: %s", out)
	}
	if err == nil {
		return 0, string(out)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(out)
	}
	t.Fatal(err)
	return -1, string(out)
}

// The actual launcher uses build-published paths at an arbitrary version,
// preserves JVM/config/user flags, and honors skip-build without running Gradle.
// This spy control proves argv, not that the synthetic archive contains Main.
func TestCounterLauncherUsesPublishedClasspath(t *testing.T) {
	for _, skip := range []string{"0", "1"} {
		t.Run("skip="+skip, func(t *testing.T) {
			f := newCounterLaunchFixture(t, launcherSource(t))
			code, out := f.run(t, skip, "0")
			if code != 0 {
				t.Fatalf("launch: exit %d: %s", code, out)
			}
			b, err := os.ReadFile(f.javaCalls)
			if err != nil {
				t.Fatal(err)
			}
			wantCP := f.jar + ":" + filepath.Join(f.root, "resolved/simulator-custom.jar") + ":" + filepath.Join(f.root, "examples/counter/applet/build/libs/counter-applet-0.1.0.jar") + ":" + filepath.Join(f.root, "examples/counter/generated/counter-server-javacard/build/libs/counter-server-javacard-1.0.0.jar")
			want := []string{"--add-modules", "java.smartcardio", "-cp", wantCP, "io.jcrpc.bridge.Main", "--config", filepath.Join(f.root, "examples/counter/bridge.properties"), "--port", "19425", "--card-provider", "example.Provider", "--card-scope", "shared"}
			if got := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"); !reflect.DeepEqual(got, want) {
				t.Fatalf("argv: %q; want %q", got, want)
			}
			builds, err := os.ReadFile(f.buildCalls)
			if skip == "1" {
				if !os.IsNotExist(err) {
					t.Fatalf("skip-build ran Gradle: %s (%v)", builds, err)
				}
			} else if err != nil || strings.Count(string(builds), "build --no-daemon --max-workers=2 -q") != 2 {
				t.Fatalf("build calls: %s (%v)", builds, err)
			}
		})
	}
}

var counterRefusals = []struct {
	name, diagnostic string
	change           func(*testing.T, *counterLaunchFixture)
}{
	{"truncated-checksums", "bridge-build-metadata-invalid", func(t *testing.T, f *counterLaunchFixture) {
		b, err := os.ReadFile(f.checksums)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.checksums, []byte(strings.SplitAfter(string(b), "\n")[0]), 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"truncated-classpath", "bridge-build-metadata-invalid", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(f.classpath, []byte(f.jar), 0644); err != nil {
			t.Fatal(err)
		}
		f.publishChecksums(t)
	}},
	{"missing-checksums", "bridge-build-metadata-missing", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.Remove(f.checksums); err != nil {
			t.Fatal(err)
		}
	}},
	{"missing-metadata", "bridge-build-metadata-missing", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.Remove(f.classpath); err != nil {
			t.Fatal(err)
		}
	}},
	{"empty-metadata", "bridge-build-metadata-missing", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(f.classpath, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"missing-archive", "bridge-artifact-missing", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.Remove(f.jar); err != nil {
			t.Fatal(err)
		}
	}},
	{"ambiguous-archive", "bridge-artifact-ambiguous", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(filepath.Join(filepath.Dir(f.jar), "jcrpc-bridge-0.1.0.jar"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"stale-only-archive", "bridge-artifact-stale", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.Rename(f.jar, filepath.Join(filepath.Dir(f.jar), "jcrpc-bridge-0.1.0.jar")); err != nil {
			t.Fatal(err)
		}
	}},
	{"changed-archive", "bridge-artifact-stale", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(f.jar, []byte("stale archive"), 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"changed-build-version", "bridge-artifact-stale", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(filepath.Join(f.root, "bridge/build.gradle"), []byte("version = '8.0.0'"), 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"missing-runtime", "bridge-artifact-stale", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.Remove(f.files[1]); err != nil {
			t.Fatal(err)
		}
	}},
	{"changed-classpath", "bridge-artifact-stale", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(f.classpath, []byte(f.jar+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"malformed-checksums", "bridge-build-metadata-invalid", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(f.checksums, []byte("not a checksum\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}},
	{"missing-classpath-entry", "bridge-classpath-missing", func(t *testing.T, f *counterLaunchFixture) {
		if err := os.WriteFile(f.classpath, []byte(f.jar+"\n\n"), 0644); err != nil {
			t.Fatal(err)
		}
		f.publishChecksums(t)
	}},
}

// Every refusal drives run-bridge.sh, checks its specific exit/diagnostic and
// proves neither Java nor Gradle started; all fixture mutations are disposable.
func TestCounterLauncherRefusals(t *testing.T) {
	for _, tc := range counterRefusals {
		t.Run(tc.name, func(t *testing.T) {
			f := newCounterLaunchFixture(t, launcherSource(t))
			tc.change(t, f)
			code, out := f.run(t, "1", "0")
			if code != 2 || !strings.Contains(out, tc.diagnostic+":") {
				t.Errorf("want refusal %s exit 2; got %d: %s", tc.diagnostic, code, out)
			}
			for _, path := range []string{f.javaCalls, f.buildCalls} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("forbidden process started: %s (%v)", path, err)
				}
			}
		})
	}
}

// A failed build must stop the launcher before applet build or Java execution.
func TestCounterLauncherBuildFailure(t *testing.T) {
	f := newCounterLaunchFixture(t, launcherSource(t))
	code, out := f.run(t, "0", "17")
	if code != 1 {
		t.Fatalf("failed build: exit %d: %s", code, out)
	}
	if _, err := os.Stat(f.javaCalls); !os.IsNotExist(err) {
		t.Fatal("failed build launched Java")
	}
	b, err := os.ReadFile(f.buildCalls)
	if err != nil || strings.Count(string(b), "build --no-daemon --max-workers=2 -q") != 1 {
		t.Fatalf("build failure continued: %s (%v)", b, err)
	}
}

// Each plant weakens one gate to admit one named bad case; unchanged refusal
// tests run through the real shell entry point. A crash/setup error is no kill.
func TestCounterLauncherNarrowingMutants(t *testing.T) {
	source := launcherSource(t)
	plants := []struct{ name, before, after, test string }{
		{"truncated-checksums", `[ "$(tail -n 1 "$LAUNCH_DIR/checksums.sha256")" != "# end of bridge build checksums" ]`, `[ "$(tail -n 1 "$LAUNCH_DIR/checksums.sha256")" != "# end of bridge build checksums" ] && [ "$(wc -l < "$LAUNCH_DIR/checksums.sha256" | tr -d ' ')" != "1" ]`, "truncated-checksums"},
		{"truncated-classpath", `IFS= read -r BRIDGE_JAR < "$LAUNCH_DIR/classpath.txt" || refuse`, `IFS= read -r BRIDGE_JAR < "$LAUNCH_DIR/classpath.txt" || [ "$BRIDGE_JAR" = "$BRIDGE_DIR/build/libs/jcrpc-bridge-7.8.9.jar" ] || refuse`, "truncated-classpath"},
		{"missing-checksums", "validate_bridge_build() {", "validate_bridge_build() {\nif [ ! -e \"$LAUNCH_DIR/checksums.sha256\" ] && [ -s \"$LAUNCH_DIR/classpath.txt\" ]; then return; fi", "missing-checksums"},
		{"missing-archive", "validate_bridge_build() {", "validate_bridge_build() {\nif [ ! -e \"$BRIDGE_DIR/build/libs/jcrpc-bridge-7.8.9.jar\" ] && [ ! -e \"$BRIDGE_DIR/build/libs/jcrpc-bridge-0.1.0.jar\" ]; then return; fi", "missing-archive"},
		{"two-archives", "validate_bridge_build() {", "validate_bridge_build() {\nif [ -f \"$BRIDGE_DIR/build/libs/jcrpc-bridge-7.8.9.jar\" ] && [ -f \"$BRIDGE_DIR/build/libs/jcrpc-bridge-0.1.0.jar\" ]; then return; fi", "ambiguous-archive"},
		{"stale-name", "validate_bridge_build() {", "validate_bridge_build() {\nif [ ! -e \"$BRIDGE_DIR/build/libs/jcrpc-bridge-7.8.9.jar\" ] && [ -f \"$BRIDGE_DIR/build/libs/jcrpc-bridge-0.1.0.jar\" ]; then return; fi", "stale-only-archive"},
		{"changed-archive", `if ! shasum -a 256 -c "$LAUNCH_DIR/checksums.sha256" >/dev/null 2>&1; then`, `if ! shasum -a 256 -c "$LAUNCH_DIR/checksums.sha256" >/dev/null 2>&1 && [ "$(cat "$BRIDGE_JAR")" != "stale archive" ]; then`, "changed-archive"},
		{"empty-classpath-entry", `[ -n "$entry" ] || refuse`, `[ -z "$entry" ] && [ "$FULL_CP" = "$BRIDGE_JAR" ] || [ -n "$entry" ] || refuse`, "missing-classpath-entry"},
	}
	for _, p := range plants {
		t.Run(p.name, func(t *testing.T) {
			if strings.Count(source, p.before) != 1 {
				t.Fatal("mutant anchor not unique")
			}
			// Install the candidate script in a temporary sibling layout and execute
			// the original named test against it, without changing the working tree.
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "codegen"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "examples/counter"), 0755); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile("counter_launcher_test.go")
			if err != nil {
				t.Fatal(err)
			}
			for path, raw := range map[string][]byte{"codegen/counter_launcher_test.go": b, "codegen/go.mod": []byte("module launcher-mutant\n\ngo 1.24\n"), "examples/counter/run-bridge.sh": []byte(strings.Replace(source, p.before, p.after, 1))} {
				if err := os.WriteFile(filepath.Join(root, path), raw, 0644); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("go", "test", ".", "-run", "^TestCounterLauncherRefusals$/^"+p.test+"$", "-count=1", "-v")
			cmd.Dir = filepath.Join(root, "codegen")
			out, err := cmd.CombinedOutput()
			exit, ok := err.(*exec.ExitError)
			name := "TestCounterLauncherRefusals/" + p.test
			if !ok || exit.ExitCode() != 1 || !strings.Contains(string(out), "--- FAIL: "+name) || !strings.Contains(string(out), "forbidden process started:") || !strings.Contains(string(out), "got 0:") {
				t.Fatalf("survived or unrelated failure: %v\n%s", err, out)
			}
			t.Logf("killed %s by %s; nested go test exit 1\n%s", p.name, name, out)
		})
	}
}

// Against an explicitly supplied real built layout, run the production launcher
// and reach Main's typed scope refusal. This proves the actual archive and its
// resolved dependencies load without opening a listener. It is not an E2E substitute.
func TestCounterLauncherBuiltBridge(t *testing.T) {
	root := os.Getenv("JCRPC_COUNTER_LAYOUT")
	if root == "" {
		t.Skip("set JCRPC_COUNTER_LAYOUT to a disposable built normal-layout checkout")
	}
	cmd := exec.Command("bash", filepath.Join(root, "examples/counter/run-bridge.sh"), "--card-scope", "invalid-launch-control", "--port", "0")
	cmd.Env = append(os.Environ(), "JCRPC_SKIP_BUILD=1")
	out, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 || !strings.Contains(string(out), "[bridge] startup refused:") || !strings.Contains(string(out), "invalid-launch-control") || strings.Contains(string(out), "[bridge] listening on") {
		t.Fatalf("actual Main scope refusal: %v\n%s", err, out)
	}
	t.Logf("production Java exit 2 (expected scope refusal, no listener):\n%s", out)
}

// The canonical make entry point builds and exercises both real clients in a
// disposable checkout with real sibling runtime snapshots. The optional layout
// is never a foreign repository; run this lane serially on free port 9025.
func TestCounterCanonicalE2E(t *testing.T) {
	root := os.Getenv("JCRPC_COUNTER_LAYOUT")
	if root == "" {
		t.Skip("set JCRPC_COUNTER_LAYOUT to a disposable normal-layout checkout")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:9025")
	if err != nil {
		t.Fatalf("port 9025 unavailable; no existing process changed: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("make", "e2e")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	t.Logf("canonical make e2e output:\n%s", out)
	if err != nil {
		t.Fatalf("canonical make e2e: %v", err)
	}
	for _, want := range []string{"[run-e2e] bridge is ready", "[run-e2e] running Swift E2E harness...", "[run-e2e] running Kotlin E2E harness..."} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing canonical evidence: %s", want)
		}
	}
	if strings.Count(string(out), "=== Results: 18 passed, 0 failed ===") != 2 {
		t.Error("both clients must report all 18 cases passing")
	}
	t.Log("canonical make e2e real exit 0")
}

// Both actual generator binaries preserve the nine-file inventory and eight
// non-skeleton files. The Java skeleton intentionally migrates to caller scratch;
// the very emitted Java corpora run shared wire probes against the immutable
// baseline. A changed Swift byte is detected separately.
func TestCounterGeneratedPackageParity(t *testing.T) {
	baseline := os.Getenv("JCRPC_COUNTER_BASELINE_GEN")
	if baseline == "" {
		t.Skip("set JCRPC_COUNTER_BASELINE_GEN to the immutable v0.5.0 generator")
	}
	candidate := os.Getenv("JCRPC_CANDIDATE_GEN")
	if candidate == "" {
		candidate = filepath.Join(t.TempDir(), "jcrpc-gen")
		if out, err := exec.Command("go", "build", "-o", candidate, "./cmd/jcrpc-gen").CombinedOutput(); err != nil {
			t.Fatalf("candidate build: %v\n%s", err, out)
		}
	}
	trees := []map[string]string{}
	var candidateOutput string
	var outputs []string
	for _, bin := range []string{baseline, candidate} {
		dir := t.TempDir()
		candidateOutput = dir
		outputs = append(outputs, dir)
		cmd := exec.Command(bin, "--all", "--out-dir", dir, "../examples/counter/counter.toml")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generation: %v\n%s", err, out)
		}
		tree := map[string]string{}
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.HasSuffix(rel, "Skeleton.java") {
				// Deliberately breaking Java API; inventory still counts this file.
				tree[rel] = "authorized caller-workspace API delta"
				return nil
			}
			tree[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		trees = append(trees, tree)
	}
	schema, err := ParseFile("../examples/counter/counter.toml")
	if err != nil {
		t.Fatal(err)
	}
	if err := wirecompat.Compare(t, outputs[0], outputs[1], schema); err != nil {
		t.Fatal(err)
	}
	if len(trees[0]) != 9 || !reflect.DeepEqual(trees[0], trees[1]) {
		t.Fatalf("generated package byte drift: checkpoint %v; candidate %v", trees[0], trees[1])
	}
	// Narrow control: retain the inventory and eight hashes, alter one output byte.
	control := map[string]string{}
	for path, hash := range trees[1] {
		control[path] = hash
	}
	controlPath := filepath.Join(candidateOutput, "counter-client-swift/Package.swift")
	raw, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(controlPath, raw, 0644); err != nil {
		t.Fatal(err)
	}
	control["counter-client-swift/Package.swift"] = fmt.Sprintf("%x", sha256.Sum256(raw))
	if reflect.DeepEqual(trees[0], control) {
		t.Fatal("parity comparator missed changed-byte control")
	}
	t.Logf("8 of 9 generated files identical; Java skeleton API delta declared; changed-byte control rejected; SHA-256: %v", trees[1])
}
