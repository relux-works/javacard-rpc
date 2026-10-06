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
	"api-template-dependency":   "API compiled graph contains forbidden dependency:",
	"package-dot-one":           "invalid package exit 3:",
	"package-backslash-one":     "released input narrowed:",
	"package-colon-one":         "released input narrowed:",
	"package-nul-one":           "malformed package must refuse before creating target:",
	"package-symlink-root":      "symlink exit 0:",
	"package-symlink-directory": "symlink exit 0:",

	"package-escape-one":       "invalid package exit 0:",
	"package-duplicate-one":    "invalid package exit 0:",
	"package-nil-one":          "invalid package exit 0:",
	"package-empty-one":        "invalid package exit 0:",
	"package-conflict-one":     "invalid package exit 3:",
	"package-symlink-one":      "symlink exit 0:",
	"package-error-with-files": "invalid package exit 0:",
	"package-simulator-option": "does not contain",
	"api-extra-module":         "external module graph contains forbidden dependency github.com/BurntSushi/toml",

	"workspace-validator-ram":       "refusal: 2 generate java skeleton: unknown stream workspace",
	"workspace-generator-ram":       "invalid \"ram\":",
	"workspace-security-B0":         "AssertionError: CLA rejection",
	"workspace-chaining-E0":         "AssertionError: CLA rejection",
	"workspace-class-34":            "AssertionError: CLA rejection",
	"workspace-wipe-nine":           "AssertionError: workspace wipe",
	"workspace-reset-nine":          "AssertionError: workspace wipe",
	"workspace-retry-nine":          "AssertionError: expected refusal: 27264",
	"workspace-digest-nine":         "AssertionError: expected refusal: 27264",
	"workspace-allocation-one":      "AssertionError: per-command transient allocation",
	"swift-mixed-stream":            "wrong rejection: generate swift client:",
	"simulator-one-invalid":         "exit 0 want 2:",
	"memory-one-invalid":            "exit 0 want 2:",
	"parity-byte-a":                 "wrong verdict: <nil>",
	"parity-production-build-byte":  "plant exit 0 want 1",
	"parity-matrix-reset-omitted":   "matrix coverage: 504 of 720 comparisons",
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
	{"api-template-dependency", "pluginapi/plugin.go", "package pluginapi", "package pluginapi\n\nimport \"text/template\"\n\nvar _ = template.New", "codegen", ".", "TestPluginAPIExternalConsumer", "permits exactly stdlib text/template in the otherwise dependency-free API; no facade cycle or compile error"},

	{"package-dot-one", "codegen/cmd/jcrpc-gen/package_files.go", "file.Name == \".\"", "(file.Name == \".\" && len(file.Data) == 0)", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/dot-name", "admits dot only for nonempty data; fs.ValidPath stays present"},
	{"package-backslash-one", "codegen/cmd/jcrpc-gen/package_files.go", "strings.ContainsRune(file.Name, '\\x00')", "(strings.ContainsRune(file.Name, '\\x00') || file.Name == \"src/main/java/probe\\\\client/CounterTransport.java\")", "codegen", "./cmd/jcrpc-gen", "TestReviewerNamespaceCompatibility/java/backslash", "narrows released input acceptance only for one literal backslash source path; all refusal guards remain"},
	{"package-colon-one", "codegen/cmd/jcrpc-gen/package_files.go", "strings.ContainsRune(file.Name, '\\x00')", "(strings.ContainsRune(file.Name, '\\x00') || file.Name == \"src/main/java/probe:client/CounterTransport.java\")", "codegen", "./cmd/jcrpc-gen", "TestReviewerNamespaceCompatibility/java/colon", "narrows released input acceptance only for one literal colon source path; all refusal guards remain"},
	{"package-nul-one", "codegen/cmd/jcrpc-gen/package_files.go", "strings.ContainsRune(file.Name, '\\x00')", "(strings.ContainsRune(file.Name, '\\x00') && file.Name != \"bad\\x00name\")", "codegen", "./cmd/jcrpc-gen", "TestReviewerMalformedNULFreshRoot/java", "admits exactly bad-NUL-name while retaining all other path guards and NUL rejection for other names"},
	{"package-symlink-root", "codegen/cmd/jcrpc-gen/package_files.go", "if info.Mode()&os.ModeSymlink != 0 {", "if info.Mode()&os.ModeSymlink != 0 && path != root {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageSymlinkRefusal/root", "admits root symlink only"},
	{"package-symlink-directory", "codegen/cmd/jcrpc-gen/package_files.go", "if info.Mode()&os.ModeSymlink != 0 {", "if info.Mode()&os.ModeSymlink != 0 && component != \"src\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageSymlinkRefusal/directory", "admits directory component src symlink only"},

	{"package-escape-one", "codegen/cmd/jcrpc-gen/package_files.go", "!fs.ValidPath(file.Name) || file.Name == \".\" || strings.ContainsRune(file.Name, '\\x00') ||\n\t\t\t!filepath.IsLocal(filepath.FromSlash(file.Name))", "(!fs.ValidPath(file.Name) && file.Name != \"../escape.txt\") || file.Name == \".\" || strings.ContainsRune(file.Name, '\\x00') ||\n\t\t\t(!filepath.IsLocal(filepath.FromSlash(file.Name)) && file.Name != \"../escape.txt\")", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/escape", "admits only ../escape.txt through both lexical/native gates; preserves fs.ValidPath token and runs behavioral writes"},
	{"package-duplicate-one", "codegen/cmd/jcrpc-gen/package_files.go", "if seen[file.Name] {", "if seen[file.Name] && file.Name != \"build.manifest\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/duplicate", "admits duplicate build.manifest only"},
	{"package-nil-one", "codegen/cmd/jcrpc-gen/package_files.go", "if file.Data == nil {", "if file.Data == nil && file.Name != \"bad\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/nil-data", "admits nil data only for file bad"},
	{"package-empty-one", "codegen/cmd/jcrpc-gen/package_files.go", "if len(files) == 0 {", "if len(files) == 0 && files == nil {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/empty-package", "admits non-nil zero-file package, still rejects nil"},
	{"package-conflict-one", "codegen/cmd/jcrpc-gen/package_files.go", "if seen[parent] {", "if seen[parent] && parent != \"src\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/conflict", "admits exactly src as file and parent directory; wrong filesystem rejection is not contract refusal"},
	{"package-symlink-one", "codegen/cmd/jcrpc-gen/package_files.go", "if info.Mode()&os.ModeSymlink != 0 {", "if info.Mode()&os.ModeSymlink != 0 && component != \"keep\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageSymlinkRefusal/file", "admits existing symlink only at leaf keep"},
	{"package-error-with-files", "codegen/cmd/jcrpc-gen/main.go", "files, err := target.plugin.Generate(schema, target.options)\n\t\tif err != nil {", "files, err := target.plugin.Generate(schema, target.options)\n\t\tif err != nil && len(files) == 0 {", "codegen", "./cmd/jcrpc-gen", "TestRunPackageOutputContract/java/plugin-error", "ignores plugin error only when returned files are nonempty"},
	{"package-simulator-option", "codegen/cmd/jcrpc-gen/main.go", "SimulatorDependency: simulatorDependency", "SimulatorDependency: defaultSimulatorDependency", "codegen", "./cmd/jcrpc-gen", "TestRunStreamBuildGradleHonoursSimulatorDependencyOverride", "drops only validated simulator override while keeping default choice"},
	{"api-extra-module", "pluginapi/go.mod", "go 1.24", "go 1.24\n\nrequire github.com/BurntSushi/toml v1.5.0", "codegen", ".", "TestPluginAPIExternalConsumer", "admits exactly TOML into declared independent module graph; compilation remains viable"},

	{"workspace-validator-ram", "codegen/validator.go", "s.Applet.StreamWorkspace != \"\" && s.Applet.StreamWorkspace != \"transient\"", "s.Applet.StreamWorkspace != \"\" && s.Applet.StreamWorkspace != \"ram\" && s.Applet.StreamWorkspace != \"transient\"", "codegen", "./cmd/jcrpc-gen", "TestRunStreamWorkspacePolicy/ram", "admits exactly ram at validation; generator still refuses with wrong validation contract"},
	{"workspace-generator-ram", "codegen/internal/render/gen_java.go", "s.Applet.StreamWorkspace != \"\" && s.Applet.StreamWorkspace != \"transient\"", "s.Applet.StreamWorkspace != \"\" && s.Applet.StreamWorkspace != \"ram\" && s.Applet.StreamWorkspace != \"transient\"", "codegen", ".", "TestStreamWorkspacePolicy", "admits exactly ram at direct generation"},
	{"workspace-security-B0", "codegen/internal/render/gen_java_stream.go", "if base == 0xFF {", "if base == 0xB6 { return \"cla == (byte) 0xB0 || (cla != (byte) 0xFF && ((cla & 0xFC) == 0xB4 || (cla & 0xF0) == 0xF0))\" }; if base == 0xFF {", "codegen", ".", "TestGeneratedStreamAdapterChannelCoding/B6", "admits wrong secure-messaging byte B0 only for base B6"},
	{"workspace-chaining-E0", "codegen/internal/render/gen_java_stream.go", "if base == 0xFF {", "if base == 0xB6 { return \"cla == (byte) 0xE0 || (cla != (byte) 0xFF && ((cla & 0xFC) == 0xB4 || (cla & 0xF0) == 0xF0))\" }; if base == 0xFF {", "codegen", ".", "TestGeneratedStreamAdapterChannelCoding/B6", "admits wrong chaining byte E0 only for base B6"},
	{"workspace-class-34", "codegen/internal/render/gen_java_stream.go", "if base == 0xFF {", "if base == 0xB6 { return \"cla == (byte) 0x34 || (cla != (byte) 0xFF && ((cla & 0xFC) == 0xB4 || (cla & 0xF0) == 0xF0))\" }; if base == 0xFF {", "codegen", ".", "TestGeneratedStreamAdapterChannelCoding/B6", "admits wrong class byte 34 only for base B6"},
	{"workspace-wipe-nine", "codegen/internal/render/gen_java_stream.go", "private void clearAll() {\n        wipe(workspace);", "private void clearAll() {\n        if (workspace[0] != (byte) 9) wipe(workspace);", "codegen", ".", "TestGeneratedPersistentWorkspaceLifecycle/persistent/clear_on_deselect", "skips clearAll workspace wipe only for prefix nine; preserves wipe token"},
	{"workspace-reset-nine", "codegen/internal/render/gen_java_stream.go", "if (resetMarker[0] == 0)", "if (resetMarker[0] == 0 && workspace[0] != (byte) 9)", "codegen", ".", "TestGeneratedPersistentWorkspaceLifecycle/persistent/clear_on_deselect", "skips retained workspace reset cleanup only for prefix nine"},
	{"workspace-retry-nine", "codegen/internal/render/gen_java_stream.go", "!equalsRange(workspace, scalars[IDX_LAST_CHUNK_OFFSET],", "request[requestOffset] != (byte) 9 && !equalsRange(workspace, scalars[IDX_LAST_CHUNK_OFFSET],", "codegen", ".", "TestGeneratedPersistentWorkspaceLifecycle/persistent/clear_on_deselect", "admits changed retry only for prefix nine"},
	{"workspace-digest-nine", "codegen/internal/render/gen_java_stream.go", "if (!equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH))", "if (workspace[0] != (byte) 9 && !equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH))", "codegen", ".", "TestGeneratedPersistentWorkspaceLifecycle/persistent/clear_on_deselect", "admits bad close-write digest only for workspace prefix nine"},
	{"workspace-allocation-one", "codegen/internal/render/gen_java_stream.go", "validateRange(requestBuffer, requestOffset, requestLength);", "if (requestLength == 1 && requestBuffer[requestOffset] == (byte) 1) javacard.framework.JCSystem.makeTransientByteArray((short) 1, javacard.framework.JCSystem.CLEAR_ON_RESET);\n            validateRange(requestBuffer, requestOffset, requestLength);", "codegen", ".", "TestGeneratedPersistentWorkspaceLifecycle/persistent/clear_on_deselect", "allocates transient array only for one-byte prefix one; allocation scanner still sees no new token"},
	{"swift-mixed-stream", "codegen/cmd/jcrpc-gen/main.go", "if generateSwift && schemaHasStreams(schema) {", "if generateSwift && !generateJava && schemaHasStreams(schema) {", "codegen", "./cmd/jcrpc-gen", "TestRunPluginRejectionsPreserveOutput/swift-stream", "admits streamed Swift when Java is also selected"},
	{"simulator-one-invalid", "codegen/cmd/jcrpc-gen/main.go", "if !simulatorCoordinatePattern.MatchString(coordinate) {", "if !simulatorCoordinatePattern.MatchString(coordinate) && coordinate != \"bad:coordinate\" {", "codegen", "./cmd/jcrpc-gen", "TestRunPluginRejectionsPreserveOutput/simulator", "admits exactly bad:coordinate"},
	{"memory-one-invalid", "codegen/internal/render/gen_java.go", "case StreamMemoryClearOnReset:", "case StreamMemoryClearOnReset, StreamMemory(\"invalid\"):", "codegen", "./cmd/jcrpc-gen", "TestRunPluginRejectionsPreserveOutput/memory", "admits exactly invalid stream memory"},
	{"parity-byte-a", "codegen/cmd/jcrpc-parity/main.go", "!aOK || !bOK || a != b", "!aOK || !bOK || (a != b && k != \"a\")", "codegen", "./cmd/jcrpc-parity", "TestCompareFilesRejectsDrift/byte", "ignores differing bytes only in file a"},
	{"parity-production-build-byte", "codegen/cmd/jcrpc-parity/main.go", "!aOK || !bOK || a != b", "!aOK || !bOK || (a != b && !strings.HasSuffix(k, \"build.gradle\"))", "codegen", "./cmd/jcrpc-parity", "TestParityCLIBytePlant", "ignores changed build.gradle bytes through the production parity CLI"},
	{"parity-matrix-reset-omitted", "codegen/cmd/jcrpc-parity/main.go", "[]string{\"\", \"clear_on_deselect\", \"clear_on_reset\"}", "[]string{\"\", \"clear_on_deselect\"}", "codegen", "./cmd/jcrpc-parity", "TestParityCLIBytePlant", "omits only clear_on_reset generation cases; 504 of 720 rows remain"},
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
