package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
	writeTestFile(t, filepath.Join(root, "settings.gradle.kts"), []byte(GenerateKotlinSettingsGradle("streamdemo")))

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
