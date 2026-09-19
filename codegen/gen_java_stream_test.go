package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGeneratedJavaStreamRuntimeHarness(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not available")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not available")
	}

	s, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	result, err := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	root := t.TempDir()
	javaCardStub := writeJavaCardJCSystemStub(t, root)
	packageDir := filepath.Join(root, "io", "jcrpc", "streamdemo", "server")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	files := map[string][]byte{
		result.StreamEndpointName + ".java": result.StreamEndpointSource,
		result.StreamRuntimeName + ".java":  result.StreamRuntimeSource,
	}
	harness, err := os.ReadFile(filepath.Join("testdata", "StreamRuntimeHarness.java"))
	if err != nil {
		t.Fatalf("read harness: %v", err)
	}
	files["StreamRuntimeHarness.java"] = harness

	paths := make([]string, 0, len(files))
	paths = append(paths, javaCardStub)
	for name, source := range files {
		path := filepath.Join(packageDir, name)
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		paths = append(paths, path)
	}

	compileArgs := append([]string{"-d", root}, paths...)
	if output, err := exec.Command(javac, compileArgs...).CombinedOutput(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, output)
	}
	cmd := exec.Command(java, "-cp", root, "io.jcrpc.streamdemo.server.StreamRuntimeHarness")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stream runtime harness failed: %v\n%s", err, output)
	}
}
