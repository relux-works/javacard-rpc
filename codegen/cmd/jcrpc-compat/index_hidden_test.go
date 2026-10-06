package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Signed, unchanged local clones accept ignored build outputs and either index
// hint. The public CLI must refuse the same compiled source drift when visible
// or hidden, before bootstrap attestation or native launch; caller index is kept.
func TestPinnedCheckoutRejectsIndexHiddenTrackedSource(t *testing.T) {
	prepared := os.Getenv("JCRPC_COMPAT_ROOT")
	if prepared == "" {
		t.Skip("set JCRPC_COMPAT_ROOT to bootstrapped runtime checkouts")
	}
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	cli := filepath.Join(t.TempDir(), "jcrpc-compat")
	c := exec.Command("go", "build", "-o", cli, ".")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("build public verifier: %v %s", e, b)
	}
	command := func(dir, name string, args ...string) (string, int) {
		t.Helper()
		c := exec.Command(name, args...)
		c.Dir = dir
		b, err := c.CombinedOutput()
		if err == nil {
			return string(b), 0
		}
		if x, ok := err.(*exec.ExitError); ok {
			return string(b), x.ExitCode()
		}
		t.Fatalf("launch %s: %v", name, err)
		return "", -1
	}
	for _, hint := range []string{"assume-unchanged", "skip-worktree"} {
		t.Run("swift/"+hint, func(t *testing.T) {
			root := t.TempDir()
			for _, target := range []string{"javacard-rpc-server-javacard", "javacard-rpc-client-kotlin", "javacard-rpc-client-swift"} {
				if b, n := command(repo, "git", "clone", "--local", "--no-hardlinks", filepath.Join(prepared, "runtimes", target), filepath.Join(root, "runtimes", target)); n != 0 {
					t.Fatalf("clone control: %d %s", n, b)
				}
			}
			dir := filepath.Join(root, "runtimes/javacard-rpc-client-swift")
			rel := "Sources/JavaCardRPCClient/APDUCommand.swift"
			path := filepath.Join(dir, rel)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			changed := append(append([]byte{}, original...), []byte("\npublic func reviewerDriftSentinel() -> UInt8 { return 7 }\n")...)
			if e := write(filepath.Join(dir, ".build/control.txt"), "ignored output"); e != nil {
				t.Fatal(e)
			}
			args := []string{"--repo", repo, "--root", root, "--mode", "bootstrap"}
			if b, n := command(repo, cli, args...); n != 0 {
				t.Fatalf("valid signed control: %d %s", n, b)
			}
			if e := os.WriteFile(path, changed, 0644); e != nil {
				t.Fatal(e)
			}
			if b, n := command(repo, cli, args...); n != 1 || !strings.Contains(b, "dirty pinned runtime swift") {
				t.Fatalf("visible drift control: %d %s", n, b)
			}
			if e := os.WriteFile(path, original, 0644); e != nil {
				t.Fatal(e)
			}
			if b, n := command(dir, "git", "update-index", "--"+hint, rel); n != 0 {
				t.Fatalf("hint setup: %d %s", n, b)
			}
			indexPath := filepath.Join(dir, ".git/index")
			indexBefore, e := os.ReadFile(indexPath)
			if e != nil {
				t.Fatal(e)
			}
			if b, n := command(repo, cli, args...); n != 0 {
				t.Fatalf("unchanged hinted control: %d %s", n, b)
			}
			if e := os.WriteFile(path, changed, 0644); e != nil {
				t.Fatal(e)
			}
			if b, n := command(dir, "git", "status", "--porcelain", "--untracked-files=all"); n != 0 || strings.TrimSpace(b) != "" {
				t.Fatalf("hidden drift setup: %d %q", n, b)
			}
			// Both public paths must reject at the source boundary, before native tools.
			for _, mode := range []string{"bootstrap", "swift"} {
				args[len(args)-1] = mode
				b, n := command(repo, cli, args...)
				t.Logf("%s %s exit=%d: %s", hint, mode, n, b)
				if n != 1 || !strings.Contains(b, "dirty pinned runtime swift: tracked release comparison") {
					t.Fatalf("source integrity bypass: expected tracked-source refusal exit 1; got %d %s", n, b)
				}
			}
			indexAfter, e := os.ReadFile(indexPath)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(indexBefore, indexAfter) {
				t.Fatal("verification modified caller index")
			}
		})
	}
}
