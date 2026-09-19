package codegen

import (
	"fmt"
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
	javaCardStub := writeJavaCardJCSystemStub(t, root)
	writeTestFile(t, filepath.Join(packageDir, result.TransportName+".java"), result.TransportSource)
	writeTestFile(t, filepath.Join(packageDir, result.SkeletonName+".java"), result.SkeletonSource)
	harness, err := os.ReadFile(filepath.Join("testdata", "DispatchAllocationHarness.java"))
	if err != nil {
		t.Fatalf("read harness: %v", err)
	}
	writeTestFile(t, filepath.Join(packageDir, "DispatchAllocationHarness.java"), harness)

	compile := exec.Command(javac, "-d", root,
		javaCardStub,
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
// Static proof on the generated source: the runtime and both nested exception
// classes declare no mutable persistent field; reusable exception status lives
// in a CLEAR_ON_RESET short array; the skeleton hands stream state
// JCSystem.makeTransient* arrays with CLEAR_ON_DESELECT; and the legacy reset
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
	assertNoMutablePrivateFields(t, "generated stream runtime", runtime)
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

	endpoint := string(result.StreamEndpointSource)
	assertNoMutablePrivateFields(t, "generated stream endpoint", endpoint)
	assertTransientStatusWordException(t, "generated stream endpoint", endpoint, "StreamStatusWordException")

	skeleton := string(result.SkeletonSource)
	assertNoMutablePrivateFields(t, "generated stream skeleton", skeleton)
	assertTransientStatusWordException(t, "generated stream skeleton", skeleton, "StatusWordException")
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

func assertNoMutablePrivateFields(t *testing.T, label, source string) {
	t.Helper()
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "private ") || !strings.HasSuffix(trimmed, ";") {
			continue
		}
		if strings.Contains(trimmed, "(") {
			continue
		}
		if strings.HasPrefix(trimmed, "private static final ") || strings.HasPrefix(trimmed, "private final ") {
			continue
		}
		t.Fatalf("%s keeps a mutable persistent field: %s", label, trimmed)
	}
}

func assertTransientStatusWordException(t *testing.T, label, source, className string) {
	t.Helper()
	if err := validateTransientStatusWordException(source, className); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

// Test the source gate against the two named narrowing mutants. These fixtures
// prove that an unrelated transient-allocation token and a persistent scalar
// cannot satisfy the nested-class assertion.
func TestGeneratedJavaStatusStorageMutantsAreRejected(t *testing.T) {
	mutants := []struct {
		name   string
		source string
	}{
		{
			name: "status kept in a persistent field",
			source: `public static final class StatusWordException extends RuntimeException {
    private short statusWord;
    public StatusWordException(short statusWord) {
        super();
        this.statusWord = statusWord;
    }
    public short getStatusWord() { return statusWord; }
}`,
		},
		{
			name: "reviewer token-preserving persistent array",
			source: `public static final class StatusWordException extends RuntimeException {
    private final short[] status;
    public StatusWordException(short statusWord) {
        super();
        JCSystem.makeTransientShortArray((short) 1, JCSystem.CLEAR_ON_RESET);
        this.status = new short[1];
    }
    public short getStatusWord() { return status[0]; }
}`,
		},
	}

	for _, mutant := range mutants {
		t.Run(mutant.name, func(t *testing.T) {
			if err := validateTransientStatusWordException(mutant.source, "StatusWordException"); err == nil {
				t.Fatalf("named mutant was admitted by the nested-class transient-storage gate")
			}
		})
	}
}

func validateTransientStatusWordException(source, className string) error {
	classBody, err := javaClassBody(source, className)
	if err != nil {
		return err
	}

	field := regexp.MustCompile(`(?m)^[ \t]*private[ \t]+final[ \t]+short\[\][ \t]+status[ \t]*;[ \t]*$`)
	if !field.MatchString(classBody) {
		return fmt.Errorf("class %s does not keep status in a final short[] field", className)
	}

	constructor := regexp.MustCompile(`(?s)\bpublic\s+` + regexp.QuoteMeta(className) +
		`\s*\(\s*short\s+statusWord\s*\)\s*\{[^{}]*this\.status\s*=\s*` +
		`JCSystem\.makeTransientShortArray\(\s*\(short\)\s*1\s*,\s*JCSystem\.CLEAR_ON_RESET\s*\)\s*;`)
	if !constructor.MatchString(classBody) {
		return fmt.Errorf("class %s does not assign status from its CLEAR_ON_RESET allocation in the constructor", className)
	}

	getter := regexp.MustCompile(`(?s)\bpublic\s+short\s+getStatusWord\s*\(\s*\)\s*\{[^{}]*return\s+status\s*\[\s*0\s*\]\s*;`)
	if !getter.MatchString(classBody) {
		return fmt.Errorf("class %s does not read getStatusWord() from status[0]", className)
	}
	return nil
}

func javaClassBody(source, className string) (string, error) {
	declaration := regexp.MustCompile(`(?m)^[ \t]*((public|protected|private|static|final)[ \t]+)*class[ \t]+` + regexp.QuoteMeta(className) + `\b`)
	loc := declaration.FindStringIndex(source)
	if loc == nil {
		return "", fmt.Errorf("class %s not found", className)
	}

	openRelative := strings.Index(source[loc[0]:], "{")
	if openRelative < 0 {
		return "", fmt.Errorf("class %s has no body", className)
	}
	open := loc[0] + openRelative
	depth := 0
	for index := open; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[open : index+1], nil
			}
		}
	}
	return "", fmt.Errorf("class %s body is not brace-balanced", className)
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
