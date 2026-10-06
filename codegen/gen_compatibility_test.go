package codegen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Invoke both actual CLIs and compare every file in each output package. The
// only default exception is the precisely scoped adapter CLA predicate block.
// Opt-in changes only Java skeleton/runtime; Kotlin and ordinary Swift output
// remain byte-identical, and streamed Swift retains its existing refusal.
func TestCLIOutputCompatibility(t *testing.T) {
	baseline := os.Getenv("JCRPC_BASELINE_GEN")
	candidate := os.Getenv("JCRPC_CANDIDATE_GEN")
	if baseline == "" || candidate == "" {
		t.Skip("set JCRPC_BASELINE_GEN and JCRPC_CANDIDATE_GEN for baseline byte comparison")
	}
	inputs := []string{"testdata/counter.toml", "testdata/stream.toml"}
	if idl := os.Getenv("JCRPC_ALLOCATION_IDL"); idl != "" {
		inputs = append(inputs, idl)
	}
	for _, input := range inputs {
		t.Run(filepath.Base(input), func(t *testing.T) {
			old, now := t.TempDir(), t.TempDir()
			args := []string{"--java", "io.jcrpc.compat", "--kotlin", "io.jcrpc.compat"}
			if strings.HasSuffix(input, "counter.toml") {
				args = append(args, "--swift", "CounterClient")
			}
			generate := func(bin, out, schema string) {
				t.Helper()
				argv := append(append([]string{}, args...), "--out-dir", out, schema)
				if b, e := exec.Command(bin, argv...).CombinedOutput(); e != nil {
					t.Fatalf("generate %s: %v\n%s", bin, e, b)
				}
			}
			generate(baseline, old, input)
			generate(candidate, now, input)
			compare := func(left, right string, optin bool) {
				t.Helper()
				count := 0
				e := filepath.WalkDir(left, func(path string, d os.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if d.IsDir() {
						return nil
					}
					rel, e := filepath.Rel(left, path)
					if e != nil {
						return e
					}
					a, e := os.ReadFile(path)
					if e != nil {
						return e
					}
					b, e := os.ReadFile(filepath.Join(right, rel))
					if e != nil {
						return e
					}
					count++
					if optin && (strings.HasSuffix(rel, "Skeleton.java") || strings.HasSuffix(rel, "BoundedStreamRuntime.java")) {
						return nil
					}
					if !optin && strings.HasSuffix(rel, "StreamAPDUAdapter.java") {
						startA := bytes.Index(a, []byte("        if (apduBuffer[ISO7816.OFFSET_CLA]"))
						startB := bytes.Index(b, []byte("        byte cla = apduBuffer[ISO7816.OFFSET_CLA]"))
						if startA < 0 || startB < 0 {
							t.Fatal("CLA exception start boundaries absent")
						}
						endA := bytes.Index(a[startA:], []byte("        try {")) + startA
						endB := bytes.Index(b[startB:], []byte("        try {")) + startB
						if startA < 0 || startB < 0 || endA < startA || endB < startB {
							t.Fatal("CLA exception boundaries absent")
						}
						a = append(append([]byte{}, a[:startA]...), a[endA:]...)
						b = append(append([]byte{}, b[:startB]...), b[endB:]...)
					}
					if !bytes.Equal(a, b) {
						t.Errorf("unexpected byte delta: %s", rel)
					}
					return nil
				})
				if e != nil {
					t.Fatal(e)
				}
				rightCount := 0
				_ = filepath.WalkDir(right, func(_ string, d os.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if !d.IsDir() {
						rightCount++
					}
					return nil
				})
				if rightCount != count {
					t.Fatalf("file inventory delta: %d vs %d", count, rightCount)
				}
				t.Logf("compared %d generated files, optin=%t", count, optin)
			}
			compare(old, now, false)
			if !strings.HasSuffix(input, "counter.toml") {
				raw, e := os.ReadFile(input)
				if e != nil {
					t.Fatal(e)
				}
				persistentInput := filepath.Join(t.TempDir(), "persistent.toml")
				raw = bytes.Replace(raw, []byte("[applet]"), []byte("[applet]\nstream_workspace = \"persistent\""), 1)
				if e := os.WriteFile(persistentInput, raw, 0644); e != nil {
					t.Fatal(e)
				}
				persistent := t.TempDir()
				generate(candidate, persistent, persistentInput)
				compare(now, persistent, true)
				for _, bin := range []string{baseline, candidate} {
					out := t.TempDir()
					b, e := exec.Command(bin, "--swift", "StreamClient", "--out-dir", out, input).CombinedOutput()
					if e == nil || !strings.Contains(string(b), "stream") {
						t.Fatalf("Swift stream refusal missing: %v %s", e, b)
					}
					entries, e := os.ReadDir(out)
					if e != nil || len(entries) != 0 {
						t.Fatal("Swift refusal wrote partial output")
					}
				}
			}
		})
	}
}

// Inspect all generated command-path bodies, not just dispatch's top level.
// This complements the lifecycle harness's no-new-transient-array assertion;
// digest provider/JCRE allocations are outside the generated-code surface.
func TestPersistentStreamCommandPathsDoNotAllocate(t *testing.T) {
	r, e := GenerateJavaSkeleton(workspaceSchema(t, "persistent"), "io.jcrpc.streamdemo.server")
	if e != nil {
		t.Fatal(e)
	}
	for source, methods := range map[string][]string{
		string(r.StreamAPDUAdapterSource): {"processIfStream", "deselect", "copy", "wipe"},
		string(r.StreamRuntimeSource):     {"dispatch", "abort", "begin", "writeOrInvoke", "closeWrite", "executeOnce", "pendingInfo", "readChunk", "closeRead", "writeDescriptor", "clearAll", "fail", "rejectWithoutClearing", "copy", "wipe", "equalsRange"},
	} {
		for _, method := range methods {
			body := javaMethodBody(t, source, method)
			if strings.Contains(body, "new ") {
				t.Fatalf("allocation in generated %s", method)
			}
		}
	}
	for name, source := range map[string][]byte{"runtime": r.StreamRuntimeSource, "skeleton": r.SkeletonSource, "endpoint": r.StreamEndpointSource, "adapter": r.StreamAPDUAdapterSource} {
		assertNoMutablePrivateFields(t, name, string(source))
	}
}
