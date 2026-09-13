package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mavenLocalProbeJar is the simulator jar used to seed the temp Maven repo.
// It exists in ~/.m2 on hosts that published the relux jCardSim fork; the
// compile probe is skipped (stated bound) elsewhere.
var mavenLocalProbeJar = filepath.Join(
	"works", "relux", "jcardsim", "3.0.5.9-relux.2", "jcardsim-3.0.5.9-relux.2.jar")

// probeCoordinate exists only inside the temp repository the test creates:
// it is neither on Maven Central nor in the host ~/.m2, so a resolution can
// succeed only through the generated build's mavenLocal() repository.
const probeCoordinate = "test.jcrpc:probe-sim:0.0.1"

// Proves that the Java Card server build emitted through run compiles with a
// --simulator-dependency that is present only in the Maven local repository
// (the generated build.gradle declares mavenLocal(), Gradle is pointed at a
// temp repo via -Dmaven.repo.local, offline so Central is never consulted).
// The paired negative removes only the mavenLocal() line from the same
// generated build and requires :compileJava to fail resolving the coordinate,
// so a generator that omits mavenLocal() cannot pass the positive half while
// a build that ignores repositories cannot pass the negative half.
// Bound: the seeded jar is the real relux jCardSim fork from ~/.m2; the test
// skips when Gradle or that jar is absent.
func TestRunStreamBuildGradleCompilesWithMavenLocalOnlyOverride(t *testing.T) {
	gradle, err := exec.LookPath("gradle")
	if err != nil {
		t.Skip("gradle is not available")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	seedJar := filepath.Join(home, ".m2", "repository", mavenLocalProbeJar)
	if _, err := os.Stat(seedJar); err != nil {
		t.Skipf("seed simulator jar not present: %s", seedJar)
	}

	repo := seedTempMavenRepo(t, seedJar)

	outDir := t.TempDir()
	var stderr bytes.Buffer
	code := run([]string{
		"--java", "io.jcrpc.streamdemo.server",
		"--out-dir", outDir,
		"--simulator-dependency", probeCoordinate,
		filepath.Join("..", "..", "testdata", "stream.toml"),
	}, &stderr)
	if code != exitCodeSuccess {
		t.Fatalf("run returned exit code %d, stderr:\n%s", code, stderr.String())
	}
	pkgDir := filepath.Join(outDir, "streamdemo-server-javacard")
	gradlePath := filepath.Join(pkgDir, "build.gradle")
	assertFileContains(t, gradlePath, "compileOnly '"+probeCoordinate+"'")

	// Positive control: resolves through mavenLocal() and compiles.
	if output, err := gradleCompile(gradle, pkgDir, repo); err != nil {
		t.Fatalf("generated build failed to compile against a mavenLocal-only override: %v\n%s", err, output)
	}

	// Negative: same generated build with only the mavenLocal() line removed.
	content, err := os.ReadFile(gradlePath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(content), "    mavenLocal()\n", "", 1)
	if mutated == string(content) {
		t.Fatalf("generated build.gradle has no mavenLocal() line to remove:\n%s", content)
	}
	if err := os.WriteFile(gradlePath, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := gradleCompile(gradle, pkgDir, repo)
	if err == nil {
		t.Fatalf("build without mavenLocal() unexpectedly resolved %s:\n%s", probeCoordinate, output)
	}
	if !strings.Contains(string(output), "Could not resolve "+probeCoordinate) {
		t.Fatalf("build without mavenLocal() failed for the wrong reason: %v\n%s", err, output)
	}
}

// seedTempMavenRepo lays out probeCoordinate as a Maven 2 repository rooted in
// a temp dir, with the given jar as its only artifact.
func seedTempMavenRepo(t *testing.T, jar string) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "test", "jcrpc", "probe-sim", "0.0.1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(jar)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe-sim-0.0.1.jar"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	pom := `<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>test.jcrpc</groupId>
  <artifactId>probe-sim</artifactId>
  <version>0.0.1</version>
  <packaging>jar</packaging>
</project>
`
	if err := os.WriteFile(filepath.Join(dir, "probe-sim-0.0.1.pom"), []byte(pom), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// gradleCompile runs :compileJava of the generated build offline with the
// Maven local repository redirected to repo, so neither the network nor the
// host ~/.m2 can satisfy the probe coordinate.
func gradleCompile(gradle, dir, repo string) ([]byte, error) {
	cmd := exec.Command(gradle, "compileJava", "--offline", "--no-daemon", "-q",
		"-Dmaven.repo.local="+repo)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
