package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAllGeneratesPackages(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{
		"--all",
		"--java", "io.jcrpc.counter.server",
		"--swift", "counter",
		"--out-dir", outDir,
		schemaPath(),
	}, &stderr)
	if code != exitCodeSuccess {
		t.Fatalf("run returned exit code %d, stderr:\n%s", code, stderr.String())
	}

	// Java package structure
	javaSrcDir := filepath.Join(outDir, "counter-server-javacard",
		"src", "main", "java", "io", "jcrpc", "counter", "server")
	javaTransportPath := filepath.Join(javaSrcDir, "CounterTransport.java")
	javaSkeletonPath := filepath.Join(javaSrcDir, "CounterSkeleton.java")
	javaBuildGradle := filepath.Join(outDir, "counter-server-javacard", "build.gradle")
	javaSettingsGradle := filepath.Join(outDir, "counter-server-javacard", "settings.gradle")

	assertGoldenEquals(t,
		filepath.Join("..", "..", "testdata", "CounterTransport.java.golden"),
		javaTransportPath,
	)
	assertGoldenEquals(t,
		filepath.Join("..", "..", "testdata", "CounterSkeleton.java.golden"),
		javaSkeletonPath,
	)
	assertFileExists(t, javaBuildGradle)
	assertFileContains(t, javaBuildGradle, "group = 'io.jcrpc'")
	assertFileExists(t, javaSettingsGradle)
	assertFileContains(t, javaSettingsGradle, "rootProject.name = 'counter-server-javacard'")

	// Swift package structure
	swiftClientPath := filepath.Join(outDir, "counter-client-swift",
		"Sources", "CounterClient", "CounterClient.swift")
	swiftPackagePath := filepath.Join(outDir, "counter-client-swift", "Package.swift")

	assertGoldenEquals(t,
		filepath.Join("..", "..", "testdata", "CounterClient.swift.golden"),
		swiftClientPath,
	)
	assertFileExists(t, swiftPackagePath)
	assertFileContains(t, swiftPackagePath, `name: "counter-client-swift"`)
}

func TestRunValidateOnlySuccess(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	code := run([]string{"--validate-only", schemaPath()}, &stderr)
	if code != exitCodeSuccess {
		t.Fatalf("run returned exit code %d, stderr:\n%s", code, stderr.String())
	}
}

func TestRunInvalidSchemaReturnsValidationExitCode(t *testing.T) {
	t.Parallel()

	badSchema := writeFile(t, t.TempDir(), "invalid.toml", `
[applet]
name = "Counter"
version = "1.0.0"
aid = "F000000101"
cla = 0xB0

[methods.alpha]
ins = 0x01

[methods.beta]
ins = 0x01
`)

	var stderr bytes.Buffer
	code := run([]string{"--validate-only", badSchema}, &stderr)
	if code != exitCodeValidation {
		t.Fatalf("run returned exit code %d, want %d, stderr:\n%s", code, exitCodeValidation, stderr.String())
	}

	output := stderr.String()
	if !strings.Contains(output, "validation errors:") {
		t.Fatalf("stderr missing validation header:\n%s", output)
	}
	if !strings.Contains(output, "duplicate INS") {
		t.Fatalf("stderr missing duplicate INS error:\n%s", output)
	}
}

func TestRunMissingInputFileReturnsIOExitCode(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.toml")
	var stderr bytes.Buffer
	code := run([]string{"--validate-only", missing}, &stderr)
	if code != exitCodeIO {
		t.Fatalf("run returned exit code %d, want %d, stderr:\n%s", code, exitCodeIO, stderr.String())
	}
}

func TestRunJavaOnlyWritesJavaPackage(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{
		"--java", "io.jcrpc.counter.server",
		"--out-dir", outDir,
		schemaPath(),
	}, &stderr)
	if code != exitCodeSuccess {
		t.Fatalf("run returned exit code %d, stderr:\n%s", code, stderr.String())
	}

	javaPath := filepath.Join(outDir, "counter-server-javacard",
		"src", "main", "java", "io", "jcrpc", "counter", "server",
		"CounterSkeleton.java")
	assertFileExists(t, javaPath)
	assertFileExists(t, filepath.Join(outDir, "counter-server-javacard", "settings.gradle"))

	// no swift package
	swiftDir := filepath.Join(outDir, "counter-client-swift")
	if _, err := os.Stat(swiftDir); !os.IsNotExist(err) {
		t.Fatalf("did not expect swift package dir %s", swiftDir)
	}
}

func TestRunSwiftOnlyWritesSwiftPackage(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{
		"--swift", "counter",
		"--out-dir", outDir,
		schemaPath(),
	}, &stderr)
	if code != exitCodeSuccess {
		t.Fatalf("run returned exit code %d, stderr:\n%s", code, stderr.String())
	}

	swiftPath := filepath.Join(outDir, "counter-client-swift",
		"Sources", "CounterClient", "CounterClient.swift")
	assertFileExists(t, swiftPath)

	// no java package
	javaDir := filepath.Join(outDir, "counter-server-javacard")
	if _, err := os.Stat(javaDir); !os.IsNotExist(err) {
		t.Fatalf("did not expect java package dir %s", javaDir)
	}
}

func TestRunAllUsesDefaultsWhenLanguagesNotSpecified(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{"--all", "--out-dir", outDir, schemaPath()}, &stderr)
	if code != exitCodeSuccess {
		t.Fatalf("run returned exit code %d, stderr:\n%s", code, stderr.String())
	}

	// default java package = lowercase applet name
	javaBuildGradle := filepath.Join(outDir, "counter-server-javacard", "build.gradle")
	assertFileExists(t, javaBuildGradle)
	assertFileExists(t, filepath.Join(outDir, "counter-server-javacard", "settings.gradle"))

	javaSkeletonPath := filepath.Join(outDir, "counter-server-javacard",
		"src", "main", "java", "counter", "CounterSkeleton.java")
	javaTransportPath := filepath.Join(outDir, "counter-server-javacard",
		"src", "main", "java", "counter", "CounterTransport.java")
	assertFileExists(t, javaSkeletonPath)
	assertFileExists(t, javaTransportPath)
	javaSrc, err := os.ReadFile(javaSkeletonPath)
	if err != nil {
		t.Fatalf("read java file: %v", err)
	}
	if !strings.Contains(string(javaSrc), "package counter;") {
		t.Fatalf("expected default java package in generated output")
	}

	// default swift module
	swiftPath := filepath.Join(outDir, "counter-client-swift",
		"Sources", "CounterClient", "CounterClient.swift")
	assertFileExists(t, swiftPath)
	swiftSrc, err := os.ReadFile(swiftPath)
	if err != nil {
		t.Fatalf("read swift file: %v", err)
	}
	if !strings.Contains(string(swiftSrc), "from `counter.toml`.") {
		t.Fatalf("expected source filename in generated output")
	}
}

func TestGeneratedPackageSwiftMatchesGolden(t *testing.T) {
	t.Parallel()

	got := generatePackageSwift("counter", "CounterClient")
	goldenPath := filepath.Join("..", "..", "testdata", "PackageSwift.golden")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		// write golden on first run
		os.WriteFile(goldenPath, []byte(got), 0o644)
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("no golden file for Package.swift: %v", err)
	}
	if got != string(want) {
		t.Fatalf("Package.swift mismatch:\n%s", lineDiff(want, []byte(got)))
	}
}

func TestGeneratedBuildGradleMatchesGolden(t *testing.T) {
	t.Parallel()

	got := generateBuildGradle("io.jcrpc.counter.server", "1.0.0")
	goldenPath := filepath.Join("..", "..", "testdata", "build.gradle.golden")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		os.WriteFile(goldenPath, []byte(got), 0o644)
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("no golden file for build.gradle: %v", err)
	}
	if got != string(want) {
		t.Fatalf("build.gradle mismatch:\n%s", lineDiff(want, []byte(got)))
	}
}

func schemaPath() string {
	return filepath.Join("..", "..", "testdata", "counter.toml")
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), substr) {
		t.Fatalf("file %s does not contain %q", path, substr)
	}
}

func assertGoldenEquals(t *testing.T, goldenPath, gotPath string) {
	t.Helper()

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read generated file %s: %v", gotPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("generated file %s does not match golden %s:\n%s", gotPath, goldenPath, lineDiff(want, got))
	}
}

func lineDiff(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	maxLines := len(wantLines)
	if len(gotLines) > maxLines {
		maxLines = len(gotLines)
	}

	var b strings.Builder
	b.WriteString("--- want\n")
	b.WriteString("+++ got\n")

	diffs := 0
	for i := 0; i < maxLines; i++ {
		var w string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		var g string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w == g {
			continue
		}
		diffs++
		fmt.Fprintf(&b, "@@ line %d @@\n", i+1)
		fmt.Fprintf(&b, "- %s\n", w)
		fmt.Fprintf(&b, "+ %s\n", g)
		if diffs >= 40 {
			b.WriteString("... diff truncated ...\n")
			break
		}
	}

	return b.String()
}
