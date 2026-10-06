package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/codegen/internal/compat"
)

// Every malformed tuple is driven through the same run used by the public CLI;
// the unchanged released tuple is the positive control. No consumer writes occur.
func TestManifestRefusals(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(repo, "compatibility/runtime-manifest.json")
	if got := run([]string{"--repo", repo, "--manifest", source}); got != 0 {
		t.Fatalf("valid pins refused: %d", got)
	}
	b, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	var original compat.Manifest
	if e = json.Unmarshal(b, &original); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*compat.Manifest){
		"missing-kotlin": func(m *compat.Manifest) { m.Targets = append(m.Targets[:1], m.Targets[2:]...) },
		"duplicate":      func(m *compat.Manifest) { m.Targets[1] = m.Targets[0] },
		"unknown":        func(m *compat.Manifest) { m.Targets[1].Target = "other" },
		"schema":         func(m *compat.Manifest) { m.Schema = 2 },
		"input-drift":    func(m *compat.Manifest) { m.BSimInput.SHA256 = strings.Repeat("0", 64) },
	}
	for _, field := range []string{"repository", "tag", "tag_object", "commit", "go_module", "backend_version", "backend_import", "runtime", "swift_package", "swift_exact_version"} {
		field := field
		cases["wrong-"+field] = func(m *compat.Manifest) {
			raw, _ := json.Marshal(m.Targets[2])
			var p map[string]any
			json.Unmarshal(raw, &p)
			p[field] = "wrong"
			raw, _ = json.Marshal(p)
			json.Unmarshal(raw, &m.Targets[2])
		}
		cases["missing-"+field] = func(m *compat.Manifest) {
			raw, _ := json.Marshal(m.Targets[2])
			var p map[string]any
			json.Unmarshal(raw, &p)
			delete(p, field)
			raw, _ = json.Marshal(p)
			m.Targets[2] = compat.Target{}
			json.Unmarshal(raw, &m.Targets[2])
		}
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var m compat.Manifest
			json.Unmarshal(b, &m)
			change(&m)
			raw, _ := json.Marshal(m)
			p := filepath.Join(t.TempDir(), "manifest.json")
			os.WriteFile(p, raw, 0644)
			if got := run([]string{"--repo", repo, "--manifest", p}); got != 1 {
				t.Fatalf("invalid manifest admitted: %s exit %d", name, got)
			}
		})
	}
	for _, content := range []string{"{", string(b) + " {}", strings.Replace(string(b), `"schema": 1`, `"unknown": 1, "schema": 1`, 1)} {
		p := filepath.Join(t.TempDir(), "bad.json")
		os.WriteFile(p, []byte(content), 0644)
		if run([]string{"--repo", repo, "--manifest", p}) != 1 {
			t.Fatal("unreadable/unknown/trailing manifest admitted")
		}
	}
	if run([]string{"--repo", repo, "--manifest", filepath.Join(t.TempDir(), "absent.json")}) != 1 {
		t.Fatal("missing manifest admitted")
	}
}

// The resolver's real module graph must match pins and must not substitute local
// sources; this fixture copies config only and executes Go's actual resolver.
func TestManifestBackendDisagreement(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"version", "replace"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/bsim-auth-2d23abd.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
				b, e := os.ReadFile(filepath.Join(repo, name))
				if e != nil {
					t.Fatal(e)
				}
				if name == "codegen/go.mod" {
					if kind == "version" {
						// Keep the real public resolver graph unchanged.
					} else {
						b = append(b, []byte("\nreplace github.com/relux-works/javacard-rpc-client-kotlin => "+filepath.Join(repo, "pluginapi")+"\n")...)
					}
				}
				if e = write(filepath.Join(root, name), string(b)); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "version" {
				manifestPath := filepath.Join(root, "compatibility/runtime-manifest.json")
				raw, _ := os.ReadFile(manifestPath)
				var m compat.Manifest
				json.Unmarshal(raw, &m)
				m.Targets[1].BackendVersion = "v0.2.0"
				m.Targets[1].Tag = "v0.2.0"
				raw, _ = json.Marshal(m)
				os.WriteFile(manifestPath, raw, 0644)
				raw, _ = json.Marshal(m.Targets[1])
				os.WriteFile(filepath.Join(root, "compatibility/releases/kotlin.json"), raw, 0644)
			}
			_, e := compat.Check(root, filepath.Join(root, "compatibility/runtime-manifest.json"))
			if e == nil || !strings.Contains(e.Error(), "backend module disagreement: kotlin") {
				t.Fatalf("wrong module verdict: %v", e)
			}
		})
	}
}

// Altering the pinned schema bytes while retaining its approved identity must
// refuse through the public runner; the unchanged copied fixture is accepted.
func TestPinnedInputDigestRefusal(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/bsim-auth-2d23abd.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
		b, e := os.ReadFile(filepath.Join(repo, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = write(filepath.Join(root, name), string(b)); e != nil {
			t.Fatal(e)
		}
	}
	if got := run([]string{"--repo", root}); got != 0 {
		t.Fatalf("valid copied pins refused: %d", got)
	}
	p := filepath.Join(root, "compatibility/inputs/bsim-auth-2d23abd.toml")
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, append(b, []byte("\n# bounded drift control\n")...), 0644); e != nil {
		t.Fatal(e)
	}
	_, e = compat.Check(root, filepath.Join(root, "compatibility/runtime-manifest.json"))
	if e == nil || !strings.Contains(e.Error(), "bsim input digest disagreement") {
		t.Fatalf("pinned input drift admitted or wrong refusal: %v", e)
	}
	if got := run([]string{"--repo", root}); got != 1 {
		t.Fatalf("production input drift admitted: %d", got)
	}
}
