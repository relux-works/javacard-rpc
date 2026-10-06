package codegen

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This invokes Go's real module resolver/compiler with GOWORK disabled and
// network disabled. The consumer's only non-main module is the candidate API;
// production API compiled imports must be stdlib or the independent API itself.
func TestPluginAPIExternalConsumer(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"go.mod", "backend.go", "backend_test.go"} {
		b, e := os.ReadFile(filepath.Join("testdata", "external-plugin", name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, name), b, 0644); e != nil {
			t.Fatal(e)
		}
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("go %v: %v\n%s", args, e, b)
		}
		return b
	}
	api := "github.com/relux-works/javacard-rpc/pluginapi"
	run("mod", "edit", "-replace="+api+"="+filepath.Join(root, "pluginapi"))
	check := func(b []byte, modules bool) {
		t.Helper()
		d := json.NewDecoder(bytes.NewReader(b))
		apiSeen := false
		for {
			var v struct {
				Path, ImportPath string
				Main, Standard   bool
				Module           *struct {
					Path string
					Main bool
				}
			}
			e := d.Decode(&v)
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(e)
			}
			if modules {
				if v.Main {
					continue
				}
				if v.Path != api {
					t.Fatalf("external module graph contains forbidden dependency %s", v.Path)
				}
				apiSeen = true
			} else {
				if v.Standard {
					continue
				}
				if v.ImportPath == api {
					apiSeen = true
				}
				if v.Module == nil || (!v.Module.Main && v.Module.Path != api) {
					t.Fatalf("external imports contain forbidden dependency %s", v.ImportPath)
				}
			}
		}
		if !apiSeen {
			t.Fatal("independent API missing from graph")
		}
	}
	check(run("list", "-mod=mod", "-m", "-json", "all"), true)
	check(run("list", "-mod=mod", "-deps", "-json", "."), false)
	// The frozen API itself consists only of descriptors and pure model helpers;
	// even stdlib parsing/templates must not be hidden by the consumer's stdlib.
	apiDependencies := strings.Fields(string(run("list", "-mod=readonly", "-deps", api)))
	if len(apiDependencies) != 1 || apiDependencies[0] != api {
		t.Fatalf("API compiled graph contains forbidden dependency: %v", apiDependencies)
	}
	run("build", "-mod=readonly", "./...")
	run("test", "-mod=readonly", "-count=1", "./...")
}
