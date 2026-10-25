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
		"missing-kotlin":         func(m *compat.Manifest) { m.Targets = append(m.Targets[:1], m.Targets[2:]...) },
		"duplicate":              func(m *compat.Manifest) { m.Targets[1] = m.Targets[0] },
		"unknown":                func(m *compat.Manifest) { m.Targets[1].Target = "other" },
		"schema":                 func(m *compat.Manifest) { m.Schema = 2 },
		"old-facade-version":     func(m *compat.Manifest) { m.FacadeVersion = "0.6.0" },
		"missing-facade-version": func(m *compat.Manifest) { m.FacadeVersion = "" },
		"input-drift":            func(m *compat.Manifest) { m.SampleInput.SHA256 = strings.Repeat("0", 64) },
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
			for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/sample-store.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
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

// Historical and candidate module identities used only by copied fixtures. The
// signed v0.5.0 backend requires API v0.1.1; the candidate pseudo-versions name
// the commits that were later tagged pluginapi/v0.1.2 and backend v0.6.0.
const (
	historicalAPISums     = "github.com/relux-works/javacard-rpc/pluginapi v0.1.1 h1:cNvoCdRTp88tLTIli+zk391jspJZRtBgdTMzLGmqPa0=\ngithub.com/relux-works/javacard-rpc/pluginapi v0.1.1/go.mod h1:vini/jA9xID/u2iYwtKViPacEvP9nLIapL3RdQV7RBc=\n"
	historicalBackendSums = "github.com/relux-works/javacard-rpc-server-javacard v0.5.0 h1:gzkUw3pYQzjupDx/sjsfJG1D4+nfZmT4xFcFshUwt3c=\ngithub.com/relux-works/javacard-rpc-server-javacard v0.5.0/go.mod h1:S4iGoPfsidhqSQ3Kap/TCJDIyON8/wIMCPkvjGtZ1M8=\n"
	candidateAPI          = "v0.1.2-0.20261014223732-4e10e2cfd254"
	candidateAPISums      = "github.com/relux-works/javacard-rpc/pluginapi " + candidateAPI + " h1:H4lp/V3q34HDvIid0xZNghs/7P3VaVvZ3fNVUHhoohs=\ngithub.com/relux-works/javacard-rpc/pluginapi " + candidateAPI + "/go.mod h1:vini/jA9xID/u2iYwtKViPacEvP9nLIapL3RdQV7RBc=\n"
	candidateBackend      = "v0.5.1-0.20261014224032-50f13333ded2"
	candidateBackendSums  = "github.com/relux-works/javacard-rpc-server-javacard " + candidateBackend + " h1:vEP2FHDJw0lfOEgKbS06ZWAVIJADsZZ9IVbB+KRwRjg=\ngithub.com/relux-works/javacard-rpc-server-javacard " + candidateBackend + "/go.mod h1:oorIx1weagWRaRsvIfPNei0UR9Yzt5C/iQfHuHBamrQ=\n"
)

// copyCompatConfig copies only the configuration Check reads, so the resolver
// runs against the real public graph without local sources.
func copyCompatConfig(t *testing.T, repo string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/sample-store.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
		b, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := write(filepath.Join(root, name), string(b)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// editFile applies exact byte replacements and appends extra bytes; a missing
// anchor fails the fixture instead of silently testing the unchanged tuple.
func editFile(t *testing.T, path string, pairs [][2]string, extra string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range pairs {
		if !bytes.Contains(b, []byte(pair[0])) {
			t.Fatalf("fixture anchor %q missing in %s", pair[0], path)
		}
		b = bytes.ReplaceAll(b, []byte(pair[0]), []byte(pair[1]))
	}
	if err := os.WriteFile(path, append(b, []byte(extra)...), 0644); err != nil {
		t.Fatal(err)
	}
}

// historicalJavaCard pins the authentic signed backend v0.5.0 tuple in go.mod,
// the manifest and its receipt, so Check passes every backend guard.
func historicalJavaCard(t *testing.T, root string) {
	t.Helper()
	editFile(t, filepath.Join(root, "codegen/go.mod"), [][2]string{{"javacard-rpc-server-javacard v0.6.0", "javacard-rpc-server-javacard v0.5.0"}}, "")
	for _, name := range []string{"compatibility/runtime-manifest.json", "compatibility/releases/javacard.json"} {
		editFile(t, filepath.Join(root, name), [][2]string{{"v0.6.0", "v0.5.0"}, {"javacard:0.6.0", "javacard:0.5.0"}, {"46fa6ecb45135be0ba9b1d4b9cd9aa8663718510", "96bf35555bba8a040eba880622629ff0f25b2556"}, {"b43b8d6d2834f57d2a1e899b99fe22996e2709ee", "9bbf1db745834f3efefb54846f26bec6310f8c22"}}, "")
	}
}

// The production compatibility CLI requires the released API v0.1.2 and refuses
// the previous release, the candidate pseudo-version of the same commit, or a
// local replacement; the exact public graph is accepted before each bounded
// change is planted in the copied configuration. Lowering only the direct
// requirement is a positive MVS control: backend v0.6.0 still selects v0.1.2.
func TestManifestAPIRefusals(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"obsolete", "pseudo", "lower-direct-require", "replace"} {
		t.Run(kind, func(t *testing.T) {
			root := copyCompatConfig(t, repo)
			if code := run([]string{"--repo", root}); code != 0 {
				t.Fatalf("public API control refused: %d", code)
			}
			mod := filepath.Join(root, "codegen/go.mod")
			sum := filepath.Join(root, "codegen/go.sum")
			switch kind {
			case "obsolete":
				// Only the real historical backend tuple can select the
				// previous API; both are signed public releases.
				historicalJavaCard(t, root)
				editFile(t, mod, [][2]string{{"javacard-rpc/pluginapi v0.1.2", "javacard-rpc/pluginapi v0.1.1"}}, "")
				editFile(t, sum, nil, historicalAPISums+historicalBackendSums)
			case "pseudo":
				// The historical backend lets MVS keep the candidate
				// pseudo-version, which sorts below released v0.1.2.
				historicalJavaCard(t, root)
				editFile(t, mod, [][2]string{{"javacard-rpc/pluginapi v0.1.2", "javacard-rpc/pluginapi " + candidateAPI}}, "")
				editFile(t, sum, nil, historicalAPISums+historicalBackendSums+candidateAPISums)
			case "lower-direct-require":
				editFile(t, mod, [][2]string{{"javacard-rpc/pluginapi v0.1.2", "javacard-rpc/pluginapi v0.1.1"}}, "")
				editFile(t, sum, nil, historicalAPISums)
			case "replace":
				editFile(t, mod, nil, "\nreplace github.com/relux-works/javacard-rpc/pluginapi => "+filepath.Join(repo, "pluginapi")+"\n")
			}
			if kind == "lower-direct-require" {
				// Resolve the edited fixture graph with Go's normal module update;
				// readonly go list refuses the unnormalized go.mod instead.
				cmd := exec.Command("go", "list", "-mod=mod", "-m", "-f", "{{.Path}} {{.Version}}", "all")
				cmd.Dir = filepath.Join(root, "codegen")
				cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
				selected, err := cmd.CombinedOutput()
				if err != nil || !strings.Contains(string(selected), "github.com/relux-works/javacard-rpc/pluginapi v0.1.2\n") {
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
			if err == nil || err.Error() != "pluginapi must resolve released v0.1.2 without replace" {
				t.Fatalf("wrong API refusal: %v", err)
			}
		})
	}
}

// The candidate backend pseudo-version is refused through the production CLI
// whether the receipts keep the signed v0.6.0 identity (graph disagreement) or
// are rewritten to claim the pseudo-version itself (not a release tag). The
// unchanged copied released tuple is the positive control.
func TestManifestCandidatePseudoVersionRefusals(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for kind, want := range map[string]string{
		"graph":   "backend module disagreement: javacard",
		"claimed": "release tag is not a plain semantic version: javacard",
	} {
		t.Run(kind, func(t *testing.T) {
			root := copyCompatConfig(t, repo)
			if code := run([]string{"--repo", root}); code != 0 {
				t.Fatalf("released tuple control refused: %d", code)
			}
			editFile(t, filepath.Join(root, "codegen/go.mod"), [][2]string{{"javacard-rpc-server-javacard v0.6.0", "javacard-rpc-server-javacard " + candidateBackend}}, "")
			editFile(t, filepath.Join(root, "codegen/go.sum"), nil, candidateBackendSums+candidateAPISums)
			if kind == "claimed" {
				for _, name := range []string{"compatibility/runtime-manifest.json", "compatibility/releases/javacard.json"} {
					editFile(t, filepath.Join(root, name), [][2]string{{"v0.6.0", candidateBackend}}, "")
				}
			}
			if code := run([]string{"--repo", root}); code != 1 {
				t.Fatalf("candidate backend accepted: %s exit %d", kind, code)
			}
			_, err := compat.Check(root, filepath.Join(root, "compatibility/runtime-manifest.json"))
			if err == nil || err.Error() != want {
				t.Fatalf("wrong candidate refusal: %v", err)
			}
		})
	}
}

// Altering the pinned schema bytes while retaining its approved identity must
// refuse through the public runner; the unchanged synthetic fixture is accepted.
func TestPinnedInputDigestRefusal(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/sample-store.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
		b, e := os.ReadFile(filepath.Join(repo, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = write(filepath.Join(root, name), string(b)); e != nil {
			t.Fatal(e)
		}
	}
	if got := run([]string{"--repo", root}); got != 0 {
		t.Fatalf("valid synthetic pins refused: %d", got)
	}
	p := filepath.Join(root, "compatibility/inputs/sample-store.toml")
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, append(b, []byte("\n# bounded drift control\n")...), 0644); e != nil {
		t.Fatal(e)
	}
	_, e = compat.Check(root, filepath.Join(root, "compatibility/runtime-manifest.json"))
	if e == nil || !strings.Contains(e.Error(), "sample input digest disagreement") {
		t.Fatalf("pinned input drift admitted or wrong refusal: %v", e)
	}
	if got := run([]string{"--repo", root}); got != 1 {
		t.Fatalf("production input drift admitted: %d", got)
	}
}
