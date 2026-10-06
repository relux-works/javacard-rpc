package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

var packageSlots = []struct{ flag, root string }{
	{"--java", "counter-server-javacard"},
	{"--swift", "counter-client-swift"},
	{"--kotlin", "counter-client-kotlin"},
}

// Existing roots retain every file after a NUL refusal, across all composition
// slots and NUL positions. Nearby valid empty files exercise the same entry.
func TestReviewerMalformedNULNoPartialWrites(t *testing.T) {
	for _, slot := range packageSlots {
		for _, name := range []string{"bad\x00name", "src/\x00bad", "src/bad\x00"} {
			t.Run(strings.TrimPrefix(slot.flag, "--")+"/"+name, func(t *testing.T) {
				out := t.TempDir()
				root := filepath.Join(out, slot.root)
				if err := os.Mkdir(root, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "manifest"), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
				good := &recordingPlugin{files: []pluginapi.File{{Name: "valid", Data: []byte{}}}}
				args := []string{slot.flag, "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}
				var stderr bytes.Buffer
				if code := runWithPlugins(args, &stderr, good, good, good); code != 0 {
					t.Fatalf("control exit %d: %s", code, &stderr)
				}
				bad := nulPackage(name)
				stderr.Reset()
				code := runWithPlugins(args, &stderr, bad, bad, bad)
				b, err := os.ReadFile(filepath.Join(root, "manifest"))
				if code != 2 || !strings.Contains(stderr.String(), "invalid plugin output: invalid path") || err != nil || string(b) != "keep" {
					t.Fatalf("malformed output must refuse exit 2 before writes: exit=%d sentinel=%q error=%v diagnostic=%q", code, b, err, stderr.String())
				}
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 2 {
					t.Fatalf("partial output: %v %v", entries, err)
				}
				b, err = os.ReadFile(filepath.Join(root, "valid"))
				if err != nil || len(b) != 0 {
					t.Fatalf("valid empty file altered: %v", err)
				}
			})
		}
	}
}

func nulPackage(name string) *recordingPlugin {
	return &recordingPlugin{files: []pluginapi.File{{Name: "manifest", Data: []byte("forbidden")}, {Name: name, Data: []byte("bad")}}}
}

// Fresh roots stay absent after malformed output; valid source/manifest output
// creates the same roots afterward. This covers all three production slots.
func TestReviewerMalformedNULFreshRoot(t *testing.T) {
	for _, slot := range packageSlots {
		t.Run(strings.TrimPrefix(slot.flag, "--"), func(t *testing.T) {
			out := t.TempDir()
			root := filepath.Join(out, slot.root)
			args := []string{slot.flag, "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}
			bad := nulPackage("bad\x00name")
			var stderr bytes.Buffer
			code := runWithPlugins(args, &stderr, bad, bad, bad)
			_, err := os.Stat(root)
			if code != 2 || !os.IsNotExist(err) || !strings.Contains(stderr.String(), "invalid plugin output: invalid path") {
				t.Fatalf("malformed package must refuse before creating target: exit=%d root-error=%v diagnostic=%q", code, err, stderr.String())
			}
			good := &recordingPlugin{files: []pluginapi.File{{Name: "manifest", Data: []byte("ok")}, {Name: "src/valid", Data: []byte{}}}}
			stderr.Reset()
			if code := runWithPlugins(args, &stderr, good, good, good); code != 0 {
				t.Fatalf("control exit %d: %s", code, &stderr)
			}
			for _, file := range good.files {
				b, err := os.ReadFile(filepath.Join(root, file.Name))
				if err != nil || !bytes.Equal(b, file.Data) {
					t.Fatalf("valid file %s: %v", file.Name, err)
				}
			}
		})
	}
}

// Keep the reviewer's precise partial-manifest witness under its original name.
func TestReviewerMalformedNULWritesManifest(t *testing.T) {
	out := t.TempDir()
	bad := nulPackage("bad\x00name")
	var stderr bytes.Buffer
	code := runWithPlugins([]string{"--java", "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr, bad, bad, bad)
	b, err := os.ReadFile(filepath.Join(out, "counter-server-javacard", "manifest"))
	if code != 2 || !os.IsNotExist(err) {
		t.Fatalf("malformed output leaked valid manifest: exit=%d manifest=%q read-error=%v diagnostic=%q", code, b, err, stderr.String())
	}
}

// A valid package blocked by an ordinary filesystem file remains I/O exit 3,
// with the blocking file intact, in every slot (not generation refusal exit 2).
func TestRunPackageIOFailureAcrossSlots(t *testing.T) {
	for _, slot := range packageSlots {
		t.Run(strings.TrimPrefix(slot.flag, "--"), func(t *testing.T) {
			out := t.TempDir()
			root := filepath.Join(out, slot.root)
			if err := os.WriteFile(root, []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			good := &recordingPlugin{files: []pluginapi.File{{Name: "manifest", Data: []byte("ok")}, {Name: "src/valid", Data: []byte{}}}}
			var stderr bytes.Buffer
			code := runWithPlugins([]string{slot.flag, "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr, good, good, good)
			b, err := os.ReadFile(root)
			if code != 3 || !strings.Contains(stderr.String(), "create ") || err != nil || string(b) != "keep" {
				t.Fatalf("ordinary I/O exit=%d file=%q error=%v diagnostic=%q", code, b, err, stderr.String())
			}
		})
	}
}

// Independently built released and candidate CLIs preserve acceptance, verbose
// ordering and every generated byte for literal POSIX colon/backslash inputs
// and neighboring controls. This does not claim native compiler acceptance.
func TestReviewerNamespaceCompatibility(t *testing.T) {
	baseline := os.Getenv("JCRPC_BASELINE_GEN")
	if baseline == "" {
		t.Skip("set JCRPC_BASELINE_GEN to the immutable v0.4.5 CLI")
	}
	if filepath.Separator != '/' {
		t.Skip("literal colon/backslash compatibility is a POSIX filesystem contract")
	}
	candidate := filepath.Join(t.TempDir(), "jcrpc-gen")
	build := exec.Command("go", "build", "-o", candidate, "./cmd/jcrpc-gen")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOWORK=off")
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("candidate build: %v %s", err, b)
	}
	input, err := filepath.Abs(filepath.Join("..", "..", "testdata", "counter.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--java", "--kotlin"} {
		for _, ns := range []struct{ name, value string }{{"normal", "probe.client"}, {"colon", "probe:client"}, {"backslash", `probe\client`}, {"literal-backslash-dots", `probe\..\client`}, {"unicode", "пример.клиент"}, {"tab", "probe\tclient"}} {
			t.Run(strings.TrimPrefix(flag, "--")+"/"+ns.name, func(t *testing.T) {
				old, now := t.TempDir(), t.TempDir()
				invoke := func(bin, out string) string {
					t.Helper()
					b, err := exec.Command(bin, "--verbose", flag, ns.value, "--out-dir", out, input).CombinedOutput()
					if err != nil {
						t.Fatalf("released input narrowed: %s: %v %s", bin, err, b)
					}
					return strings.ReplaceAll(string(b), out, "<out-dir>")
				}
				oldLog, newLog := invoke(baseline, old), invoke(candidate, now)
				if oldLog != newLog {
					t.Fatalf("namespace diagnostic/order drift: %q vs %q", oldLog, newLog)
				}
				left, right := namespaceInventory(t, old), namespaceInventory(t, now)
				if len(left) == 0 || len(left) != len(right) {
					t.Fatalf("namespace inventory drift: %d vs %d", len(left), len(right))
				}
				for name, a := range left {
					b, ok := right[name]
					if !ok || !bytes.Equal(a, b) {
						t.Fatalf("namespace byte drift: %s", name)
					}
				}
			})
		}
	}
}

func namespaceInventory(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[name] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
