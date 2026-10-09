package codegen

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Converts both unchanged Java package identities: the generated counter
// library CAP/EXP, then the real wrapper/business applet CAP importing it.
// This is verified Classic packaging, not a physical installation claim.
// The host-Math plant must fail specifically against the real SDK API before
// conversion. Oracle 3.0.5u4 crashes internally on that unresolved invocation;
// a converter crash is not a successful negative control.
func TestCounterWriterClassicCAP(t *testing.T) {
	ant, e := exec.LookPath("ant")
	if e != nil {
		t.Skip("ant is not available")
	}
	task, kit := os.Getenv("JCRPC_ANT_JAVACARD_JAR"), os.Getenv("JCRPC_JCKIT_DIR")
	if task == "" || kit == "" {
		t.Skip("set JCRPC_ANT_JAVACARD_JAR and JCRPC_JCKIT_DIR for real Counter CAP conversion")
	}
	root := t.TempDir()
	schema, e := ParseFile("../examples/counter/counter.toml")
	if e != nil {
		t.Fatal(e)
	}
	generated, e := GenerateJavaSkeleton(schema, "counter")
	if e != nil {
		t.Fatal(e)
	}
	for name, b := range map[string][]byte{"CounterSkeleton.java": generated.SkeletonSource, "CounterTransport.java": generated.TransportSource} {
		p := filepath.Join(root, "library-src/counter", name)
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			t.Fatal(e)
		}
		writeTestFile(t, p, b)
	}
	for _, name := range []string{"CounterApplet.java", "CounterJCApplet.java"} {
		b, e := os.ReadFile(filepath.Join("../examples/counter/applet/src/main/java/io/jcrpc/example", name))
		if e != nil {
			t.Fatal(e)
		}
		p := filepath.Join(root, "applet-src/io/jcrpc/counter/example", name)
		if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
			t.Fatal(e)
		}
		writeTestFile(t, p, b)
	}
	// import/export syntax follows pinned ant-javacard 3605dc35 README.
	xml := fmt.Sprintf(`<project name="counter-writer-cap" default="cap">
 <taskdef name="javacard" classname="pro.javacard.ant.JavaCard" classpath="%s"/>
 <target name="library"><javacard>
  <cap jckit="%s" sources="%s/library-src" package="counter" aid="F000000103" version="1.0" ints="true" verify="true" export="%s/exports" jar="%s/exports/counter.jar" output="%s/counter-library.cap"/>
 </javacard></target>
 <target name="applet"><javacard>
  <cap jckit="%s" sources="%s/applet-src" package="io.jcrpc.counter.example" aid="F00000010102" version="1.0" ints="true" verify="true" output="%s/counter-applet.cap">
   <import exps="%s/exports" jar="%s/exports/counter.jar"/>
   <applet class="io.jcrpc.counter.example.CounterJCApplet" aid="F000000101"/>
  </cap>
 </javacard></target>
 <target name="cap" depends="library,applet"/>
</project>`, filepath.ToSlash(task), filepath.ToSlash(kit), root, root, root, root, filepath.ToSlash(kit), root, root, root, root)
	build := filepath.Join(root, "build.xml")
	writeTestFile(t, build, []byte(xml))
	run := func(target string) ([]byte, error) {
		if target == "applet" {
			// Use the SDK's java.lang subset instead of the host boot classes.
			// Ant's normal host javac accepts Math, then this converter crashes.
			// Override is a source-only host annotation absent from this SDK.
			// Erase only that annotation for the API compile; Ant converts the
			// original production source, including its original annotations.
			var sources []string
			for _, name := range []string{"CounterApplet.java", "CounterJCApplet.java"} {
				b, e := os.ReadFile(filepath.Join(root, "applet-src/io/jcrpc/counter/example", name))
				if e != nil {
					return nil, e
				}
				p := filepath.Join(root, "sdk-src", name)
				if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
					return nil, e
				}
				writeTestFile(t, p, []byte(strings.ReplaceAll(string(b), "@Override", "")))
				sources = append(sources, p)
			}
			javac := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "javac")
			args := []string{"-source", "7", "-target", "7",
				"-bootclasspath", filepath.Join(kit, "lib", "api_classic.jar"),
				"-classpath", filepath.Join(root, "exports", "counter.jar"),
				"-d", filepath.Join(root, "sdk-classes")}
			cmd := exec.Command(javac, append(args, sources...)...)
			b, e := cmd.CombinedOutput()
			code := -1
			if cmd.ProcessState != nil {
				code = cmd.ProcessState.ExitCode()
			}
			t.Logf("exact SDK javac real exit %d\n%s", code, b)
			if e != nil {
				return b, e
			}
		}
		cmd := exec.Command(ant, "-f", build, target)
		cmd.Dir = root
		b, e := cmd.CombinedOutput()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		t.Logf("ant %s real exit %d\n%s", target, code, b)
		return b, e
	}
	t.Run("real-library-and-wrapper", func(t *testing.T) {
		if b, e := run("cap"); e != nil {
			t.Fatalf("Counter Classic compile/convert/verify: %v\n%s", e, b)
		}
		for _, name := range []string{"counter-library.cap", "counter-applet.cap"} {
			path := filepath.Join(root, name)
			b, e := os.ReadFile(path)
			if e != nil || len(b) == 0 {
				t.Fatalf("missing CAP %s: %v", name, e)
			}
			z, e := zip.OpenReader(path)
			if e != nil {
				t.Fatal(e)
			}
			components, method := uint64(0), uint64(0)
			for _, f := range z.File {
				if strings.HasSuffix(f.Name, ".cap") {
					components += f.UncompressedSize64
				}
				if strings.HasSuffix(f.Name, "/Method.cap") {
					method = f.UncompressedSize64
				}
			}
			z.Close()
			if components == 0 || method == 0 {
				t.Fatal("CAP components missing")
			}
			t.Logf("%s SHA256=%x bytes=%d components=%d Method.cap=%d", name, sha256.Sum256(b), len(b), components, method)
			if out := os.Getenv("JCRPC_COUNTER_CAP_OUT"); out != "" {
				if e = os.MkdirAll(out, 0755); e != nil {
					t.Fatal(e)
				}
				writeTestFile(t, filepath.Join(out, name), b)
			}
		}
	})
	if t.Failed() {
		return
	}
	// A valid SDK-only compile/convert is required before any negative plant.
	if output, e := run("applet"); e != nil {
		t.Fatalf("exact SDK positive control: %v\n%s", e, output)
	}
	t.Run("host-Math-refused", func(t *testing.T) {
		p := filepath.Join(root, "applet-src/io/jcrpc/counter/example/CounterJCApplet.java")
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		before := "buffer.length > 255 ? (short) 255 : (short) buffer.length"
		if strings.Count(string(b), before) != 1 {
			t.Fatal("Math plant not applied")
		}
		writeTestFile(t, p, []byte(strings.Replace(string(b), before, "(short) Math.min(buffer.length, 255)", 1)))
		output, e := run("applet")
		if e == nil || !strings.Contains(string(output), "Math") || (!strings.Contains(string(output), "cannot find symbol") && !strings.Contains(string(output), "not found in export")) {
			t.Fatalf("Classic gate admitted host Math or failed elsewhere: %v\n%s", e, output)
		}
		// Restore exact wrapper bytes and require converter/verifier success;
		// the control must not leave the gate validating only a bad fixture.
		writeTestFile(t, p, b)
		if output, e := run("applet"); e != nil {
			t.Fatalf("restored Counter Classic conversion: %v\n%s", e, output)
		}
	})
	// Keep a converter-specific control separate from SDK API admission:
	// javac accepts this builder, but Classic clinit cannot invoke it.
	t.Run("clinit-builder-converter-refused", func(t *testing.T) {
		p := filepath.Join(root, "applet-src/io/jcrpc/counter/example/CounterApplet.java")
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		before := "private final byte[] mockSpki;"
		assignment := "        mockSpki = buildMockSpki();"
		if strings.Count(string(b), before) != 1 || strings.Count(string(b), assignment) != 1 {
			t.Fatal("clinit builder plant not applied")
		}
		mutated := strings.Replace(string(b), before, "private static final byte[] mockSpki = buildMockSpki();", 1)
		mutated = strings.Replace(mutated, assignment, "", 1)
		writeTestFile(t, p, []byte(mutated))
		output, e := run("applet")
		if e == nil || !strings.Contains(string(output), "CounterApplet: unsupported bytecode invokestatic in clinit method") {
			t.Fatalf("Classic converter admitted clinit builder or failed elsewhere: %v\n%s", e, output)
		}
		writeTestFile(t, p, b)
		if output, e := run("applet"); e != nil {
			t.Fatalf("restored clinit builder conversion: %v\n%s", e, output)
		}
	})
}
