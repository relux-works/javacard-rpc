package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
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

// The production compatibility CLI requires the published compatible API and
// refuses the obsolete API or a local replacement; the exact public graph is
// accepted before either bounded change is planted in the copied configuration.
func TestManifestAPIRefusals(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"obsolete", "lower-direct-require", "replace"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/bsim-auth-2d23abd.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
				b, err := os.ReadFile(filepath.Join(repo, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := write(filepath.Join(root, name), string(b)); err != nil {
					t.Fatal(err)
				}
			}
			if code := run([]string{"--repo", root}); code != 0 {
				t.Fatalf("public API control refused: %d", code)
			}
			mod := filepath.Join(root, "codegen/go.mod")
			b, err := os.ReadFile(mod)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "obsolete" || kind == "lower-direct-require" {
				b = bytes.Replace(b, []byte("github.com/relux-works/javacard-rpc/pluginapi v0.1.1"), []byte("github.com/relux-works/javacard-rpc/pluginapi v0.1.0"), 1)
				if kind == "obsolete" {
					// v0.3.1 transitively requires API v0.1.1. Only the real
					// historical backend tuple can select the obsolete API;
					// lowering a direct require alone is an accepted MVS control.
					b = bytes.Replace(b, []byte("javacard-rpc-server-javacard v0.3.1"), []byte("javacard-rpc-server-javacard v0.3.0"), 1)
					for _, name := range []string{"compatibility/runtime-manifest.json", "compatibility/releases/javacard.json"} {
						raw, err := os.ReadFile(filepath.Join(root, name))
						if err != nil {
							t.Fatal(err)
						}
						for _, pair := range [][2]string{{"v0.3.1", "v0.3.0"}, {"javacard:0.3.1", "javacard:0.3.0"}, {"56b07eaf757d51b80b6e0adfff7b8f8326f0b6fb", "0a41fc2cd30f0b1e0871a0e2ff74583a8c04128c"}, {"ca371dc528c263aa7892cc42ffebe4192c081e9c", "e6d02397fc94a771c2e2ac9f27f43a2ea7c5a0de"}} {
							raw = bytes.ReplaceAll(raw, []byte(pair[0]), []byte(pair[1]))
						}
						if err := os.WriteFile(filepath.Join(root, name), raw, 0644); err != nil {
							t.Fatal(err)
						}
					}
				}
				// Signed v0.4.6's API checksums keep this a semantic refusal test,
				// rather than an unrelated missing-sum resolver failure.
				sumPath := filepath.Join(root, "codegen/go.sum")
				sums, err := os.ReadFile(sumPath)
				if err != nil {
					t.Fatal(err)
				}
				sums = append(sums, []byte("github.com/relux-works/javacard-rpc/pluginapi v0.1.0 h1:SNT1MS/IcEzv84sJKwDGnP2j0QH8rexMpLNpOSAIlbI=\ngithub.com/relux-works/javacard-rpc/pluginapi v0.1.0/go.mod h1:vini/jA9xID/u2iYwtKViPacEvP9nLIapL3RdQV7RBc=\n")...)
				if kind == "obsolete" {
					sums = append(sums, []byte("github.com/relux-works/javacard-rpc-server-javacard v0.3.0 h1:PHDnUvoqNlUw8bqwqOodldlESy8jDu0BKCcn1lztlF4=\ngithub.com/relux-works/javacard-rpc-server-javacard v0.3.0/go.mod h1:bzncCYOWYLhEIUxldV1QDD1UzNuClA0fOchbBz3Ml+E=\n")...)
				}
				if err := os.WriteFile(sumPath, sums, 0644); err != nil {
					t.Fatal(err)
				}
			} else {
				b = append(b, []byte("\nreplace github.com/relux-works/javacard-rpc/pluginapi => "+filepath.Join(repo, "pluginapi")+"\n")...)
			}
			if err := os.WriteFile(mod, b, 0644); err != nil {
				t.Fatal(err)
			}
			if kind == "lower-direct-require" {
				// Resolve the edited fixture graph with Go's normal module update;
				// readonly go list refuses the unnormalized go.mod instead.
				cmd := exec.Command("go", "list", "-mod=mod", "-m", "-f", "{{.Path}} {{.Version}}", "all")
				cmd.Dir = filepath.Join(root, "codegen")
				cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
				selected, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(selected), "github.com/relux-works/javacard-rpc/pluginapi v0.1.1\n") {
					t.Fatalf("MVS selected %q: %v", selected, err)
				}
				if code := run([]string{"--repo", root}); code != 0 {
					t.Fatalf("higher transitive API refused: %d", code)
				}
				return
			}
			if code := run([]string{"--repo", root}); code != 1 {
				t.Fatalf("%s API accepted: %d", kind, code)
			}
			_, err = compat.Check(root, filepath.Join(root, "compatibility/runtime-manifest.json"))
			if err == nil || err.Error() != "pluginapi must resolve released v0.1.1 without replace" {
				t.Fatalf("wrong API refusal: %v", err)
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
