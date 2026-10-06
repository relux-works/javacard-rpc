package main

import (
	"bytes"
	"errors"
	"github.com/relux-works/javacard-rpc/pluginapi"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingPackagePlugin struct{}

func (failingPackagePlugin) Generate(*pluginapi.Schema, pluginapi.Options) ([]pluginapi.File, error) {
	return []pluginapi.File{{Name: "partial.txt", Data: []byte("forbidden")}}, errors.New("backend refused")
}

// All three production composition slots accept nested sources/root manifests
// and empty files, and reject malformed packages before changing any output.
func TestRunPackageOutputContract(t *testing.T) {
	good := []pluginapi.File{{Name: "build.manifest", Data: []byte("manifest")}, {Name: "src/nested/source.txt", Data: []byte("source")}, {Name: "empty", Data: []byte{}}}
	cases := []struct {
		name, path, diagnostic string
		files                  []pluginapi.File
		fails                  bool
	}{
		{name: "valid", files: good},
		{name: "absolute", path: "/absolute.txt", diagnostic: "invalid path"},
		{name: "escape", path: "../escape.txt", diagnostic: "invalid path"},
		{name: "dot", path: "src/../escape.txt", diagnostic: "invalid path"},
		{name: "double-slash", path: "src//file", diagnostic: "invalid path"},
		{name: "backslash", files: append(append([]pluginapi.File{}, good...), pluginapi.File{Name: `src\file`, Data: []byte("literal")})},
		{name: "drive", files: append(append([]pluginapi.File{}, good...), pluginapi.File{Name: "C:/file", Data: []byte("relative")})},
		{name: "empty-name", diagnostic: "invalid path"},
		{name: "dot-name", path: ".", diagnostic: "invalid path"},
		{name: "duplicate", files: append(append([]pluginapi.File{}, good...), good[0]), diagnostic: "duplicate path"},
		{name: "nil-data", files: []pluginapi.File{{Name: "bad"}}, diagnostic: "nil data"},
		{name: "empty-package", files: []pluginapi.File{}, diagnostic: "empty package"},
		{name: "conflict", files: []pluginapi.File{{Name: "src", Data: []byte("x")}, {Name: "src/file", Data: []byte("x")}}, diagnostic: "file/directory conflict"},
		{name: "plugin-error", fails: true, diagnostic: "backend refused"},
	}
	for _, slot := range []struct{ flag, root string }{{"--java", "counter-server-javacard"}, {"--swift", "counter-client-swift"}, {"--kotlin", "counter-client-kotlin"}} {
		for _, tc := range cases {
			t.Run(strings.TrimPrefix(slot.flag, "--")+"/"+tc.name, func(t *testing.T) {
				out := t.TempDir()
				root := filepath.Join(out, slot.root)
				if e := os.Mkdir(root, 0755); e != nil {
					t.Fatal(e)
				}
				sentinel := filepath.Join(root, "build.manifest")
				if e := os.WriteFile(sentinel, []byte("keep"), 0644); e != nil {
					t.Fatal(e)
				}
				outside := filepath.Join(out, "escape.txt")
				if e := os.WriteFile(outside, []byte("outside"), 0644); e != nil {
					t.Fatal(e)
				}
				files := tc.files
				if files == nil && !tc.fails {
					files = append(append([]pluginapi.File{}, good...), pluginapi.File{Name: tc.path, Data: []byte("bad")})
				}
				var p pluginapi.Plugin = &recordingPlugin{files: files}
				if tc.fails {
					p = failingPackagePlugin{}
				}
				var stderr bytes.Buffer
				code := runWithPlugins([]string{slot.flag, "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr, p, p, p)
				literalControl := (tc.name == "backslash" || tc.name == "drive") && filepath.Separator == '/'
				if tc.name == "valid" || literalControl {
					if code != 0 {
						t.Fatalf("valid exit %d: %s", code, &stderr)
					}
					for _, f := range files {
						b, e := os.ReadFile(filepath.Join(root, f.Name))
						if e != nil || !bytes.Equal(b, f.Data) {
							t.Fatalf("valid file %s: %s %v", f.Name, b, e)
						}
					}
				} else {
					diagnostic := tc.diagnostic
					if tc.name == "backslash" || tc.name == "drive" {
						diagnostic = "invalid path"
					}
					if code != 2 || !strings.Contains(stderr.String(), diagnostic) {
						t.Fatalf("invalid package exit %d: %s", code, &stderr)
					}
					b, e := os.ReadFile(sentinel)
					if e != nil || string(b) != "keep" {
						t.Fatal("target sentinel altered")
					}
					entries, e := os.ReadDir(root)
					if e != nil || len(entries) != 1 {
						t.Fatal("partial output")
					}
				}
				b, e := os.ReadFile(outside)
				if e != nil || string(b) != "outside" {
					t.Fatal("outside sentinel altered")
				}
			})
		}
	}
}

// Existing root, directory and leaf symlinks cannot redirect package writes.
// Refusal happens before manifests change; concurrent filesystem races are out.
func TestRunPackageSymlinkRefusal(t *testing.T) {
	for _, name := range []string{"root", "directory", "file"} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			outside := t.TempDir()
			root := filepath.Join(out, "counter-server-javacard")
			if e := os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0644); e != nil {
				t.Fatal(e)
			}
			if name == "root" {
				if e := os.Symlink(outside, root); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := os.Mkdir(root, 0755); e != nil {
					t.Fatal(e)
				}
				if name == "directory" {
					if e := os.Symlink(outside, filepath.Join(root, "src")); e != nil {
						t.Fatal(e)
					}
				} else {
					if e := os.Mkdir(filepath.Join(root, "src"), 0755); e != nil {
						t.Fatal(e)
					}
					if e := os.Symlink(filepath.Join(outside, "keep"), filepath.Join(root, "src", "keep")); e != nil {
						t.Fatal(e)
					}
				}
			}
			p := &recordingPlugin{files: []pluginapi.File{{Name: "manifest", Data: []byte("x")}, {Name: "src/keep", Data: []byte("bad")}}}
			var stderr bytes.Buffer
			if code := runWithPlugins([]string{"--java", "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr, p, p, p); code != 2 || !strings.Contains(stderr.String(), "symlink output path") {
				t.Fatalf("symlink exit %d: %s", code, &stderr)
			}
			b, e := os.ReadFile(filepath.Join(outside, "keep"))
			if e != nil || string(b) != "outside" {
				t.Fatal("outside changed")
			}
			if _, e := os.Stat(filepath.Join(root, "manifest")); !os.IsNotExist(e) {
				t.Fatal("manifest written on refusal")
			}
		})
	}
}

// Filesystem failures retain exit 3. A later backend error does not roll back a
// successfully written previous target or write that failed target's files.
func TestRunPackageFailureScope(t *testing.T) {
	t.Run("io", func(t *testing.T) {
		out := t.TempDir()
		if e := os.WriteFile(filepath.Join(out, "counter-server-javacard"), []byte("keep"), 0644); e != nil {
			t.Fatal(e)
		}
		var stderr bytes.Buffer
		if code := run([]string{"--java", "probe", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr); code != 3 || !strings.Contains(stderr.String(), "create java package dir") {
			t.Fatalf("io exit %d: %s", code, &stderr)
		}
	})
	t.Run("later-target", func(t *testing.T) {
		out := t.TempDir()
		p := &recordingPlugin{files: []pluginapi.File{{Name: "manifest", Data: []byte("ok")}}}
		var stderr bytes.Buffer
		if code := runWithPlugins([]string{"--all", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr, p, failingPackagePlugin{}, p); code != 2 || !strings.Contains(stderr.String(), "backend refused") {
			t.Fatalf("exit %d: %s", code, &stderr)
		}
		b, e := os.ReadFile(filepath.Join(out, "counter-server-javacard", "manifest"))
		if e != nil || string(b) != "ok" {
			t.Fatal("previous target lost")
		}
		if _, e := os.Stat(filepath.Join(out, "counter-client-swift")); !os.IsNotExist(e) {
			t.Fatal("failed target wrote output")
		}
	})
}

// Namespace layout keeps the released filepath.Join cleanup, rather than
// treating accepted repeated/leading/trailing dots as malformed plugin paths.
func TestRunPackageNamespaceLayoutCompatibility(t *testing.T) {
	for _, flag := range []string{"--java", "--kotlin"} {
		for _, namespace := range []string{".probe..client.", "probe/client", "probe client"} {
			t.Run(flag+"/"+namespace, func(t *testing.T) {
				out := t.TempDir()
				var stderr bytes.Buffer
				if code := run([]string{flag, namespace, "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr); code != 0 {
					t.Fatalf("compatible namespace exit %d: %s", code, &stderr)
				}
				suffix, lang, file := "-server-javacard", "java", "CounterSkeleton.java"
				if flag == "--kotlin" {
					suffix, lang, file = "-client-kotlin", "kotlin", "CounterClient.kt"
				}
				target := filepath.Join(out, "counter"+suffix, "src", "main", lang, strings.ReplaceAll(namespace, ".", string(filepath.Separator)), file)
				if b, e := os.ReadFile(target); e != nil || len(b) == 0 {
					t.Fatalf("released layout %s: %v", target, e)
				}
			})
		}
	}
}
