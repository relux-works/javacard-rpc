package main

import (
	"github.com/relux-works/javacard-rpc/codegen/internal/compat"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The actual public signed release is accepted with the approved trust anchor
// and refused through native-build launch when that signer is absent. Crypto
// validity alone must not stand in for approved signer authority.
func TestPinnedCheckoutSignatureRefusals(t *testing.T) {
	prepared := os.Getenv("JCRPC_COMPAT_ROOT")
	if prepared == "" {
		t.Skip("set JCRPC_COMPAT_ROOT to bootstrapped runtime checkouts")
	}
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	m, e := compat.Check(repo, filepath.Join(repo, "compatibility/runtime-manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range m.Targets {
		t.Run(p.Target+"/missing-signer", func(t *testing.T) {
			dir := filepath.Join(prepared, "runtimes", filepath.Base(p.GoModule))
			if e := compat.VerifyCheckout(repo, dir, p); e != nil {
				t.Fatalf("approved signature refused: %v", e)
			}
			fixture := t.TempDir()
			for _, name := range []string{"codegen/go.mod", "codegen/go.sum", "compatibility/runtime-manifest.json", "compatibility/inputs/bsim-auth-2d23abd.toml", "compatibility/releases/javacard.json", "compatibility/releases/kotlin.json", "compatibility/releases/swift.json"} {
				b, e := os.ReadFile(filepath.Join(repo, name))
				if e != nil {
					t.Fatal(e)
				}
				if e = write(filepath.Join(fixture, name), string(b)); e != nil {
					t.Fatal(e)
				}
			}
			keys, e := os.ReadFile(filepath.Join(repo, "compatibility/allowed_signers"))
			if e != nil {
				t.Fatal(e)
			}
			keyKind := "ssh-ed25519"
			if p.Target == "kotlin" {
				keyKind = "ecdsa-sha2-nistp521"
			}
			var retained []string
			for _, line := range strings.Split(string(keys), "\n") {
				if line != "" && !strings.Contains(line, " "+keyKind+" ") {
					retained = append(retained, line)
				}
			}
			if e = write(filepath.Join(fixture, "compatibility/allowed_signers"), strings.Join(retained, "\n")+"\n"); e != nil {
				t.Fatal(e)
			}
			if e = compat.VerifyCheckout(fixture, dir, p); e == nil || !strings.Contains(e.Error(), "No principal matched") {
				t.Fatalf("missing signer admitted or wrong refusal: %v", e)
			}
			if got := run([]string{"--repo", fixture, "--root", prepared, "--mode", "jvm"}); got != 1 {
				t.Fatalf("native launch admitted unapproved signer: %d", got)
			}
		})
	}
}
