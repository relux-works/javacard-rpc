package backend

import (
	"github.com/relux-works/javacard-rpc/pluginapi"
	"testing"
)

// The separate module implements the full options/model/package-file contract,
// with no host parsing or facade imports; a missing schema returns no files.
func TestIndependentBackend(t *testing.T) {
	p := Backend{}
	s := &pluginapi.Schema{Applet: pluginapi.Applet{Version: "1.2", StreamWorkspace: "persistent"}}
	files, err := p.Generate(s, pluginapi.Options{Namespace: "probe", StreamMemory: "clear_on_reset", SimulatorDependency: "works.relux:jcardsim:3.0.5.9-relux.1"})
	if err != nil || len(files) != 2 || files[0].Name != "build.manifest" || string(files[0].Data) != "1.2" || files[1].Name != "src/backend.txt" || string(files[1].Data) != "probe\nclear_on_reset\nworks.relux:jcardsim:3.0.5.9-relux.1\npersistent" {
		t.Fatalf("contract: %v %v", files, err)
	}
	files, err = p.Generate(nil, pluginapi.Options{})
	if err == nil || len(files) != 0 {
		t.Fatalf("missing schema: %v %v", files, err)
	}
}
