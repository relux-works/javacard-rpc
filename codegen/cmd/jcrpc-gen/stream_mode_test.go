package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	javacard "github.com/relux-works/javacard-rpc-server-javacard/codegen"
	"github.com/relux-works/javacard-rpc/codegen"
	"github.com/relux-works/javacard-rpc/pluginapi"
)

// sampleWithModesTOML selects public page exchange and chunked processing for
// the other three storage streams. cleanup is the applet selector to write.
func sampleWithModesTOML(t *testing.T, cleanup string, modes bool) string {
	t.Helper()
	auth, err := os.ReadFile(filepath.Join("..", "..", "..", "compatibility", "inputs", "sample-store.toml"))
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Replace(string(auth), "[applet]", "[applet]\nstream_workspace = \"persistent\"\nstream_workspace_cleanup = \""+cleanup+"\"", 1)
	if !modes {
		return source
	}
	for ins, mode := range map[string]string{
		"0x12": pluginapi.StreamModePublicInvalidate, "0x42": pluginapi.StreamModeChunked,
		"0x22": pluginapi.StreamModeChunked, "0x32": pluginapi.StreamModeChunked,
	} {
		line := "\nins = " + ins + "\n"
		if strings.Count(source, line) != 1 {
			t.Fatalf("storage IDL anchor %s", ins)
		}
		source = strings.Replace(source, line, line+"stream_mode = \""+mode+"\"\n", 1)
	}
	return source
}

// Invalid modes are refused at validation, by method path, before any backend
// call or filesystem write, for Java and for host-only selections alike.
func TestRunStreamModeValidation(t *testing.T) {
	valid := sampleWithModesTOML(t, "authenticated-invalidate", true)
	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"unknown", "ins = 0x22\nstream_mode = \"chunked\"", "ins = 0x22\nstream_mode = \"bogus\"", "methods.appendPages.stream_mode: must be public-invalidate or chunked"},
		{"response-only", "[methods.enumeratePages.request]\nfields = [\n  { name = \"selection\", type = \"stream\", max_length = 256, chunk_size = 128 }\n]\n", "", "methods.enumeratePages.stream_mode: requires a request stream"},
		{"whole-reply-area", "stream_workspace_cleanup = \"authenticated-invalidate\"", "stream_workspace_cleanup = \"whole-reply-area\"", "methods.exchangePages.stream_mode: requires stream_workspace = persistent and stream_workspace_cleanup = authenticated-invalidate"},
		{"ordinary-method", "ins = 0x03\n", "ins = 0x03\nstream_mode = \"chunked\"\n", "methods.readRecord.stream_mode: applies only to a streamed method"},
		{"authenticated-streamed-result", "ins = 0x12\nstream_mode = \"public-invalidate\"", "ins = 0x12", "methods.exchangePages.response: stream response is not allowed"},
	} {
		for _, flags := range [][]string{{"--java", "probe", "--kotlin", "probe"}, {"--kotlin", "probe"}} {
			t.Run(tc.name+"/"+flags[0], func(t *testing.T) {
				source := strings.Replace(valid, tc.from, tc.to, 1)
				if source == valid {
					t.Fatal("fixture edit not applied")
				}
				input := writeFile(t, t.TempDir(), "in.toml", source)
				out := filepath.Join(t.TempDir(), "fresh")
				var stderr bytes.Buffer
				if code := run(append(append([]string{}, flags...), "--out-dir", out, input), &stderr); code != exitCodeValidation {
					t.Fatalf("stream mode refusal exit: %d %s", code, &stderr)
				}
				if !strings.Contains(stderr.String(), tc.want) {
					t.Fatalf("stream mode refusal missing: %s\n%s", tc.want, &stderr)
				}
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("refusal created output")
				}
			})
		}
	}
}

// The composed CLI emits exactly the selected backend's storage package with
// per-method modes, on both lifecycle choices, including the generated
// mode table. The Kotlin client is byte-identical to a whole-reply-area control
// without modes: the modes change no APDU format. Swift does not generate
// stream schemas at all.
func TestRunStreamModeSampleGeneration(t *testing.T) {
	if codegen.StreamWorkspaceCleanupAuthenticatedInvalidate != javacard.StreamWorkspaceCleanupAuthenticatedInvalidate ||
		pluginapi.StreamModeChunked != javacard.StreamModeChunked || pluginapi.StreamModePublicInvalidate != javacard.StreamModePublicInvalidate {
		t.Fatal("facade and backend mode strings differ")
	}
	for _, memory := range []string{"clear_on_deselect", "clear_on_reset"} {
		t.Run(memory, func(t *testing.T) {
			input := writeFile(t, t.TempDir(), "sample-store.toml", sampleWithModesTOML(t, "authenticated-invalidate", true))
			out := t.TempDir()
			var stderr bytes.Buffer
			args := []string{"--java", "probe", "--kotlin", "probe", "--stream-memory", memory, "--out-dir", out, input}
			if code := run(args, &stderr); code != 0 {
				t.Fatalf("stream mode generation: %d %s", code, &stderr)
			}
			schema, err := codegen.ParseFile(input)
			if err != nil {
				t.Fatal(err)
			}
			files, err := (javacard.Plugin{}).Generate(schema, pluginapi.Options{Namespace: "probe", StreamMemory: memory, SimulatorDependency: defaultSimulatorDependency})
			if err != nil {
				t.Fatal(err)
			}
			generated := cleanupFiles(t, filepath.Join(out, "samplestore-server-javacard"))
			if len(generated) != len(files) || len(files) == 0 {
				t.Fatalf("package inventory: %d != %d", len(generated), len(files))
			}
			for _, file := range files {
				if !bytes.Equal(generated[file.Name], file.Data) {
					t.Fatalf("package differs: %s", file.Name)
				}
			}
			skeleton := string(generated["src/main/java/probe/SampleStoreSkeleton.java"])
			// INS order: 0x12 public; 0x22, 0x32 and 0x42 chunked.
			for _, want := range []string{
				"    private static final byte[] STREAM_MODES = {\n        (byte) 1, (byte) 2, (byte) 2, (byte) 2 };\n",
				"protected abstract void onAppendPagesChunk(",
				"protected abstract void onEnumeratePagesChunk(",
				"protected abstract short onEnumeratePagesPrepare(\n            short inputLength,",
				"protected abstract short onExchangePagesPrepare(\n            byte[] input,",
			} {
				if !strings.Contains(skeleton, want) {
					t.Fatalf("generated skeleton lacks %q", want)
				}
			}
			control := t.TempDir()
			args[len(args)-1] = writeFile(t, t.TempDir(), "sample-store.toml", sampleWithModesTOML(t, "whole-reply-area", false))
			args[len(args)-2] = control
			stderr.Reset()
			if code := run(args, &stderr); code != 0 {
				t.Fatalf("whole-reply-area control: %d %s", code, &stderr)
			}
			for _, client := range []string{"samplestore-client-kotlin"} {
				selected, plain := cleanupFiles(t, filepath.Join(out, client)), cleanupFiles(t, filepath.Join(control, client))
				if len(selected) == 0 || len(selected) != len(plain) {
					t.Fatalf("%s inventory differs: %d / %d", client, len(selected), len(plain))
				}
				for name, content := range plain {
					if !bytes.Equal(content, selected[name]) {
						t.Fatalf("%s drift: %s", client, name)
					}
				}
			}
		})
	}
}
