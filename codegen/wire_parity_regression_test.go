package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/codegen/internal/wirecompat"
)

// The reviewer's exact token-preserving INS03->7E plant must compile and fail
// the wire probe at GET03/6D00. The unplanted real CLI corpus is the paired
// positive; expected wire is grounded in IDL and the immutable v0.5.0 corpus.
func TestCLIWireParityRejectsReviewerINS03Mutation(t *testing.T) {
	runCLIWirePlant(t, "testdata/counter.toml", "CounterSkeleton.java", `(INS_GET\s*= \(byte\) )0x03;`, `${1}0x7E;`, "ordinary INS03 status 6d00 want 9000")
}

// Each adjacent plant changes exactly one wire contract while retaining the
// API-bearing source and its tokens. Both compiled outputs run the SAME probe;
// a javac failure, arbitrary crash or different assertion is never a kill.
func TestCLIWireParityRejectsAdjacentJavaDrift(t *testing.T) {
	for _, m := range []struct{ name, input, file, before, after, diagnostic string }{
		{"CLA", "testdata/counter.toml", "CounterSkeleton.java", `(CLA_COUNTER = \(byte\) )0xB0;`, `${1}0xB4;`, "CLA constant"},
		{"length-SW", "testdata/counter.toml", "CounterSkeleton.java", `(SW_WRONG_LENGTH = \(short\) )0x6700;`, `${1}0x6701;`, "wrong request length status"},
		{"GET-width", "testdata/counter.toml", "CounterSkeleton.java", `(?s)(private short handleGet\(.*?return \(short\) )2;`, `${1}1;`, "ordinary INS03 response length"},
		{"GET-extra-byte-admitted", "testdata/counter.toml", "CounterSkeleton.java", `(?s)(private short handleGet\(.*?if \()(requestLength != 0)(\))`, `${1}${2} && requestLength != 1${3}`, "wrong request length admitted"},
		{"runtime-descriptor-width", "testdata/stream.toml", "StreamDemoBoundedStreamRuntime.java", `return DESCRIPTOR_LENGTH;`, `return (short)(DESCRIPTOR_LENGTH - 1);`, "stream invoke response length"},
		{"endpoint-abort-opcode", "testdata/stream.toml", "StreamDemoStreamEndpoint.java", `(OP_ABORT\s*= \(byte\) )0x05;`, `${1}0x06;`, "stream abort status 6985"},
		{"adapter-send-width", "testdata/stream.toml", "StreamDemoStreamAPDUAdapter.java", `apdu.sendBytesLong\(ioScratch, \(short\) 0, outcome\);`, `apdu.sendBytesLong(ioScratch, (short) 0, (short)(outcome - 1));`, "stream invoke response length"},
	} {
		t.Run(m.name, func(t *testing.T) { runCLIWirePlant(t, m.input, m.file, m.before, m.after, m.diagnostic) })
	}
}

func runCLIWirePlant(t *testing.T, input, file, before, after, diagnostic string) {
	t.Helper()
	baseline := os.Getenv("JCRPC_BASELINE_GEN")
	if baseline == "" {
		t.Skip("set JCRPC_BASELINE_GEN to immutable facade v0.5.0 CLI")
	}
	candidate := os.Getenv("JCRPC_CANDIDATE_GEN")
	if candidate == "" {
		candidate = filepath.Join(t.TempDir(), "jcrpc-gen")
		cmd := exec.Command("go", "build", "-o", candidate, "./cmd/jcrpc-gen")
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("candidate build: %v\n%s", e, b)
		}
	}
	old, now := t.TempDir(), t.TempDir()
	for _, tc := range []struct{ bin, out string }{{baseline, old}, {candidate, now}} {
		cmd := exec.Command(tc.bin, "--java", "wireprobe", "--out-dir", tc.out, input)
		b, e := cmd.CombinedOutput()
		t.Logf("production jcrpc-gen %q exit=%d\n%s", cmd.Args, cmd.ProcessState.ExitCode(), b)
		if e != nil {
			t.Fatalf("generation: %v\n%s", e, b)
		}
	}
	schema, e := ParseFile(input)
	if e != nil {
		t.Fatal(e)
	}
	if e := wirecompat.Compare(t, old, now, schema); e != nil {
		t.Fatalf("valid control: %v", e)
	}
	var path string
	e = filepath.WalkDir(now, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() && filepath.Base(p) == file {
			path = p
		}
		return nil
	})
	if e != nil || path == "" {
		t.Fatalf("plant path: %s %v", file, e)
	}
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	re := regexp.MustCompile(before)
	if len(re.FindAll(original, -1)) != 1 {
		t.Fatalf("plant must be unique: %s", before)
	}
	planted := re.ReplaceAll(original, []byte(after))
	if strings.Contains(string(original), "callerWorkspace") && !strings.Contains(string(planted), "callerWorkspace") {
		t.Fatal("plant removed API tokens")
	}
	if e := os.WriteFile(path, planted, 0644); e != nil {
		t.Fatal(e)
	}
	// Restore the exact emitted bytes even if the intended refusal is missing.
	defer func() {
		if e := os.WriteFile(path, original, 0644); e != nil {
			t.Error(e)
		}
	}()
	_, e = wirecompat.Observe(t, now, schema)
	if e == nil || !strings.Contains(e.Error(), "behavior: exit status 1") || !strings.Contains(e.Error(), "AssertionError: wire contract: "+diagnostic) {
		t.Fatalf("wire plant survived or failed for unrelated reason: %v; want %s", e, diagnostic)
	}
	t.Logf("named wire probe killed token-preserving plant: %s; javac=0, java=1", diagnostic)
	if e := os.WriteFile(path, original, 0644); e != nil {
		t.Fatal(e)
	}
	if e := wirecompat.Compare(t, old, now, schema); e != nil {
		t.Fatalf("restored positive: %v", e)
	}
}
