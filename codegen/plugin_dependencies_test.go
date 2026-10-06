package codegen

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

// The compiled transitive plugin graph includes the independent API, excludes
// the facade and TOML parser, and remains usable through the old facade aliases.
func TestPluginDependencyBoundary(t *testing.T) {
	for _, target := range []string{"javacard", "kotlin", "swift"} {
		cmd := exec.Command("go", "list", "-deps", "./plugins/"+target)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("plugin graph: %v\n%s", err, out)
		}
		api := false
		for _, path := range strings.Fields(string(out)) {
			if path == "github.com/relux-works/javacard-rpc/codegen" || path == "github.com/BurntSushi/toml" {
				t.Fatalf("%s depends on facade/parsing: %s", target, path)
			}
			api = api || path == "github.com/relux-works/javacard-rpc/pluginapi"
		}
		if !api {
			t.Fatalf("%s lacks independent API", target)
		}
	}
	var old *Schema = &pluginapi.Schema{Methods: map[string]*pluginapi.Method{}}
	var shared *pluginapi.Schema = old
	if shared != old {
		t.Fatal("model identity lost")
	}
}
