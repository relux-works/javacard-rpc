package backend

import (
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

// The per-method stream mode names are string constants usable in a separate
// module, and the model stores any value verbatim, including empty and unknown
// values. Validation belongs to the facade and the target backend.
func TestStreamModeMetadataRoundTrip(t *testing.T) {
	const public string = pluginapi.StreamModePublicInvalidate
	const chunked string = pluginapi.StreamModeChunked
	if public != "public-invalidate" || chunked != "chunked" {
		t.Fatalf("stream mode names changed: %q / %q", public, chunked)
	}
	for _, mode := range []string{"", public, chunked, "unknown"} {
		t.Run(mode, func(t *testing.T) {
			method := &pluginapi.Method{Name: "sign", INS: 0x20, StreamMode: mode}
			schema := pluginapi.Schema{Methods: map[string]*pluginapi.Method{"sign": method}}
			var selector string = schema.Methods["sign"].StreamMode
			if selector != mode {
				t.Fatalf("metadata altered: %+v", schema.Methods["sign"])
			}
		})
	}
}
