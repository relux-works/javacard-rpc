package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The production parity CLI sees a valid whole matrix as equal, then exits 1
// specifically for the one-byte plant in a real generated candidate file.
func TestParityCLIBytePlant(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "jcrpc-gen")
	build := exec.Command("go", "build", "-o", binary, "../jcrpc-gen")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "report.json")
	args := []string{"--baseline", binary, "--candidate", binary, "--repo", repo, "--consumer", filepath.Join(repo, "codegen", "testdata", "counter.toml"), "--out", out}
	if code := run(args); code != 0 {
		t.Fatalf("equal CLI control exit %d", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var evidence report
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	// Four inputs, six target selections, three memory choices, three simulator
	// choices, and four non-generation controls: omission cannot look like parity.
	if len(evidence.Comparisons) != 4*(6*3*3+4) {
		t.Fatalf("matrix coverage: %d of 232 comparisons", len(evidence.Comparisons))
	}
	if code := run(append(args, "--plant-byte-change")); code != 1 {
		t.Fatalf("plant exit %d want 1", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "output drift:") || strings.Contains(string(b), "CLI behavior drift:") {
		t.Fatalf("wrong plant verdict: %s", b)
	}
}

// The comparator admits equal inventories but refuses a changed byte, a missing
// file, and an extra file. These controls bound parity to file names and bytes.
func TestCompareFilesRejectsDrift(t *testing.T) {
	for _, tc := range []struct {
		name      string
		candidate map[string]string
		drift     bool
	}{
		{"equal", map[string]string{"a": "one", "b": "two"}, false},
		{"byte", map[string]string{"a": "changed", "b": "two"}, true},
		{"missing", map[string]string{"a": "one"}, true},
		{"extra", map[string]string{"a": "one", "b": "two", "c": "three"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := compareFiles(map[string]string{"a": "one", "b": "two"}, tc.candidate)
			if tc.drift {
				if err == nil || !strings.Contains(err.Error(), "output drift:") {
					t.Fatalf("wrong verdict: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Real file inventory hashes bytes and refuses missing roots and symlinks;
// inability to inspect output must never become an empty, passing inventory.
func TestInventoryRejectsUnreadableTree(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source")
	if err := os.WriteFile(path, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	a, err := inventory(root)
	if err != nil || len(a) != 1 {
		t.Fatalf("valid inventory: %v %v", a, err)
	}
	if err := os.WriteFile(path, []byte("two"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if compareFiles(a, b) == nil {
		t.Fatal("changed bytes admitted")
	}
	if _, err := inventory(filepath.Join(root, "absent")); err == nil {
		t.Fatal("missing root admitted")
	}
	if err := os.Symlink(path, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory(root); err == nil || !strings.Contains(err.Error(), "non-regular output") {
		t.Fatalf("symlink verdict: %v", err)
	}
}
