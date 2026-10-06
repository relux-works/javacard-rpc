package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The immutable B6 schema rejects Swift selection through the actual generator
// executable before creating output. Explicit Java/Kotlin generation is valid.
func TestPinnedBSimSwiftStreamRefusal(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	bin := filepath.Join(t.TempDir(), "jcrpc-gen")
	c := exec.Command("go", "build", "-o", bin, "../jcrpc-gen")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, b)
	}
	input := filepath.Join(repo, "compatibility/inputs/bsim-auth-2d23abd.toml")
	for _, flag := range []string{"--all", "--swift"} {
		out := filepath.Join(t.TempDir(), "out")
		args := []string{flag}
		if flag == "--swift" {
			args = append(args, "BSimAuthClient")
		}
		args = append(args, "--out-dir", out, input)
		c = exec.Command(bin, args...)
		b, e := c.CombinedOutput()
		exit, ok := e.(*exec.ExitError)
		if !ok || exit.ExitCode() != 2 || !strings.Contains(string(b), "Swift stream generation is not implemented") {
			t.Fatalf("pinned stream refusal: %v %s", e, b)
		}
		if _, e = os.Stat(out); !os.IsNotExist(e) {
			t.Fatalf("Swift stream wrote output: %v", e)
		}
	}
	c = exec.Command(bin, "--java", "io.jcrpc.bsim", "--kotlin", "io.jcrpc.bsim", "--out-dir", t.TempDir(), input)
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("supported pinned generation: %v %s", e, b)
	}
}
