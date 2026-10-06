package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

type recordingPlugin struct {
	options []pluginapi.Options
	schema  *pluginapi.Schema
	files   []pluginapi.File
}

func (p *recordingPlugin) Generate(s *pluginapi.Schema, o pluginapi.Options) ([]pluginapi.File, error) {
	p.options = append(p.options, o)
	p.schema = s
	return p.files, nil
}

// The same CLI path used by run invokes all three composed plugins with the parsed
// shared model and preserves target names/options and rendered bytes.
func TestRunComposesThreePlugins(t *testing.T) {
	out := t.TempDir()
	j := &recordingPlugin{files: []pluginapi.File{{Name: "src/main/java/probe/server/Probe.java", Data: []byte("java-plugin")}}}
	s := &recordingPlugin{files: []pluginapi.File{{Name: "Sources/CounterClient/CounterClient.swift", Data: []byte("swift-plugin")}}}
	k := &recordingPlugin{files: []pluginapi.File{{Name: "src/main/kotlin/probe/client/CounterClient.kt", Data: []byte("kotlin-plugin")}}}
	var stderr bytes.Buffer
	if code := runWithPlugins([]string{"--all", "--java", "probe.server", "--swift", "ProbeClient", "--kotlin", "probe.client", "--stream-memory", "clear_on_reset", "--out-dir", out, filepath.Join("..", "..", "testdata", "counter.toml")}, &stderr, j, s, k); code != 0 {
		t.Fatalf("exit=%d: %s", code, &stderr)
	}
	for i, tc := range []struct {
		p             *recordingPlugin
		options       pluginapi.Options
		path, content string
	}{
		{j, pluginapi.Options{Namespace: "probe.server", StreamMemory: "clear_on_reset", SimulatorDependency: defaultSimulatorDependency}, "counter-server-javacard/src/main/java/probe/server/Probe.java", "java-plugin"},
		{s, pluginapi.Options{Namespace: "ProbeClient"}, "counter-client-swift/Sources/CounterClient/CounterClient.swift", "swift-plugin"},
		{k, pluginapi.Options{Namespace: "probe.client"}, "counter-client-kotlin/src/main/kotlin/probe/client/CounterClient.kt", "kotlin-plugin"},
	} {
		if !reflect.DeepEqual(tc.p.options, []pluginapi.Options{tc.options}) || tc.p.schema == nil || tc.p.schema.Applet.Name != "Counter" {
			t.Fatalf("plugin %d: %+v", i, tc.p)
		}
		b, err := os.ReadFile(filepath.Join(out, tc.path))
		if err != nil || string(b) != tc.content {
			t.Fatalf("plugin %d output %q: %s, %v", i, tc.path, b, err)
		}
	}
	if j.schema != s.schema || s.schema != k.schema {
		t.Fatal("plugins did not share parsed model")
	}
}

// Real run rejects invalid options/IDL before it creates new outputs or alters
// an existing sentinel; nearby valid controls exercise each accepted path.
func TestRunPluginRejectionsPreserveOutput(t *testing.T) {
	for _, tc := range []struct {
		name, input, diagnostic string
		flags                   []string
		code                    int
	}{
		{"swift-stream", "stream.toml", "Swift stream generation is not implemented", []string{"--java", "probe", "--swift", "Probe"}, 2},
		{"simulator", "counter.toml", "invalid --simulator-dependency", []string{"--all", "--simulator-dependency", "bad:coordinate"}, 2},
		{"memory", "stream.toml", "unknown stream memory", []string{"--java", "probe", "--stream-memory", "invalid"}, 2},
		{"valid-java-stream", "stream.toml", "", []string{"--java", "probe", "--stream-memory", "clear_on_reset"}, 0},
		{"valid-all", "counter.toml", "", []string{"--all", "--simulator-dependency", "works.relux:jcardsim:3.0.5.9-relux.1"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			sentinel := filepath.Join(out, "sentinel")
			if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			args := append(append([]string{}, tc.flags...), "--out-dir", out, filepath.Join("..", "..", "testdata", tc.input))
			var stderr bytes.Buffer
			if code := run(args, &stderr); code != tc.code {
				t.Fatalf("exit %d want %d: %s", code, tc.code, &stderr)
			}
			if !strings.Contains(stderr.String(), tc.diagnostic) {
				t.Fatalf("wrong rejection: %s", &stderr)
			}
			b, err := os.ReadFile(sentinel)
			if err != nil || string(b) != "keep" {
				t.Fatal("sentinel altered")
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if tc.code != 0 && len(entries) != 1 {
				t.Fatal("partial output on refusal")
			}
			if tc.code == 0 && len(entries) < 2 {
				t.Fatal("valid control produced no output")
			}
		})
	}
}
