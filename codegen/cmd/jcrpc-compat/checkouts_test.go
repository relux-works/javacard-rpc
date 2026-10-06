package main

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/compat"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Real signed release clones accept ignored build outputs but reject tracked and
// non-ignored untracked source drift before the production runner starts a build.
// This lane requires the explicit bootstrap checkout root; no test fetches tags.
func TestPinnedCheckoutSourceRefusals(t *testing.T) {
	prepared := os.Getenv("JCRPC_COMPAT_ROOT")
	if prepared == "" {
		t.Skip("set JCRPC_COMPAT_ROOT to the bootstrapped runtime checkout root")
	}
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	m, e := compat.Check(repo, filepath.Join(repo, "compatibility/runtime-manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, target := range m.Targets {
		for _, kind := range []string{"untracked-source", "tracked-source", "missing-tag"} {
			t.Run(target.Target+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				for _, p := range m.Targets {
					source := filepath.Join(prepared, "runtimes", filepath.Base(p.GoModule))
					dest := filepath.Join(root, "runtimes", filepath.Base(p.GoModule))
					c := exec.Command("git", "clone", "--local", "--no-hardlinks", source, dest)
					if b, e := c.CombinedOutput(); e != nil {
						t.Fatalf("clone control: %v %s", e, b)
					}
				}
				dir := filepath.Join(root, "runtimes", filepath.Base(target.GoModule))
				ignored := "build/control.txt"
				source := "src/main/java/UntrackedControl.java"
				if target.Target == "kotlin" {
					source = "src/main/kotlin/UntrackedControl.kt"
				}
				if target.Target == "swift" {
					ignored = ".build/control.txt"
					source = "Sources/JavaCardRPCClient/UntrackedControl.swift"
				}
				if e = write(filepath.Join(dir, ignored), "ignored build control"); e != nil {
					t.Fatal(e)
				}
				if e = compat.VerifyCheckout(repo, dir, target); e != nil {
					t.Fatalf("prepared valid checkout refused: %v", e)
				}
				want := "dirty pinned runtime"
				switch kind {
				case "untracked-source":
					e = write(filepath.Join(dir, source), "// Untracked source control\n")
				case "tracked-source":
					b, err := os.ReadFile(filepath.Join(dir, "README.md"))
					if err != nil {
						t.Fatal(err)
					}
					e = os.WriteFile(filepath.Join(dir, "README.md"), append(b, []byte("\nTracked drift control\n")...), 0644)
				case "missing-tag":
					c := exec.Command("git", "tag", "-d", target.Tag)
					c.Dir = dir
					e = c.Run()
					want = "git [rev-parse"
				}
				if e != nil {
					t.Fatal(e)
				}
				if e = compat.VerifyCheckout(repo, dir, target); e == nil || !strings.Contains(e.Error(), want) {
					t.Fatalf("checkout drift admitted or wrong refusal: %v", e)
				}
				if got := run([]string{"--repo", repo, "--root", root, "--mode", "jvm"}); got != 1 {
					t.Fatalf("production build admitted checkout drift: exit %d", got)
				}
			})
		}
	}
}
