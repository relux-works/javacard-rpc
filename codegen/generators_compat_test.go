package codegen

import "testing"

// Ordinary compatibility results keep stream metadata names even when no stream
// source is emitted; streamed results expose the backend's complete sources.
func TestJavaCompatibilityMetadata(t *testing.T) {
	for _, name := range []string{"counter", "stream"} {
		s, e := ParseFile("testdata/" + name + ".toml")
		if e != nil {
			t.Fatal(e)
		}
		r, e := GenerateJavaSkeleton(s, "probe")
		if e != nil {
			t.Fatal(e)
		}
		stem := s.Applet.Name
		if r.StreamEndpointName != stem+"StreamEndpoint" || r.StreamRuntimeName != stem+"BoundedStreamRuntime" || r.StreamAPDUAdapterName != stem+"StreamAPDUAdapter" {
			t.Fatalf("compatibility metadata drift: %+v", r)
		}
		if (len(r.StreamRuntimeSource) > 0) != (name == "stream") {
			t.Fatalf("stream source presence drift: %s", name)
		}
	}
}
