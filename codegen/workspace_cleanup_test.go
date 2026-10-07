package codegen

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

// Parse transports exact selector bytes into the public API without silently
// fixing invalid spellings. Semantic validation, rather than parsing, rejects them.
func TestParseStreamWorkspaceCleanup(t *testing.T) {
	for _, mode := range []string{"", pluginapi.StreamWorkspaceCleanupWholeReplyArea, pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "bogus", "Written-bytes-only", " written-bytes-only "} {
		t.Run(mode, func(t *testing.T) {
			input := strings.Replace(minimalValidSchemaTOML, "[applet]", fmt.Sprintf("[applet]\nstream_workspace = %q\nstream_workspace_cleanup = %q", "persistent", mode), 1)
			s, err := Parse(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			if s.Applet.StreamWorkspaceCleanup != mode || s.Applet.StreamWorkspace != "persistent" {
				t.Fatalf("cleanup metadata lost: %+v; want %q", s.Applet, mode)
			}
		})
	}
	s := validSchema(t)
	if s.Applet.StreamWorkspaceCleanup != "" {
		t.Fatal("omitted cleanup changed default")
	}
}

// Each narrowing plant must be killed by its named contract assertion after
// that exact test passes unmodified. The harness refuses setup/compile failures
// as kills. Only disposable fixtures replace copied public backend sources.
func TestCleanupNarrowingMutants(t *testing.T) {
	for _, name := range []string{"cleanup-parser-written", "cleanup-validator-bogus", "cleanup-whole-transient", "cleanup-written-persistent", "manifest-api-old"} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("go", "run", "./cmd/jcrpc-mutants", "--repo", "..", "--out", filepath.Join(t.TempDir(), "mutants"), "--only", name)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "control-exit=0 exit=1 intended-assertion=true") {
				t.Fatalf("cleanup mutant did not reach named assertion: %v\n%s", err, output)
			}
			t.Logf("%s", output)
		})
	}
}

// Only whole-reply-area is accepted with persistent storage, even on non-stream schemas;
// an empty selector accepts all three released storage spellings. Programmatic
// API callers receive the same verdict as parsed callers.
func TestValidateStreamWorkspaceCleanup(t *testing.T) {
	for _, storage := range []string{"", "transient", "persistent"} {
		for _, mode := range []string{"", pluginapi.StreamWorkspaceCleanupWholeReplyArea, pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "bogus", "Written-bytes-only", " written-bytes-only "} {
			t.Run(storage+"/"+mode, func(t *testing.T) {
				s := validSchema(t)
				s.Applet.StreamWorkspace = storage
				s.Applet.StreamWorkspaceCleanup = mode
				errs := Validate(s)
				known := mode == pluginapi.StreamWorkspaceCleanupWholeReplyArea
				if mode == "" || known && storage == "persistent" {
					if len(errs) != 0 {
						t.Fatalf("valid cleanup refused: %v", errs)
					}
					return
				}
				message := "must be whole-reply-area"
				if known {
					message = "requires stream_workspace = persistent"
				}
				if len(errs) != 1 || errs[0].Path != "applet.stream_workspace_cleanup" || errs[0].Message != message {
					t.Fatalf("wrong cleanup refusal: %v; want %q", errs, message)
				}
			})
		}
	}
}
