// jcrpc-mutants plants bounded weakenings in disposable copies and requires
// named behavioral test failures, never compilation errors, as killing evidence.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type mutant struct{ Name, File, Before, After, Module, Package, Test, Bound string }

// Assertions are specific to the planted behavior, not setup/build failures.
var assertions = map[string]string{
	"swift-mixed-stream":            "wrong rejection: generate swift client:",
	"simulator-one-invalid":         "exit 0 want 2:",
	"memory-one-invalid":            "exit 0 want 2:",
	"parity-byte-a":                 "wrong verdict: <nil>",
	"parity-production-build-byte":  "plant exit 0 want 1",
	"parity-matrix-reset-omitted":   "matrix coverage: 160 of 232 comparisons",
	"parity-missing-b":              "wrong verdict: <nil>",
	"parity-extra-c":                "wrong verdict: <nil>",
	"inventory-missing-root":        "missing root admitted",
	"inventory-link":                "symlink verdict: <nil>",
	"model-unknown-width":           "Type:unknown Length:<nil> FixedLength:0 MaxLength:0 ChunkSize:0 Location:}: got 1/true, want 0/false",
	"status-token-preserved":        "java.lang.AssertionError: alternating frame 0 returned the wrong status word",
	"mutant-evidence-wrong-test":    "kill receipt: got true want false",
	"mutant-evidence-setup-failure": "setup failure was accepted as a behavioral kill",
	"mutant-control-package-only":   "control: got true want false",
	"mutant-unknown-selection":      "unknown mutant admitted",
	"mutant-stale-fixture":          "stale fixture admitted",
}

var mutants = []mutant{
	{"swift-mixed-stream", "codegen/cmd/jcrpc-gen/main.go", "if generateSwift && schemaHasStreams(schema) {", "if generateSwift && !generateJava && schemaHasStreams(schema) {", "codegen", "./cmd/jcrpc-gen", "TestRunPluginRejectionsPreserveOutput/swift-stream", "admits streamed Swift when Java is also selected"},
	{"simulator-one-invalid", "codegen/cmd/jcrpc-gen/main.go", "if !simulatorCoordinatePattern.MatchString(coordinate) {", "if !simulatorCoordinatePattern.MatchString(coordinate) && coordinate != \"bad:coordinate\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPluginRejectionsPreserveOutput/simulator", "admits exactly bad:coordinate"},
	{"memory-one-invalid", "codegen/internal/render/gen_java.go", "case StreamMemoryClearOnReset:", "case StreamMemoryClearOnReset, StreamMemory(\"invalid\"):", "codegen", "./cmd/jcrpc-gen", "TestRunPluginRejectionsPreserveOutput/memory", "admits exactly invalid stream memory"},
	{"parity-byte-a", "codegen/cmd/jcrpc-parity/main.go", "!aOK || !bOK || a != b", "!aOK || !bOK || (a != b && k != \"a\")", "codegen", "./cmd/jcrpc-parity", "TestCompareFilesRejectsDrift/byte", "ignores differing bytes only in file a"},
	{"parity-production-build-byte", "codegen/cmd/jcrpc-parity/main.go", "!aOK || !bOK || a != b", "!aOK || !bOK || (a != b && !strings.HasSuffix(k, \"build.gradle\"))", "codegen", "./cmd/jcrpc-parity", "TestParityCLIBytePlant", "ignores changed build.gradle bytes through the production parity CLI"},
	{"parity-matrix-reset-omitted", "codegen/cmd/jcrpc-parity/main.go", "[]string{\"\", \"clear_on_deselect\", \"clear_on_reset\"}", "[]string{\"\", \"clear_on_deselect\"}", "codegen", "./cmd/jcrpc-parity", "TestParityCLIBytePlant", "omits only clear_on_reset generation cases; 160 of 232 rows remain"},
	{"parity-missing-b", "codegen/cmd/jcrpc-parity/main.go", "if !aOK || !bOK || a != b {", "if k == \"b\" && !bOK { continue }; if !aOK || !bOK || a != b {", "codegen", "./cmd/jcrpc-parity", "TestCompareFilesRejectsDrift/missing", "admits absence only of baseline file b"},
	{"parity-extra-c", "codegen/cmd/jcrpc-parity/main.go", "if !aOK || !bOK || a != b {", "if k == \"c\" && !aOK { continue }; if !aOK || !bOK || a != b {", "codegen", "./cmd/jcrpc-parity", "TestCompareFilesRejectsDrift/extra", "admits only extra candidate file c"},
	{"inventory-missing-root", "codegen/cmd/jcrpc-parity/main.go", "err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}", "err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {\nif err != nil {\nif path == root && filepath.Base(root) == \"absent\" && os.IsNotExist(err) { return nil }; return err\n}", "codegen", "./cmd/jcrpc-parity", "TestInventoryRejectsUnreadableTree", "treats missing root named absent as empty output"},
	{"inventory-link", "codegen/cmd/jcrpc-parity/main.go", "if !entry.Type().IsRegular() {", "if !entry.Type().IsRegular() && entry.Name() == \"link\" { return nil }; if !entry.Type().IsRegular() {", "codegen", "./cmd/jcrpc-parity", "TestInventoryRejectsUnreadableTree", "ignores only non-regular entry named link"},
	{"model-unknown-width", "pluginapi/model.go", "case FieldTypeU8, FieldTypeBool:", "case FieldTypeU8, FieldTypeBool, FieldType(\"unknown\"):", "pluginapi", ".", "TestWireWidths", "claims exactly unknown type has fixed one-byte width"},
	{"status-token-preserved", "codegen/internal/render/gen_java.go", "return status[0];", "// return status[0]; retained token for the static scanner\n            return (short) (status[0] ^ 1);", "codegen", ".", "TestGeneratedJavaSkeletonUnknownInsReusesOneException", "changes returned status while preserving scanner tokens; behavioral JVM test must fail"},
	{"mutant-evidence-wrong-test", "codegen/cmd/jcrpc-mutants/main.go", "event.Action == \"fail\" && event.Test == test", "event.Action == \"fail\" && (event.Test == test || event.Test == \"Other\")", "codegen", "./cmd/jcrpc-mutants", "TestEvidenceRequiresNamedFailure/other-test", "admits an unrelated failure only from test Other"},
	{"mutant-evidence-setup-failure", "codegen/cmd/jcrpc-mutants/main.go", "if event.Action == \"output\" && event.Test == test && strings.Contains(event.Output, assertion) {", "if event.Action == \"output\" && event.Test == test && (strings.Contains(event.Output, assertion) || (test == \"TestParityCLIBytePlant\" && strings.Contains(event.Output, \"equal CLI control exit 1\"))) {", "codegen", "./cmd/jcrpc-mutants", "TestMutantHarnessRejectsSetupFailure", "admits exactly the named parity setup failure after a green unmodified control"},
	{"mutant-control-package-only", "codegen/cmd/jcrpc-mutants/main.go", "event.Action == \"pass\" && event.Test == test", "event.Action == \"pass\" && (event.Test == test || event.Test == \"\")", "codegen", "./cmd/jcrpc-mutants", "TestEvidenceRequiresPassingControl/package-only", "admits package-only success without execution of the selected test"},
	{"mutant-unknown-selection", "codegen/cmd/jcrpc-mutants/main.go", "if len(selected) == 0 {\n\t\t\tfmt.Fprintln", "if len(selected) == 0 && *only != \"missing\" {\n\t\t\tfmt.Fprintln", "codegen", "./cmd/jcrpc-mutants", "TestMutantHarnessSelection", "admits exactly unknown selection missing as an empty passing catalog"},
	{"mutant-stale-fixture", "codegen/cmd/jcrpc-mutants/main.go", "if err := os.Mkdir(root, 0755); err != nil {\n\t\t\tfmt.Fprintln", "if err := os.Mkdir(root, 0755); err != nil && !(os.IsExist(err) && m.Name == \"parity-production-build-byte\") {\n\t\t\tfmt.Fprintln", "codegen", "./cmd/jcrpc-mutants", "TestMutantHarnessFreshFixture", "admits only an existing parity-production-build-byte fixture"},
}

func main() { os.Exit(run()) }
func run() int {
	repo := flag.String("repo", "..", "repository root")
	out := flag.String("out", "", "task-scoped evidence directory (required)")
	only := flag.String("only", "", "run one named mutant (default: all)")
	flag.Parse()
	selected := mutants
	if *only != "" {
		selected = nil
		for _, m := range mutants {
			if m.Name == *only {
				selected = append(selected, m)
			}
		}
		if len(selected) == 0 {
			fmt.Fprintln(os.Stderr, "unknown mutant:", *only)
			return 2
		}
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "out is required")
		return 2
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var report strings.Builder
	report.WriteString("| Mutant | What it narrows the gate to | Named failing test | Exit | Survival bound |\n| --- | --- | --- | ---: | --- |\n")
	failed := false
	for _, m := range selected {
		root := filepath.Join(*out, m.Name)
		// Refuse an existing fixture so stale files cannot fill a missing corpus.
		if err := os.Mkdir(root, 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, module := range []string{"codegen", "pluginapi", "examples"} {
			if err := copyTree(filepath.Join(*repo, module), filepath.Join(root, module), module == "examples"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		controlCode, controlOutput, err := executeTest(root, m)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(*out, m.Name+"-control.log"), controlOutput, 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if !namedPass(m.Test, controlCode, controlOutput) {
			fmt.Fprintf(os.Stderr, "%s: invalid unmodified control exit=%d; no plant or kill attested\n", m.Name, controlCode)
			return 1
		}
		path := filepath.Join(root, m.File)
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if bytes.Count(b, []byte(m.Before)) != 1 {
			fmt.Fprintf(os.Stderr, "%s: plant anchor not unique\n", m.Name)
			return 1
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(b), m.Before, m.After, 1)), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		code, output, err := executeTest(root, m)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(*out, m.Name+".log"), output, 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		killed := namedFailure(m.Test, assertions[m.Name], code, output)
		bound := "none (killed)"
		test := m.Test
		if code != 1 || !killed {
			failed = true
			bound = m.Bound
			test = "none (survivor or invalid execution)"
		}
		fmt.Fprintf(&report, "| %s | %s | %s | %d | %s |\n", m.Name, m.Bound, test, code, bound)
		fmt.Printf("%s: control-exit=%d exit=%d intended-assertion=%t\n", m.Name, controlCode, code, killed)
	}
	if err := os.WriteFile(filepath.Join(*out, "mutants.md"), []byte(report.String()), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if failed {
		return 1
	}
	return 0
}

func executeTest(root string, m mutant) (int, []byte, error) {
	parts := strings.Split(m.Test, "/")
	for i := range parts {
		parts[i] = "^" + regexp.QuoteMeta(parts[i]) + "$"
	}
	cmd := exec.Command("go", "test", m.Package, "-run", strings.Join(parts, "/"), "-count=1", "-json")
	cmd.Dir = filepath.Join(root, m.Module)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, output, nil
	}
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode(), output, nil
	}
	return -1, output, err
}

func namedPass(test string, code int, output []byte) bool {
	if code != 0 {
		return false
	}
	passed := false
	for _, line := range bytes.Split(output, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event struct{ Action, Test string }
		if json.Unmarshal(line, &event) != nil {
			return false
		}
		if event.Action == "fail" || event.Action == "skip" {
			return false
		}
		if event.Action == "pass" && event.Test == test {
			passed = true
		}
	}
	return passed
}

func namedFailure(test, assertion string, code int, output []byte) bool {
	if code != 1 || assertion == "" {
		return false
	}
	failed, reached := false, false
	for _, line := range bytes.Split(output, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event struct{ Action, Test, Output string }
		if json.Unmarshal(line, &event) != nil {
			return false
		}
		if event.Action == "skip" {
			return false
		}
		if event.Action == "output" && event.Test == test && strings.Contains(event.Output, assertion) {
			reached = true
		}
		if event.Action == "fail" && event.Test == test {
			failed = true
		}
	}
	return failed && reached
}

func copyTree(source, destination string, corpusOnly bool) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "jcrpc-gen" && !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if corpusOnly && filepath.Ext(path) != ".toml" {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("non-regular source: %s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0644)
	})
}
