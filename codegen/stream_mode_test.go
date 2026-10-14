package codegen

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

const streamModeSchemaTOML = `[applet]
name = "Rev4Shape"
version = "1.0.0"
aid = "F00000010401"
cla = 0xB0
stream_workspace = "persistent"
stream_workspace_cleanup = "authenticated-invalidate"

[methods.getVersion]
ins = 0x01
[methods.getVersion.response]
fields = [{ name = "version", type = "u8" }]

[methods.signTranscript]
ins = 0x20
stream_mode = "chunked"
[methods.signTranscript.request]
fields = [{ name = "transcript", type = "stream", max_length = 2048, chunk_size = 192 }]
[methods.signTranscript.response]
fields = [{ name = "signatureLength", type = "u8" }, { name = "signature", type = "bytes[72]" }]

[methods.putLeaf]
ins = 0x58
[methods.putLeaf.request]
fields = [{ name = "leaf", type = "stream", max_length = 2048, chunk_size = 192 }]
[methods.putLeaf.response]
fields = [{ name = "generation", type = "u32" }, { name = "digest", type = "bytes[32]" }]

[methods.signAdmission]
ins = 0x70
stream_mode = "public-invalidate"
[methods.signAdmission.request]
fields = [{ name = "transcript", type = "stream", max_length = 320, chunk_size = 192 }]
[methods.signAdmission.response]
fields = [{ name = "proof", type = "stream", max_length = 1408, chunk_size = 192 }]
`

// Parse transports exact per-method mode bytes without fixing spellings; an
// omitted key stays empty. Semantic validation, not parsing, rejects values.
func TestParseStreamMode(t *testing.T) {
	for _, mode := range []string{pluginapi.StreamModeChunked, pluginapi.StreamModePublicInvalidate, "bogus", " chunked", "Chunked"} {
		t.Run(mode, func(t *testing.T) {
			input := strings.Replace(streamModeSchemaTOML, "stream_mode = \"chunked\"", "stream_mode = \""+mode+"\"", 1)
			s := mustParseSchema(t, input)
			if s.Methods["signTranscript"].StreamMode != mode {
				t.Fatalf("stream mode metadata lost: %q; want %q", s.Methods["signTranscript"].StreamMode, mode)
			}
			if s.Methods["putLeaf"].StreamMode != "" || s.Methods["signAdmission"].StreamMode != pluginapi.StreamModePublicInvalidate {
				t.Fatalf("other methods changed: %q %q", s.Methods["putLeaf"].StreamMode, s.Methods["signAdmission"].StreamMode)
			}
		})
	}
}

// Every misplaced or unknown mode is refused at its method path; the fixture
// with chunked, authenticated and public methods is the valid control, so a
// validator that refuses everything fails. Programmatic callers get the same
// verdict as parsed ones.
func TestValidateStreamMode(t *testing.T) {
	if errs := Validate(mustParseSchema(t, streamModeSchemaTOML)); len(errs) != 0 {
		t.Fatalf("valid stream modes refused: %v", errs)
	}
	for _, tc := range []struct {
		name, from, to, path, message string
	}{
		{"unknown", `stream_mode = "chunked"`, `stream_mode = "Chunked"`, "methods.signTranscript.stream_mode", "must be public-invalidate or chunked"},
		{"padded", `stream_mode = "chunked"`, `stream_mode = "chunked "`, "methods.signTranscript.stream_mode", "must be public-invalidate or chunked"},
		{"ordinary", "ins = 0x01\n", "ins = 0x01\nstream_mode = \"chunked\"\n", "methods.getVersion.stream_mode", "applies only to a streamed method"},
		{"whole-reply-area", `stream_workspace_cleanup = "authenticated-invalidate"`, `stream_workspace_cleanup = "whole-reply-area"`, "methods.signTranscript.stream_mode", "requires stream_workspace = persistent and stream_workspace_cleanup = authenticated-invalidate"},
		{"released-default", "stream_workspace_cleanup = \"authenticated-invalidate\"\n", "", "methods.signAdmission.stream_mode", "requires stream_workspace = persistent and stream_workspace_cleanup = authenticated-invalidate"},
		{"chunked-streamed-result", `stream_mode = "public-invalidate"`, `stream_mode = "chunked"`, "methods.signAdmission.stream_mode", "chunked supports short results only"},
		{"authenticated-streamed-result", "stream_mode = \"public-invalidate\"\n", "", "methods.signAdmission.response", "stream response is not allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := strings.Replace(streamModeSchemaTOML, tc.from, tc.to, 1)
			if input == streamModeSchemaTOML {
				t.Fatal("fixture edit not applied")
			}
			requireValidationError(t, Validate(mustParseSchema(t, input)), tc.path, tc.message)
		})
	}
	// A public method needs a request stream: a response-only public method is refused.
	s := mustParseSchema(t, streamModeSchemaTOML)
	s.Methods["signAdmission"].Request = nil
	requireValidationError(t, Validate(s), "methods.signAdmission.stream_mode", "requires a request stream")
}

// Each facade stream-mode check keeps its guard and admits exactly one member
// of the class it must refuse; jcrpc-mutants requires the named assertion
// after the unmodified test passes, and refuses setup/compile failures as kills.
func TestStreamModeNarrowingMutants(t *testing.T) {
	for _, name := range []string{"mode-parser-drops-chunked", "mode-validator-unknown-bogus", "mode-validator-chunked-0x70", "mode-validator-whole-reply", "cleanup-auth-response-stream"} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("go", "run", "./cmd/jcrpc-mutants", "--repo", "..", "--out", filepath.Join(t.TempDir(), "mutants"), "--only", name)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "control-exit=0 exit=1 intended-assertion=true") {
				t.Fatalf("stream mode mutant did not reach named assertion: %v\n%s", err, output)
			}
			t.Logf("%s", output)
		})
	}
}
