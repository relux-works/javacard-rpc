package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredKotlinContractTargetFailsWhenGradleIsUnavailable(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not available")
	}
	command := exec.Command(makePath, "-f", filepath.Join("..", "Makefile"), "test-kotlin-contract")
	command.Env = append(os.Environ(), "PATH="+t.TempDir())
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("required Kotlin target unexpectedly passed without Gradle:\n%s", output)
	}
	if !strings.Contains(string(output), "gradle is required") {
		t.Fatalf("required Kotlin target failed for the wrong reason: %v\n%s", err, output)
	}
}

func TestGeneratedKotlinStreamClientHarness(t *testing.T) {
	gradle, err := exec.LookPath("gradle")
	if err != nil {
		t.Skip("gradle is not available")
	}

	s, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	s.Methods["sign"] = &Method{
		Name: "sign",
		INS:  0x30,
		Request: &Message{Fields: []Field{{
			Name: "payload", Type: FieldTypeStream, MaxLength: 1024, ChunkSize: 192,
		}}},
		Response: &Message{Fields: []Field{{Name: "receipt", Type: FieldTypeU16}}},
	}
	s.Methods["export"] = &Method{
		Name: "export",
		INS:  0x40,
		Request: &Message{Fields: []Field{{
			Name: "selector", Type: FieldTypeU8, Location: ParameterLocationData,
		}}},
		Response: &Message{Fields: []Field{{
			Name: "packet", Type: FieldTypeStream, MaxLength: 2048, ChunkSize: 224,
		}}},
	}
	s.Methods["getFixedInfo"] = &Method{
		Name: "getFixedInfo",
		INS:  0x02,
		Response: &Message{Fields: []Field{
			{Name: "schema", Type: FieldTypeU8},
			{Name: "identity", Type: FieldTypeBytesFixed, FixedLength: 10},
			{Name: "generation", Type: FieldTypeU32},
		}},
	}
	source, err := GenerateKotlinClient(s, "io.jcrpc.streamdemo.client")
	if err != nil {
		t.Fatalf("GenerateKotlinClient returned error: %v", err)
	}
	javaResult, err := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	root := t.TempDir()
	mainDir := filepath.Join(root, "src", "main", "kotlin", "io", "jcrpc", "streamdemo", "client")
	javaDir := filepath.Join(root, "src", "main", "java", "io", "jcrpc", "streamdemo", "server")
	testDir := filepath.Join(root, "src", "test", "kotlin", "io", "jcrpc", "streamdemo", "client")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatalf("MkdirAll main: %v", err)
	}
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatalf("MkdirAll test: %v", err)
	}
	if err := os.MkdirAll(javaDir, 0o755); err != nil {
		t.Fatalf("MkdirAll Java: %v", err)
	}
	writeTestFile(t, filepath.Join(mainDir, "StreamDemoClient.kt"), source)
	writeTestFile(t, filepath.Join(javaDir, javaResult.StreamEndpointName+".java"), javaResult.StreamEndpointSource)
	writeTestFile(t, filepath.Join(javaDir, javaResult.StreamRuntimeName+".java"), javaResult.StreamRuntimeSource)
	harness, err := os.ReadFile(filepath.Join("testdata", "StreamClientHarness.kt"))
	if err != nil {
		t.Fatalf("read harness: %v", err)
	}
	writeTestFile(t, filepath.Join(testDir, "StreamClientHarness.kt"), harness)
	writeTestFile(t, filepath.Join(root, "build.gradle.kts"), []byte(GenerateKotlinBuildGradle("streamdemo", "io.jcrpc.streamdemo.client", "1.0.0")))
	runtimeFixture, err := filepath.Abs(filepath.Join("testdata", "kotlin-runtime"))
	if err != nil {
		t.Fatalf("resolve Kotlin runtime fixture: %v", err)
	}
	settings := GenerateKotlinSettingsGradle("streamdemo") + `

includeBuild("` + filepath.ToSlash(runtimeFixture) + `") {
    dependencySubstitution {
        substitute(module("io.jcrpc:javacard-rpc-client-kotlin")).using(project(":"))
    }
}
`
	writeTestFile(t, filepath.Join(root, "settings.gradle.kts"), []byte(settings))

	cmd := exec.Command(gradle, "test", "--no-daemon", "-q")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Kotlin stream harness failed: %v\n%s", err, output)
	}
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
