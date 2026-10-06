package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func receipt(test, action, output string) string {
	b, _ := json.Marshal(struct{ Action, Test, Output string }{action, test, output})
	return string(b) + "\n"
}

// Receipt classification requires the intended assertion and the named fail;
// setup errors, missing/skipped tests and malformed receipts cannot attest kills.
func TestEvidenceRequiresNamedFailure(t *testing.T) {
	assertion := receipt("Expected", "output", "behavior admitted")
	for _, tc := range []struct {
		name, output string
		code         int
		killed       bool
	}{
		{"named", assertion + receipt("Expected", "fail", ""), 1, true},
		{"green", assertion + receipt("Expected", "fail", ""), 0, false},
		{"wrong-exit", assertion + receipt("Expected", "fail", ""), 2, false},
		{"other-test", assertion + receipt("Other", "fail", ""), 1, false},
		{"skip", assertion + receipt("Expected", "skip", ""), 1, false},
		{"compile", receipt("", "fail", ""), 1, false},
		{"malformed", assertion + `{"Action":"fail","Test":`, 1, false},
		{"setup", receipt("Expected", "output", "equal CLI control exit 1") + receipt("Expected", "fail", ""), 1, false},
		{"assertion-without-fail", assertion, 1, false},
		{"fail-without-assertion", receipt("Expected", "fail", ""), 1, false},
		{"other-assertion", receipt("Other", "output", "behavior admitted") + receipt("Expected", "fail", ""), 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := namedFailure("Expected", "behavior admitted", tc.code, []byte(tc.output)); got != tc.killed {
				t.Fatalf("kill receipt: got %t want %t", got, tc.killed)
			}
		})
	}
}

// A selected test must actually pass before mutation; skips and package-only
// success are not positive controls.
func TestEvidenceRequiresPassingControl(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		code         int
		valid        bool
	}{
		{"named", receipt("Expected", "pass", ""), 0, true},
		{"nonzero", receipt("Expected", "pass", ""), 1, false},
		{"package-only", receipt("", "pass", ""), 0, false},
		{"skip", receipt("Expected", "skip", "") + receipt("", "pass", ""), 0, false},
		{"malformed", receipt("Expected", "pass", "") + "broken", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := namedPass("Expected", tc.code, []byte(tc.output)); got != tc.valid {
				t.Fatalf("control: got %t want %t", got, tc.valid)
			}
		})
	}
}

// This bounded host fixture drives the public harness, not the real generator:
// it emits the same named assertion or setup diagnostic after the exact
// build.gradle narrowing, so the attestation guard is isolated from JVM/tooling.
func harnessFixture(t *testing.T, setup bool) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"compatibility/fixture.txt": "attestation classifier fixture",
		"codegen/go.mod":            "module fixture\n\ngo 1.23\n",
		"pluginapi/go.mod":          "module fixtureapi\n\ngo 1.23\n",
		"examples/corpus.toml":      "[applet]\nname = 'Corpus'\n",
		"codegen/cmd/jcrpc-parity/main.go": `package main
import "strings"
func compare(a,b map[string]string) bool {
 for k,a:=range a { b,bOK:=b[k]; aOK:=true
 if !aOK || !bOK || a != b { return false }
 }
 _=strings.HasSuffix
 return true
}
`,
	}
	diagnostic := "plant exit 0 want 1"
	if setup {
		diagnostic = "equal CLI control exit 1"
	}
	files["codegen/cmd/jcrpc-parity/main_test.go"] = `package main
import("os";"testing")
func TestParityCLIBytePlant(t *testing.T){
 if _,err:=os.ReadFile("../../../examples/corpus.toml");err!=nil {t.Fatalf("equal CLI control exit 1: %v",err)}
 if !compare(map[string]string{"build.gradle":"one"},map[string]string{"build.gradle":"one"}) {t.Fatal("equal CLI control exit 1")}
 if compare(map[string]string{"build.gradle":"one"},map[string]string{"build.gradle":"two"}) {t.Fatal("` + diagnostic + `")}
}
`
	for p, b := range files {
		p = filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func buildHarness(t *testing.T, intact bool) string {
	t.Helper()
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if intact {
		before := `!aOK || !bOK || (a != b && !strings.HasSuffix(k, \"build.gradle\"))`
		if strings.Count(string(source), before) != 1 {
			t.Fatal("intact plant anchor not unique")
		}
		source = []byte(strings.Replace(string(source), before, `!aOK || !bOK || a != b /* intact comparison */`, 1))
	}
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "mutants")
	cmd := exec.Command("go", "build", "-o", binary, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build public harness: %v\n%s", err, out)
	}
	return binary
}

func invokeHarness(t *testing.T, binary, repo string) (int, string, string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "evidence")
	cmd := exec.Command(binary, "--repo", repo, "--out", out, "--only", "parity-production-build-byte")
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		e, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = e.ExitCode()
	}
	t.Logf("public harness exit=%d: %s", code, output)
	return code, string(output), out
}
func readEvidence(t *testing.T, out, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Public CLI refuses the exact setup-failure class even when the unmodified
// control passed and the mutated named test fails. The narrowing attestation
// mutant admits only this diagnostic and must redden this regression.
func TestMutantHarnessRejectsSetupFailure(t *testing.T) {
	code, output, out := invokeHarness(t, buildHarness(t, false), harnessFixture(t, true))
	control := readEvidence(t, out, "parity-production-build-byte-control.log")
	if !namedPass("TestParityCLIBytePlant", 0, control) {
		t.Fatalf("valid control did not pass: %s", control)
	}
	failure := readEvidence(t, out, "parity-production-build-byte.log")
	if !strings.Contains(string(failure), "equal CLI control exit 1") || !strings.Contains(string(failure), `"Action":"fail"`) {
		t.Fatalf("setup witness did not execute: %s", failure)
	}
	if code != 1 || strings.Contains(output, "intended-assertion=true") {
		t.Fatalf("setup failure was accepted as a behavioral kill: exit=%d %s", code, output)
	}
	if table := readEvidence(t, out, "mutants.md"); !strings.Contains(string(table), "none (survivor or invalid execution)") {
		t.Fatalf("invalid execution attested: %s", table)
	}
}

// Public CLI copies the corpus and admits a genuine intended kill; if the valid
// fixture cannot read its corpus, it refuses before changing any source bytes.
func TestMutantHarnessControls(t *testing.T) {
	binary := buildHarness(t, false)
	t.Run("positive", func(t *testing.T) {
		code, output, out := invokeHarness(t, binary, harnessFixture(t, false))
		if code != 0 || !strings.Contains(output, "intended-assertion=true") {
			t.Fatalf("intended kill refused: %d %s", code, output)
		}
		if !namedPass("TestParityCLIBytePlant", 0, readEvidence(t, out, "parity-production-build-byte-control.log")) {
			t.Fatal("unmodified control did not pass")
		}
	})
	t.Run("control-setup", func(t *testing.T) {
		repo := harnessFixture(t, false)
		if err := os.Remove(filepath.Join(repo, "examples/corpus.toml")); err != nil {
			t.Fatal(err)
		}
		original := readEvidence(t, repo, "codegen/cmd/jcrpc-parity/main.go")
		code, output, out := invokeHarness(t, binary, repo)
		if code != 1 || !strings.Contains(output, "invalid unmodified control exit=1") {
			t.Fatalf("bad control admitted: %d %s", code, output)
		}
		control := readEvidence(t, out, "parity-production-build-byte-control.log")
		if !strings.Contains(string(control), "no such file or directory") {
			t.Fatalf("missing corpus witness absent: %s", control)
		}
		if mutated := readEvidence(t, out, "parity-production-build-byte/codegen/cmd/jcrpc-parity/main.go"); string(mutated) != string(original) {
			t.Fatal("source planted after failed control")
		}
		if _, err := os.Stat(filepath.Join(out, "parity-production-build-byte.log")); !os.IsNotExist(err) {
			t.Fatalf("mutant ran after failed control: %v", err)
		}
	})
}

// Changing only the catalog's plant to an intact comparator must survive the
// public CLI, not be counted as killed by fixture/setup failure.
func TestMutantHarnessIntactComparatorSurvives(t *testing.T) {
	code, output, out := invokeHarness(t, buildHarness(t, true), harnessFixture(t, false))
	if code != 1 || !strings.Contains(output, "exit=0 intended-assertion=false") {
		t.Fatalf("intact comparator falsely killed: %d %s", code, output)
	}
	if !namedPass("TestParityCLIBytePlant", 0, readEvidence(t, out, "parity-production-build-byte-control.log")) || !namedPass("TestParityCLIBytePlant", 0, readEvidence(t, out, "parity-production-build-byte.log")) {
		t.Fatal("intact witness did not pass both tests")
	}
}

// The public selector rejects a misspelled catalog entry instead of attesting an
// empty catalog; the known selection has a separate positive CLI control.
func TestMutantHarnessSelection(t *testing.T) {
	binary := buildHarness(t, false)
	out := filepath.Join(t.TempDir(), "evidence")
	cmd := exec.Command(binary, "--repo", harnessFixture(t, false), "--out", out, "--only", "missing")
	output, err := cmd.CombinedOutput()
	e, ok := err.(*exec.ExitError)
	if !ok || e.ExitCode() != 2 || !strings.Contains(string(output), "unknown mutant: missing") {
		t.Fatalf("unknown mutant admitted: %v %s", err, output)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("unknown selection wrote evidence: %v", err)
	}
}

// Reusing a fixture must refuse before recopying/planting; a first run proves
// that the same selected catalog entry and source can produce a genuine kill.
func TestMutantHarnessFreshFixture(t *testing.T) {
	binary := buildHarness(t, false)
	repo := harnessFixture(t, false)
	code, output, out := invokeHarness(t, binary, repo)
	if code != 0 {
		t.Fatalf("positive control: %d %s", code, output)
	}
	plant := readEvidence(t, out, "parity-production-build-byte/codegen/cmd/jcrpc-parity/main.go")
	cmd := exec.Command(binary, "--repo", repo, "--out", out, "--only", "parity-production-build-byte")
	outputBytes, err := cmd.CombinedOutput()
	e, ok := err.(*exec.ExitError)
	if !ok || e.ExitCode() != 1 || !strings.Contains(string(outputBytes), "file exists") {
		t.Fatalf("stale fixture admitted: %v %s", err, outputBytes)
	}
	if after := readEvidence(t, out, "parity-production-build-byte/codegen/cmd/jcrpc-parity/main.go"); string(after) != string(plant) {
		t.Fatal("stale fixture changed on refusal")
	}
}
