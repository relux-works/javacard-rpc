package pluginapi_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Go's resolver/compiler runs an API-only module outside the repository with
// network and workspaces disabled. It retains a copy of the existing backend fixture and
// exercises the new metadata without adding dependencies to the API.
func TestCleanupAPIIndependentConsumer(t *testing.T) {
	apiDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"go.mod", "backend.go", "backend_test.go", "cleanup_test.go"} {
		source := filepath.Join(apiDir, "testdata", "external-consumer", name)
		if name == "cleanup_test.go" {
			source = filepath.Join(apiDir, "testdata", name)
		}
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
		t.Logf("go %v: exit 0\n%s", args, out)
		return out
	}
	const api = "github.com/relux-works/javacard-rpc/pluginapi"
	run("mod", "edit", "-replace="+api+"="+apiDir)
	modules := strings.Fields(string(run("list", "-mod=readonly", "-m", "-f", "{{.Path}}", "all")))
	if len(modules) != 2 || modules[0] != "example.com/independent-backend" || modules[1] != api {
		t.Fatalf("consumer must require only the API: %v", modules)
	}
	deps := strings.Fields(string(run("list", "-mod=readonly", "-deps", api)))
	if len(deps) != 1 || deps[0] != api {
		t.Fatalf("API must have no compiled dependencies: %v", deps)
	}
	run("build", "-mod=readonly", "./...")
	run("test", "-mod=readonly", "-count=1", "-v", "./...")
}
