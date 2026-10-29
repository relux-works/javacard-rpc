package codegen

import (
	"strings"
	"testing"
)

// The facade's released backend exposes the authority-change hook only when a
// schema has public streams. Runtime invalidation behavior is tested by the backend.
func TestReleasedPublicInvalidationSurface(t *testing.T) {
	authenticated := `[applet]
name = "IssuerStore"
version = "1.0.0"
aid = "F00000010402"
cla = 0xB0
stream_workspace = "persistent"
stream_workspace_cleanup = "authenticated-invalidate"
[methods.upload]
ins = 0x10
[methods.upload.request]
fields = [{ name = "data", type = "stream", max_length = 1024, chunk_size = 192 }]
[methods.upload.response]
fields = [{ name = "length", type = "u16" }]
`
	for _, tc := range []struct {
		name, input string
		public      bool
	}{
		{"mixed", streamModeSchemaTOML, true}, {"authenticated-only", authenticated, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := mustParseSchema(t, tc.input)
			if errors := Validate(schema); len(errors) != 0 {
				t.Fatalf("valid schema refused: %v", errors)
			}
			result, err := GenerateJavaSkeleton(schema, "io.jcrpc.release.server")
			if err != nil {
				t.Fatal(err)
			}
			for label, pair := range map[string]struct {
				source   []byte
				fragment string
			}{
				"skeleton":   {result.SkeletonSource, "public final void abortPublicStreams(byte reason)"},
				"delegation": {result.SkeletonSource, "streamSession.abortPublic(reason)"},
				"endpoint":   {result.StreamEndpointSource, "void abortPublic(byte reason)"},
				"runtime":    {result.StreamRuntimeSource, "public void abortPublic(byte reason)"},
			} {
				if got := strings.Contains(string(pair.source), pair.fragment); got != tc.public {
					t.Fatalf("%s public hook=%v, want %v", label, got, tc.public)
				}
			}
			if !strings.Contains(string(result.SkeletonSource), "public final void abortStreams(byte reason)") {
				t.Fatal("full invalidation hook lost")
			}
		})
	}
}
