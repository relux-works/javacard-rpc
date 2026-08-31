package codegen

import (
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

	schema, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	result, err := GenerateJavaSkeleton(schema, "io.jcrpc.streamdemo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

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
		"StreamDemoApplet.java":                []byte(streamDemoAppletFixture),
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
                 targetsdk="3.0.5"
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
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("CAP conversion failed: %v\n%s", err, output)
	}
	if info, err := os.Stat(filepath.Join(root, "streamdemo.cap")); err != nil || info.Size() == 0 {
		t.Fatalf("CAP output missing or empty: %v", err)
	}
}
