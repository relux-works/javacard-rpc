package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Security audit S-01 (javacard-rpc, plan row T-20): the generated short-dispatch
// default branch and every helper error path must not allocate per call. On a
// real Java Card the heap is never reclaimed, so an unauthenticated reader that
// loops unknown-INS frames exhausts memory until the applet answers 6A84.
//
// This test is the static half: it proves that no `throw new` survives anywhere
// in the generated skeleton and that the dispatch entry point has no `new` at
// all. The behavioral half is TestGeneratedJavaSkeletonUnknownInsReusesOneException.
func TestGeneratedJavaSkeletonHasNoAllocatingThrowOnDispatchPaths(t *testing.T) {
	for _, tc := range []struct {
		schema  string
		pkg     string
		methods []string
	}{
		{"counter.toml", "io.jcrpc.counter.server", []string{"dispatch"}},
		{"stream.toml", "io.jcrpc.streamdemo.server", []string{"dispatch", "dispatchStreamTo", "execute"}},
	} {
		s, err := ParseFile(filepath.Join("testdata", tc.schema))
		if err != nil {
			t.Fatalf("%s: ParseFile returned error: %v", tc.schema, err)
		}
		result, err := GenerateJavaSkeleton(s, tc.pkg)
		if err != nil {
			t.Fatalf("%s: GenerateJavaSkeleton returned error: %v", tc.schema, err)
		}
		skeleton := string(result.SkeletonSource)

		// Every error path in the skeleton (dispatch default, pack*/read*/slice
		// guards, fixed-length response checks) must go through the one
		// preconstructed exception. `throw new` anywhere is a per-call allocation.
		if idx := strings.Index(skeleton, "throw new "); idx >= 0 {
			t.Fatalf("%s: generated skeleton allocates an exception per call:\n%s", tc.schema, excerpt(skeleton, idx))
		}
		// Exactly one construction of the reusable exception is allowed and it
		// must live in the constructor, not on any command path.
		if got := strings.Count(skeleton, "new StatusWordException("); got != 1 {
			t.Fatalf("%s: expected exactly one preconstructed StatusWordException, found %d", tc.schema, got)
		}
		for _, method := range tc.methods {
			body := javaMethodBody(t, skeleton, method)
			if strings.Contains(body, "new ") {
				t.Fatalf("%s: %s(...) allocates on the command path:\n%s", tc.schema, method, body)
			}
		}
	}
}

// Behavioral half of S-01, driven through the production entry point
// `dispatch(ins, p1, p2, data)` of the generated skeleton: N unknown-INS frames
// yield N throws of the SAME exception object (zero new objects), the status
// word stays 6D00, a valid request still succeeds afterwards, and the helper
// error paths (wrong request length, wrong fixed response length) reuse that
// same instance. Positive control: a developer-side `new StatusWordException`
// is a distinct object, so the identity check is discriminating.
func TestGeneratedJavaSkeletonUnknownInsReusesOneException(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not available")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not available")
	}

	s, err := ParseFile(filepath.Join("testdata", "counter.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	result, err := GenerateJavaSkeleton(s, "io.jcrpc.counter.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	root := t.TempDir()
	packageDir := filepath.Join(root, "io", "jcrpc", "counter", "server")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeTestFile(t, filepath.Join(packageDir, result.TransportName+".java"), result.TransportSource)
	writeTestFile(t, filepath.Join(packageDir, result.SkeletonName+".java"), result.SkeletonSource)
	harness, err := os.ReadFile(filepath.Join("testdata", "DispatchAllocationHarness.java"))
	if err != nil {
		t.Fatalf("read harness: %v", err)
	}
	writeTestFile(t, filepath.Join(packageDir, "DispatchAllocationHarness.java"), harness)

	compile := exec.Command(javac, "-d", root,
		filepath.Join(packageDir, result.TransportName+".java"),
		filepath.Join(packageDir, result.SkeletonName+".java"),
		filepath.Join(packageDir, "DispatchAllocationHarness.java"),
	)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("javac failed: %v\n%s", err, output)
	}
	run := exec.Command(java, "-cp", root, "io.jcrpc.counter.server.DispatchAllocationHarness")
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("dispatch allocation harness failed: %v\n%s", err, output)
	}
}

// Security audit S-06 (javacard-rpc, plan row T-20): the generated bounded
// stream runtime must keep its state machine in CLEAR_ON_DESELECT transient
// arrays, not in persistent instance fields. Every WRITE chunk, every result
// and every abort used to rewrite ~17 EEPROM scalars; on a physical card that
// is an endurance sink and a persistent-state leak across deselect.
//
// Static proof on the generated source: the runtime declares no mutable
// instance field (every field is `private final` or `private static final`),
// the skeleton hands it `JCSystem.makeTransientShortArray(..., CLEAR_ON_DESELECT)`
// and `makeTransientObjectArray(..., CLEAR_ON_DESELECT)`, and the legacy reset
// marker is gone. jCardSim cannot measure EEPROM writes, so this is the bound
// the generator test can state; endurance measurement is the hardware lane.
func TestGeneratedJavaStreamRuntimeKeepsStateInTransientArrays(t *testing.T) {
	s, err := ParseFile(filepath.Join("testdata", "stream.toml"))
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	result, err := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
	if err != nil {
		t.Fatalf("GenerateJavaSkeleton returned error: %v", err)
	}

	runtime := string(result.StreamRuntimeSource)
	classBody := runtime[strings.Index(runtime, "public final class "):]
	// Field declarations are the `    private ...;` lines without a `(`;
	// anything that is neither `static final` nor `final` is a mutable
	// persistent scalar (or object reference) and fails the S-06 bound.
	fieldLine := regexp.MustCompile(`(?m)^    private [^(\n]*;$`)
	for _, line := range fieldLine.FindAllString(classBody, -1) {
		if strings.HasPrefix(line, "    private static final ") || strings.HasPrefix(line, "    private final ") {
			continue
		}
		t.Fatalf("generated stream runtime keeps a mutable persistent field:\n%s", line)
	}
	for _, fragment := range []string{
		"short[] scalars",
		"Object[] handlerSlot",
	} {
		if !strings.Contains(runtime, fragment) {
			t.Fatalf("generated stream runtime constructor does not take %q:\n%s", fragment, runtime)
		}
	}
	if strings.Contains(runtime, "resetMarker") {
		t.Fatalf("generated stream runtime still carries the reset-marker trick that only persistent scalars needed")
	}

	skeleton := string(result.SkeletonSource)
	for _, fragment := range []string{
		"JCSystem.makeTransientShortArray(",
		"JCSystem.makeTransientObjectArray(",
		"STREAM_SCALAR_COUNT, JCSystem.CLEAR_ON_DESELECT)",
		"STREAM_HANDLER_SLOT_COUNT, JCSystem.CLEAR_ON_DESELECT)",
	} {
		if !strings.Contains(skeleton, fragment) {
			t.Fatalf("generated skeleton does not inject %q:\n%s", fragment, skeleton)
		}
	}
	if strings.Contains(skeleton, "STREAM_RESET_MARKER_LENGTH") {
		t.Fatalf("generated skeleton still allocates the reset marker")
	}
}

// javaMethodBody returns the brace-balanced body of the first Java method
// named `name` in `source` (the text between its opening `{` and matching `}`).
func javaMethodBody(t *testing.T, source, name string) string {
	t.Helper()
	sig := regexp.MustCompile(`(?m)^    (public|protected|private)[^\n]*\b` + regexp.QuoteMeta(name) + `\(`)
	loc := sig.FindStringIndex(source)
	if loc == nil {
		t.Fatalf("method %s not found in generated source", name)
	}
	open := strings.Index(source[loc[0]:], "{")
	if open < 0 {
		t.Fatalf("method %s has no body", name)
	}
	start := loc[0] + open
	depth := 0
	for i := start; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : i+1]
			}
		}
	}
	t.Fatalf("method %s body is not brace-balanced", name)
	return ""
}

func excerpt(source string, idx int) string {
	start := idx - 200
	if start < 0 {
		start = 0
	}
	end := idx + 200
	if end > len(source) {
		end = len(source)
	}
	return source[start:end]
}
