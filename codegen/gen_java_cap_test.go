package codegen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredCAPTargetFailsWhenAntIsUnavailable(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not available")
	}
	command := exec.Command(makePath, "-f", filepath.Join("..", "Makefile"), "test-cap")
	command.Env = append(os.Environ(),
		"PATH="+t.TempDir(),
		"JCRPC_ANT_JAVACARD_JAR=/configured/ant-javacard.jar",
		"JCRPC_JCKIT_DIR=/configured/jckit",
	)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("required CAP target unexpectedly passed without ant:\n%s", output)
	}
	if !strings.Contains(string(output), "ant is required") {
		t.Fatalf("required CAP target failed for the wrong reason: %v\n%s", err, output)
	}
}

func TestGeneratedJavaStreamPackageConvertsToCAP(t *testing.T) {
	ant, err := exec.LookPath("ant")
	if err != nil {
		t.Skip("ant is not available")
	}
	antJavaCardJar := os.Getenv("JCRPC_ANT_JAVACARD_JAR")
	jckitDir := os.Getenv("JCRPC_JCKIT_DIR")
	if antJavaCardJar == "" || jckitDir == "" {
		t.Skip("set JCRPC_ANT_JAVACARD_JAR and JCRPC_JCKIT_DIR for CAP conversion smoke")
	}
	// Real converter verification includes both storage policies and both
	// lifecycle modes; simulator javac success alone does not establish this.
	for _, policy := range []string{"transient", "persistent"} {
		for _, memory := range []StreamMemory{StreamMemoryClearOnDeselect, StreamMemoryClearOnReset} {
			t.Run(policy+"/"+string(memory), func(t *testing.T) {
				convertStreamCAP(t, ant, antJavaCardJar, jckitDir, policy, memory)
			})
		}
	}
}

func convertStreamCAP(t *testing.T, ant, antJavaCardJar, jckitDir, policy string, memory StreamMemory) {
	t.Helper()

	schema, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	schema.Applet.StreamWorkspace = policy
	result, err := GenerateJavaSkeletonWithOptions(schema, "io.jcrpc.streamdemo.server", JavaOptions{StreamMemory: memory})
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}
	convertFacadeFixtureCAP(t, ant, antJavaCardJar, jckitDir, result, streamDemoAppletFixture)
}

func convertFacadeFixtureCAP(t *testing.T, ant, antJavaCardJar, jckitDir string, result *JavaGenerationResult, fixture string) {
	t.Helper()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "src", "io", "jcrpc", "streamdemo", "server")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll source: %v", err)
	}
	files := map[string][]byte{
		result.TransportName + ".java":         result.TransportSource,
		result.SkeletonName + ".java":          result.SkeletonSource,
		result.StreamEndpointName + ".java":    result.StreamEndpointSource,
		result.StreamRuntimeName + ".java":     result.StreamRuntimeSource,
		result.StreamAPDUAdapterName + ".java": result.StreamAPDUAdapterSource,
		"StreamDemoApplet.java":                []byte(fixture),
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(sourceDir, name), source, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	buildXML := fmt.Sprintf(`<project name="generated-stream-cap" default="cap">
    <taskdef name="javacard" classname="pro.javacard.ant.JavaCard" classpath="%s"/>
    <target name="cap">
        <javacard>
            <cap jckit="%s"

                 sources="%s"
                 package="io.jcrpc.streamdemo.server"
                 aid="F000000102"
                 version="1.0"
                 ints="true"
                 output="%s">
                <applet class="io.jcrpc.streamdemo.server.StreamDemoApplet" aid="F00000010201"/>
            </cap>
        </javacard>
    </target>
</project>
`, filepath.ToSlash(antJavaCardJar), filepath.ToSlash(jckitDir),
		filepath.ToSlash(filepath.Join(root, "src")), filepath.ToSlash(filepath.Join(root, "streamdemo.cap")))
	buildPath := filepath.Join(root, "build.xml")
	if err := os.WriteFile(buildPath, []byte(buildXML), 0o644); err != nil {
		t.Fatalf("write build.xml: %v", err)
	}

	command := exec.Command(ant, "-f", buildPath, "cap")
	command.Dir = root
	output, err := command.CombinedOutput()
	code := -1
	if command.ProcessState != nil {
		code = command.ProcessState.ExitCode()
	}
	t.Logf("actual Ant command %s; exit %d\n%s", command.String(), code, output)
	if err != nil {
		t.Fatalf("CAP conversion failed: %v\n%s", err, output)
	}
	if info, err := os.Stat(filepath.Join(root, "streamdemo.cap")); err != nil || info.Size() == 0 {
		t.Fatalf("CAP output missing or empty: %v", err)
	}
	capBytes, err := os.ReadFile(filepath.Join(root, "streamdemo.cap"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("CAP SHA256=%x bytes=%d", sha256.Sum256(capBytes), len(capBytes))
	if out := os.Getenv("JCRPC_FACADE_CAP_OUT"); out != "" {
		if err = os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		name := strings.ReplaceAll(t.Name(), "/", "_")
		writeTestFile(t, filepath.Join(out, name+".cap"), capBytes)
		writeTestFile(t, filepath.Join(out, name+".log"), output)
	}
}
