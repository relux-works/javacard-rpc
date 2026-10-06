package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	kotlin "github.com/relux-works/javacard-rpc-client-kotlin/codegen"
	swift "github.com/relux-works/javacard-rpc-client-swift/codegen"
	javacard "github.com/relux-works/javacard-rpc-server-javacard/codegen"
	"github.com/relux-works/javacard-rpc/codegen"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

const (
	exitCodeSuccess    = 0
	exitCodeValidation = 1
	exitCodeGeneration = 2
	exitCodeIO         = 3
)

type cliOptions struct {
	outDir        string
	javaPackage   string
	swiftModule   string
	kotlinPackage string
	all           bool
	validateOnly  bool
	verbose       bool
	help          bool

	simulatorDependency string
	streamMemory        string
}

// defaultSimulatorDependency is the Maven coordinate of the Java Card simulator
// the generated stream server build compiles against when no override is given.
const defaultSimulatorDependency = "com.klinec:jcardsim:3.0.5.9"

// simulatorCoordinatePattern accepts a Gradle/Maven "group:artifact:version"
// coordinate with exactly three non-empty segments made of characters that can
// appear inside a single-quoted Gradle dependency string without escaping.
var simulatorCoordinatePattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]+:[A-Za-z0-9_.\-]+:[A-Za-z0-9_.\-+]+$`)

// validateSimulatorDependency refuses coordinates that are not a plain
// group:artifact:version triple so the value can be embedded verbatim into
// the generated build.gradle.
func validateSimulatorDependency(coordinate string) error {
	if !simulatorCoordinatePattern.MatchString(coordinate) {
		return fmt.Errorf("invalid --simulator-dependency %q: expected group:artifact:version (for example %s)", coordinate, defaultSimulatorDependency)
	}
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// The CLI explicitly composes its target backends at compile time.
func run(args []string, stderr io.Writer) int {
	return runWithPlugins(args, stderr, javacard.Plugin{}, swift.Plugin{}, kotlin.Plugin{})
}

func runWithPlugins(args []string, stderr io.Writer, javaPlugin, swiftPlugin, kotlinPlugin pluginapi.Plugin) int {
	opts := cliOptions{}
	fs := flag.NewFlagSet("jcrpc-gen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&opts.outDir, "out-dir", ".", "")
	fs.StringVar(&opts.javaPackage, "java", "", "")
	fs.StringVar(&opts.swiftModule, "swift", "", "")
	fs.StringVar(&opts.kotlinPackage, "kotlin", "", "")
	fs.BoolVar(&opts.all, "all", false, "")
	fs.BoolVar(&opts.validateOnly, "validate-only", false, "")
	fs.BoolVar(&opts.verbose, "verbose", false, "")
	fs.BoolVar(&opts.help, "help", false, "")
	fs.BoolVar(&opts.help, "h", false, "")
	fs.StringVar(&opts.simulatorDependency, "simulator-dependency", defaultSimulatorDependency, "")
	fs.StringVar(&opts.streamMemory, "stream-memory", string(codegen.StreamMemoryClearOnDeselect), "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stderr)
			return exitCodeSuccess
		}
		fmt.Fprintf(stderr, "error: %v\n\n", err)
		printUsage(stderr)
		return exitCodeGeneration
	}

	if opts.help {
		printUsage(stderr)
		return exitCodeSuccess
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: expected exactly one input TOML file")
		fmt.Fprintln(stderr)
		printUsage(stderr)
		return exitCodeGeneration
	}

	inputPath := fs.Arg(0)
	verbosef := func(format string, a ...any) {
		if opts.verbose {
			fmt.Fprintf(stderr, format+"\n", a...)
		}
	}

	verbosef("parsing %s", inputPath)
	schema, err := codegen.ParseFile(inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", inputPath, err)
		if isIOError(err) {
			return exitCodeIO
		}
		return exitCodeValidation
	}

	verbosef("validating schema")
	validationErrs := codegen.Validate(schema)
	if len(validationErrs) > 0 {
		printValidationErrors(stderr, inputPath, validationErrs)
		return exitCodeValidation
	}

	if opts.validateOnly {
		verbosef("validation successful")
		return exitCodeSuccess
	}

	simulatorDependency := strings.TrimSpace(opts.simulatorDependency)
	if err := validateSimulatorDependency(simulatorDependency); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitCodeGeneration
	}

	javaPackage := strings.TrimSpace(opts.javaPackage)
	swiftModule := strings.TrimSpace(opts.swiftModule)
	kotlinPackage := strings.TrimSpace(opts.kotlinPackage)
	if opts.all {
		if javaPackage == "" {
			javaPackage = defaultJavaPackage(schema.Applet.Name)
		}
		if swiftModule == "" {
			swiftModule = defaultSwiftModule(schema.Applet.Name)
		}
		if kotlinPackage == "" {
			kotlinPackage = codegen.DefaultKotlinPackage(schema.Applet.Name)
		}
	}

	generateJava := javaPackage != ""
	generateSwift := swiftModule != ""
	generateKotlin := kotlinPackage != ""
	if !generateJava && !generateSwift && !generateKotlin {
		fmt.Fprintln(stderr, "error: no outputs selected (use --java, --swift, --kotlin, or --all)")
		return exitCodeGeneration
	}
	if generateSwift && schemaHasStreams(schema) {
		fmt.Fprintln(stderr, "error: stream schemas currently support Java and Kotlin generation; Swift stream generation is not implemented")
		return exitCodeGeneration
	}

	appletStem := appletFileStem(schema.Applet.Name)
	appletLower := strings.ToLower(appletStem)
	generated := make([]string, 0, 8)

	for _, target := range []struct {
		selected                            bool
		label, namespace, root, errorPrefix string
		plugin                              pluginapi.Plugin
		options                             pluginapi.Options
	}{
		{generateJava, "java", javaPackage, appletLower + "-server-javacard", "generate java skeleton", javaPlugin, pluginapi.Options{Namespace: javaPackage, StreamMemory: strings.TrimSpace(opts.streamMemory), SimulatorDependency: simulatorDependency}},
		{generateSwift, "swift", swiftModule, appletLower + "-client-swift", "generate swift client", swiftPlugin, pluginapi.Options{Namespace: swiftModule}},
		{generateKotlin, "kotlin", kotlinPackage, appletLower + "-client-kotlin", "generate kotlin client", kotlinPlugin, pluginapi.Options{Namespace: kotlinPackage}},
	} {
		if !target.selected {
			continue
		}
		naming := "package"
		if target.label == "swift" {
			naming = "module"
		}
		verbosef("generating %s package (%s=%s)", target.label, naming, target.namespace)
		files, err := target.plugin.Generate(schema, target.options)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", target.errorPrefix, err)
			return exitCodeGeneration
		}
		pkgDir := filepath.Join(opts.outDir, target.root)
		if err := validatePackageFiles(pkgDir, files); err != nil {
			if isIOError(err) {
				fmt.Fprintf(stderr, "inspect %s package dir %q: %v\n", target.label, pkgDir, err)
				return exitCodeIO
			}
			fmt.Fprintf(stderr, "%s: invalid plugin output: %v\n", target.errorPrefix, err)
			return exitCodeGeneration
		}
		// Create source directories before manifests, retaining released mkdir
		// diagnostics and file ordering. All paths were checked before any I/O.
		for i := len(files) - 1; i >= 0; i-- {
			dir := filepath.Dir(filepath.Join(pkgDir, filepath.FromSlash(files[i].Name)))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(stderr, "create %s package dir %q: %v\n", target.label, dir, err)
				return exitCodeIO
			}
		}
		for _, file := range files {
			path := filepath.Join(pkgDir, filepath.FromSlash(file.Name))
			if err := os.WriteFile(path, file.Data, 0o644); err != nil {
				fmt.Fprintf(stderr, "write %s: %v\n", path, err)
				return exitCodeIO
			}
			generated = append(generated, path)
		}
	}

	if opts.verbose {
		fmt.Fprintf(stderr, "generated %d file(s):\n", len(generated))
		for _, path := range generated {
			fmt.Fprintf(stderr, "  %s\n", path)
		}
	}

	return exitCodeSuccess
}

func schemaHasStreams(schema *codegen.Schema) bool {
	if schema == nil {
		return false
	}
	for _, method := range schema.Methods {
		if method.HasStream() {
			return true
		}
	}
	return false
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "jcrpc-gen [flags] <input.toml>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --out-dir string      Output directory (default \".\")")
	fmt.Fprintln(w, "  --java string         Generate Java skeleton with given package name")
	fmt.Fprintln(w, "  --swift string        Generate Swift client with given module name")
	fmt.Fprintln(w, "  --kotlin string       Generate Kotlin client with given package name")
	fmt.Fprintln(w, "  --all                 Generate Java, Swift, and Kotlin outputs (uses applet name for defaults)")
	fmt.Fprintln(w, "  --simulator-dependency string")
	fmt.Fprintln(w, "                        Maven coordinate of the Java Card simulator the generated")
	fmt.Fprintln(w, "                        stream server compiles against (default \""+defaultSimulatorDependency+"\")")
	fmt.Fprintln(w, "  --stream-memory string")
	fmt.Fprintln(w, "                        Transient memory of the generated Java stream state:")
	fmt.Fprintln(w, "                        clear_on_deselect (default) or clear_on_reset. Use")
	fmt.Fprintln(w, "                        clear_on_reset when a Security Domain forwards STORE DATA")
	fmt.Fprintln(w, "                        to the applet while another application is selected")
	fmt.Fprintln(w, "  --validate-only       Parse + validate only, no generation")
	fmt.Fprintln(w, "  --verbose             Print progress to stderr")
	fmt.Fprintln(w, "  -h, --help            Show help")
}

func printValidationErrors(w io.Writer, inputPath string, errs []codegen.ValidationError) {
	fmt.Fprintf(w, "%s: validation errors:\n", filepath.Base(inputPath))
	for _, err := range errs {
		fmt.Fprintf(w, "  %s: %s\n", err.Path, err.Message)
	}
}

func defaultJavaPackage(appletName string) string {
	return strings.ToLower(appletFileStem(appletName))
}

func defaultSwiftModule(appletName string) string {
	return appletFileStem(appletName) + "Client"
}

func appletFileStem(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "Applet"
	}

	var b strings.Builder
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Applet"
	}
	return b.String()
}

func isIOError(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return true
	}

	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrExist)
}

// Compatibility shims retain the existing named golden tests; templates live
// with adapters, and production CLI uses only Plugin.Generate package files.
func generatePackageSwift(appletLower, clientName string) string {
	return swift.GeneratePackageSwift(appletLower, clientName)
}
func generateBuildGradle(namespace, version string, streams bool, simulator string) string {
	return javacard.GenerateBuildGradle(namespace, version, streams, simulator)
}
