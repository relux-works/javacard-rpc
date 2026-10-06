package codegen

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Each isolated source mutant keeps its policy gate and accepts only the
// additional spelling "ram". Production CLI/direct-generation tests must fail
// by name and contract assertion; build failures cannot count as kills.
func TestWorkspacePolicyNarrowingMutants(t *testing.T) {
	for _, tc := range []struct{ name, file, pkg, test, claim string }{
		{"validator-ram", "validator.go", "./cmd/jcrpc-gen", "TestRunStreamWorkspacePolicy/ram", "refusal: 2 generate java skeleton: unknown stream workspace \"ram\""},
		{"generator-ram", "", ".", "TestStreamWorkspacePolicy", "invalid \"ram\":"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "generator-ram" {
				command := exec.Command("go", "run", "./cmd/jcrpc-mutants", "--repo", "..", "--out", filepath.Join(t.TempDir(), "mutants"), "--only", "workspace-generator-ram")
				output, err := command.CombinedOutput()
				if err != nil || !strings.Contains(string(output), "intended-assertion=true") {
					t.Fatalf("released backend policy mutant: %v\n%s", err, output)
				}
				t.Logf("%s", output)
				return
			}
			fixture := t.TempDir()
			root := filepath.Join(fixture, "codegen")
			if e := os.Mkdir(root, 0755); e != nil {
				t.Fatal(e)
			}
			if e := os.CopyFS(filepath.Join(fixture, "pluginapi"), os.DirFS("../pluginapi")); e != nil {
				t.Fatal(e)
			}
			e := filepath.WalkDir(".", func(path string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.IsDir() {
					if path != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "build") {
						return filepath.SkipDir
					}
					return nil
				}
				if !strings.HasSuffix(path, ".go") && path != "go.mod" && path != "go.sum" && !strings.HasPrefix(path, "testdata"+string(os.PathSeparator)) {
					return nil
				}
				content, e := os.ReadFile(path)
				if e != nil {
					return e
				}
				target := filepath.Join(root, path)
				if e := os.MkdirAll(filepath.Dir(target), 0755); e != nil {
					return e
				}
				return os.WriteFile(target, content, 0644)
			})
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(root, tc.file)
			source, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			before := `s.Applet.StreamWorkspace != "" && s.Applet.StreamWorkspace != "transient"`
			after := `s.Applet.StreamWorkspace != "" && s.Applet.StreamWorkspace != "ram" && s.Applet.StreamWorkspace != "transient"`
			mutant := strings.Replace(string(source), before, after, 1)
			if mutant == string(source) {
				t.Fatal("policy plant not applied")
			}
			if e := os.WriteFile(path, []byte(mutant), 0644); e != nil {
				t.Fatal(e)
			}
			command := exec.Command("go", "test", tc.pkg, "-run", "^"+tc.test+"$", "-count=1", "-v")
			command.Dir = root
			output, e := command.CombinedOutput()
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 1 || !strings.Contains(string(output), "--- FAIL: "+tc.test) || !strings.Contains(string(output), tc.claim) {
				t.Fatalf("policy mutant escaped named assertion: %v\n%s", e, output)
			}
			t.Logf("killed %s by %s (nested go test exit 1): %s", tc.name, tc.test, tc.claim)
		})
	}
}
