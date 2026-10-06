package codegen

import (
	"github.com/relux-works/javacard-rpc/codegen/plugins/javacard"
	"github.com/relux-works/javacard-rpc/codegen/plugins/kotlin"
	"github.com/relux-works/javacard-rpc/codegen/plugins/swift"
	"github.com/relux-works/javacard-rpc/pluginapi"
	"reflect"
	"testing"
)

// Actual adapters return each whole target package in released write order,
// without any facade packaging call. Manifest/source bytes are separately
// compared to v0.4.5 by the production CLI parity matrix.
func TestTargetAdaptersReturnWholePackages(t *testing.T) {
	s, e := ParseFile("testdata/counter.toml")
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name      string
		p         pluginapi.Plugin
		namespace string
		want      []string
	}{
		{"java", javacard.Plugin{}, "probe.server", []string{"settings.gradle", "build.gradle", "src/main/java/probe/server/CounterTransport.java", "src/main/java/probe/server/CounterSkeleton.java"}},
		{"swift", swift.Plugin{}, "ProbeClient", []string{"Package.swift", "Sources/CounterClient/CounterClient.swift"}},
		{"kotlin", kotlin.Plugin{}, "probe.client", []string{"settings.gradle.kts", "build.gradle.kts", "src/main/kotlin/probe/client/CounterClient.kt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, e := tc.p.Generate(s, pluginapi.Options{Namespace: tc.namespace, StreamMemory: "clear_on_reset", SimulatorDependency: "com.klinec:jcardsim:3.0.5.9"})
			if e != nil {
				t.Fatal(e)
			}
			var names []string
			for _, f := range files {
				names = append(names, f.Name)
				if len(f.Data) == 0 {
					t.Fatalf("empty package member %s", f.Name)
				}
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("package files %v want %v", names, tc.want)
			}
		})
	}
}
