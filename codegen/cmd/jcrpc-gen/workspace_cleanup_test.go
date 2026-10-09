package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	javacard "github.com/relux-works/javacard-rpc-server-javacard/codegen"
	"github.com/relux-works/javacard-rpc/codegen"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

func cleanupInput(t *testing.T, storage, mode string) string {
	t.Helper()
	input, err := os.ReadFile(filepath.Join("..", "..", "testdata", "stream.toml"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := "[applet]"
	if storage != "omitted" {
		metadata += fmt.Sprintf("\nstream_workspace = %q", storage)
	}
	if mode != "omitted" {
		metadata += fmt.Sprintf("\nstream_workspace_cleanup = %q", mode)
	}
	return writeFile(t, t.TempDir(), "stream.toml", strings.Replace(string(input), "[applet]", metadata, 1))
}

// Real composed CLI output equals the actually published whole-only backend's
// entire package for both control-memory policies. Kotlin output remains equal
// to the empty-selector control. Native target properties exercise these same
// Java bytes; this test does not claim hardware cleanup or allocation behavior.
func TestRunWholeCleanupGeneration(t *testing.T) {
	for _, memory := range []string{"clear_on_deselect", "clear_on_reset"} {
		t.Run(memory, func(t *testing.T) {
			input := cleanupInput(t, "persistent", pluginapi.StreamWorkspaceCleanupWholeReplyArea)
			out := t.TempDir()
			var stderr bytes.Buffer
			args := []string{"--java", "probe", "--kotlin", "probe", "--stream-memory", memory, "--out-dir", out, input}
			if code := run(args, &stderr); code != 0 {
				t.Fatalf("published whole generation: %d %s", code, &stderr)
			}
			schema, err := codegen.ParseFile(input)
			if err != nil {
				t.Fatal(err)
			}
			files, err := (javacard.Plugin{}).Generate(schema, pluginapi.Options{Namespace: "probe", StreamMemory: memory, SimulatorDependency: defaultSimulatorDependency})
			if err != nil {
				t.Fatal(err)
			}
			generated := cleanupFiles(t, filepath.Join(out, "streamdemo-server-javacard"))
			if len(generated) != len(files) || len(files) == 0 {
				t.Fatalf("published package inventory: %d != %d", len(generated), len(files))
			}
			for _, file := range files {
				if !bytes.Equal(generated[file.Name], file.Data) {
					t.Fatalf("published package differs: %s", file.Name)
				}
			}
			control := t.TempDir()
			args[len(args)-1] = cleanupInput(t, "persistent", "")
			args[len(args)-2] = control
			stderr.Reset()
			if code := run(args, &stderr); code != 0 {
				t.Fatalf("empty cleanup control: %d %s", code, &stderr)
			}
			wholeKotlin := cleanupFiles(t, filepath.Join(out, "streamdemo-client-kotlin"))
			defaultKotlin := cleanupFiles(t, filepath.Join(control, "streamdemo-client-kotlin"))
			if len(wholeKotlin) == 0 || len(wholeKotlin) != len(defaultKotlin) {
				t.Fatal("Kotlin inventory differs")
			}
			for name, content := range defaultKotlin {
				if !bytes.Equal(content, wholeKotlin[name]) {
					t.Fatalf("Kotlin cleanup drift: %s", name)
				}
			}
		})
	}
}

// Retains the historical measured whole-mode identities and unchanged transport.
// Real TOML CLI generation matches the five regenerated signed v0.5.0 source
// identities, whose four API-bearing files intentionally changed. B4 restores
// the keeper fixture CLA. This is source parity, not repeated Auth measurement.
func TestRunWholeCleanupMeasuredAuthIdentity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "compatibility", "inputs", "bsim-auth-2d23abd.toml"))
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Replace(string(raw), "cla = 0xB6", "cla = 0xB4", 1)
	input = strings.Replace(input, "[applet]", "[applet]\nstream_workspace = \"persistent\"\nstream_workspace_cleanup = \"whole-reply-area\"", 1)
	schema := writeFile(t, t.TempDir(), "auth.toml", input)
	out := t.TempDir()
	var stderr bytes.Buffer
	if code := run([]string{"--java", "ru.mts.bsimid.applet.auth", "--stream-memory", "clear_on_reset", "--simulator-dependency", "works.relux:jcardsim:3.0.5.9-relux.2", "--out-dir", out, schema}, &stderr); code != 0 {
		t.Fatalf("measured Auth generation: %d %s", code, &stderr)
	}
	historical := map[string]string{
		"BSimAuthBoundedStreamRuntime.java": "5063f440b42c08619274cd16206541c55c98004acd3e3c8b727c26e9af9fb90f",
		"BSimAuthSkeleton.java":             "42ad5fa6ca43e93eb39e9d28e60b302ffd6bd51080a0e2f773c02e25f0d56cd7",
		"BSimAuthStreamAPDUAdapter.java":    "ffeabcbc5fd61f86ce2fbe66912f0d8d228a1fbde4ac10737934e5c9ce49c8f8",
		"BSimAuthStreamEndpoint.java":       "52af535eb9bf709701333862ea07d3e58f61f21ddbeac02da83465d525c05c09",
		"BSimAuthTransport.java":            "75ff6fd4b035769347be3c6debe4e458e0141a0544f2f779a727939c626fb279",
	}
	var expected map[string]string
	receipt, err := os.ReadFile(filepath.Join("..", "..", "testdata", "bsim-whole-source-v050.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(receipt, &expected); err != nil {
		t.Fatal(err)
	}
	if len(expected) != len(historical) || expected["BSimAuthTransport.java"] != historical["BSimAuthTransport.java"] {
		t.Fatal("historical transport/inventory identity changed")
	}
	checked := 0
	for name, data := range cleanupFiles(t, out) {
		want, ok := expected[filepath.Base(name)]
		if !ok {
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("measured Auth source drift: %s", name)
		}
		checked++
	}
	if checked != len(expected) {
		t.Fatalf("measured source coverage %d of %d", checked, len(expected))
	}
}

// The real CLI's parse -> Validate path admits the persistent whole opt-in and old
// defaults, with no output writes. This is semantic evidence only; explicit-mode
// backend generation needs the published target and is tested separately.
func TestRunStreamWorkspaceCleanupValidation(t *testing.T) {
	for _, storage := range []string{"omitted", "", "transient", "persistent"} {
		for _, mode := range []string{"omitted", "", pluginapi.StreamWorkspaceCleanupWholeReplyArea, pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "bogus", "Written-bytes-only", " written-bytes-only "} {
			t.Run(storage+"/"+mode, func(t *testing.T) {
				input := cleanupInput(t, storage, mode)
				out := filepath.Join(t.TempDir(), "out")
				var stderr bytes.Buffer
				code := run([]string{"--validate-only", "--out-dir", out, input}, &stderr)
				valid := mode == "omitted" || mode == "" || storage == "persistent" && mode == pluginapi.StreamWorkspaceCleanupWholeReplyArea
				if valid {
					if code != exitCodeSuccess || stderr.Len() != 0 {
						t.Fatalf("valid cleanup refused: %d %s", code, &stderr)
					}
				} else if code != exitCodeValidation || !strings.Contains(stderr.String(), "applet.stream_workspace_cleanup") {
					t.Fatalf("cleanup validation refusal missing: %d %s", code, &stderr)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("validate-only wrote output")
				}
			})
		}
	}
}

// Real composed generation refuses unknown/dropped selectors and nonpersistent
// combinations at semantic exit 1, before a backend or filesystem write. Existing
// generated files stay intact; fresh destinations stay absent. An empty-selector
// generation control uses the same input and target selection.
func TestRunStreamWorkspaceCleanupRefusals(t *testing.T) {
	for _, tc := range []struct{ name, storage, mode, diagnostic string }{
		{"unknown", "persistent", "bogus", "must be whole-reply-area"},
		{"whole-omitted", "omitted", pluginapi.StreamWorkspaceCleanupWholeReplyArea, "requires stream_workspace = persistent"},
		{"whole-empty", "", pluginapi.StreamWorkspaceCleanupWholeReplyArea, "requires stream_workspace = persistent"},
		{"whole-transient", "transient", pluginapi.StreamWorkspaceCleanupWholeReplyArea, "requires stream_workspace = persistent"},
		{"written-persistent", "persistent", pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "must be whole-reply-area"},
		{"written-omitted", "omitted", pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "must be whole-reply-area"},
		{"written-empty", "", pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "must be whole-reply-area"},
		{"written-transient", "transient", pluginapi.StreamWorkspaceCleanupWrittenBytesOnly, "must be whole-reply-area"},
		{"whole-case", "persistent", "Whole-reply-area", "must be whole-reply-area"},
		{"whole-whitespace", "persistent", " whole-reply-area ", "must be whole-reply-area"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, flags := range [][]string{{"--java", "probe", "--kotlin", "probe"}, {"--kotlin", "probe"}} {
				control := t.TempDir()
				args := append(append([]string{}, flags...), "--out-dir", control, cleanupInput(t, tc.storage, ""))
				var stderr bytes.Buffer
				if code := run(args, &stderr); code != 0 {
					t.Fatalf("empty selector generation control: %d %s", code, &stderr)
				}
				for _, out := range []string{control, filepath.Join(t.TempDir(), "fresh")} {
					before := cleanupFiles(t, out)
					stderr.Reset()
					args := append(append([]string{}, flags...), "--out-dir", out, cleanupInput(t, tc.storage, tc.mode))
					code := run(args, &stderr)
					if code != exitCodeValidation || !strings.Contains(stderr.String(), "applet.stream_workspace_cleanup: "+tc.diagnostic) {
						t.Fatalf("cleanup generation refusal missing: %d %s", code, &stderr)
					}
					after := cleanupFiles(t, out)
					if len(before) != len(after) {
						t.Fatal("cleanup refusal changed output inventory")
					}
					for name, content := range before {
						if !bytes.Equal(content, after[name]) {
							t.Fatalf("cleanup refusal changed %s", name)
						}
					}
					if before == nil {
						if _, err := os.Stat(out); !os.IsNotExist(err) {
							t.Fatal("cleanup refusal created output root")
						}
					}
				}
			}
		})
	}
}

func cleanupFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular output: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		files[rel] = content
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// Explicit empty cleanup produces identical file inventories and bytes to
// omission for released storage/control policies on example and immutable Auth
// schemas. This checks current parity; signed-release binary parity is a separate
// release lane. It never exercises a hypothetical explicit-mode backend.
func TestRunEmptyCleanupPreservesOutputs(t *testing.T) {
	for _, input := range []string{filepath.Join("..", "..", "testdata", "counter.toml"), filepath.Join("..", "..", "testdata", "stream.toml"), filepath.Join("..", "..", "..", "compatibility", "inputs", "bsim-auth-2d23abd.toml")} {
		raw, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		for _, storage := range []string{"omitted", "transient", "persistent"} {
			for _, memory := range []string{"clear_on_deselect", "clear_on_reset"} {
				t.Run(filepath.Base(input)+"/"+storage+"/"+memory, func(t *testing.T) {
					metadata := "[applet]"
					if storage != "omitted" {
						metadata += fmt.Sprintf("\nstream_workspace = %q", storage)
					}
					baseline := strings.Replace(string(raw), "[applet]", metadata, 1)
					var previous map[string][]byte
					for _, extra := range []string{"", "\nstream_workspace_cleanup = \"\""} {
						schema := writeFile(t, t.TempDir(), "schema.toml", strings.Replace(baseline, "[applet]", "[applet]"+extra, 1))
						out := t.TempDir()
						flags := []string{"--java", "probe", "--kotlin", "probe", "--stream-memory", memory, "--out-dir", out}
						if filepath.Base(input) == "counter.toml" {
							flags = append(flags, "--swift", "CounterClient")
						}
						var stderr bytes.Buffer
						if code := run(append(flags, schema), &stderr); code != 0 {
							t.Fatalf("default parity generation: %d %s", code, &stderr)
						}
						files := cleanupFiles(t, out)
						if len(files) == 0 {
							t.Fatal("default parity generated no files")
						}
						if previous != nil {
							if len(previous) != len(files) {
								t.Fatal("empty cleanup changed file inventory")
							}
							for name, content := range previous {
								if !bytes.Equal(content, files[name]) {
									t.Fatalf("empty cleanup changed bytes: %s", name)
								}
							}
						}
						previous = files
					}
				})
			}
		}
	}
}
