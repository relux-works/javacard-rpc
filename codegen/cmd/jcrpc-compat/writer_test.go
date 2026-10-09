package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/codegen/internal/compat"
)

// Production run -> compat.Check accepts the released writer tuple and rejects
// an authentic older signed tag object or version paired with the current
// receipt. This proves consistency; VerifyCheckout supplies signature trust.
func TestManifestReleasedWriterReceiptRefusals(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	if code := run([]string{"--repo", repo}); code != 0 {
		t.Fatalf("valid writer rejected: %d", code)
	}
	raw, e := os.ReadFile(filepath.Join(repo, "compatibility/runtime-manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"old-signed-tag", "old-version"} {
		t.Run(kind, func(t *testing.T) {
			var m compat.Manifest
			if e := json.Unmarshal(raw, &m); e != nil {
				t.Fatal(e)
			}
			for i := range m.Targets {
				if m.Targets[i].Target == "javacard" {
					if kind == "old-signed-tag" {
						m.Targets[i].TagObject = "ca371dc528c263aa7892cc42ffebe4192c081e9c"
					} else {
						m.Targets[i].Tag = "v0.3.1"
						m.Targets[i].BackendVersion = "v0.3.1"
						m.Targets[i].Runtime = "io.jcrpc:javacard-rpc-server-javacard:0.3.1"
					}
				}
			}
			b, e := json.Marshal(m)
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(t.TempDir(), "manifest.json")
			if e = os.WriteFile(path, b, 0644); e != nil {
				t.Fatal(e)
			}
			_, e = compat.Check(repo, path)
			if e == nil || e.Error() != "release receipt disagreement: javacard" {
				t.Fatalf("mismatched signed writer receipt admitted: %v", e)
			}
			if code := run([]string{"--repo", repo, "--manifest", path}); code != 1 {
				t.Fatalf("production mismatched writer receipt admitted: %d", code)
			}
		})
	}
}

// Production prepare emits the actual includeBuild consumer roots and pinned
// coordinates without launching a JVM. Gradle execution is a separate lane;
// this direct preparation fixture does not attest checkout verification.
func TestPreparedWriterConsumerCoordinates(t *testing.T) {
	repo, e := filepath.Abs("../../..")
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	if e = prepare(repo, root); e != nil {
		t.Fatal(e)
	}
	build, e := os.ReadFile(filepath.Join(root, "jvm/build.gradle"))
	if e != nil {
		t.Fatal(e)
	}
	for _, coordinate := range []string{"io.jcrpc:javacard-rpc-server-javacard:0.4.0", "io.jcrpc:javacard-rpc-client-kotlin:0.3.0"} {
		if !strings.Contains(string(build), coordinate) {
			t.Fatalf("missing pin %s", coordinate)
		}
	}
	if strings.Contains(string(build), "javacard:0.3.1") {
		t.Fatal("obsolete writer runtime coordinate")
	}
	settings, e := os.ReadFile(filepath.Join(root, "jvm/settings.gradle"))
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"javacard-rpc-server-javacard", "javacard-rpc-client-kotlin"} {
		if !strings.Contains(string(settings), "substitute(module('io.jcrpc:"+name+"')).using(project(':'))") {
			t.Fatalf("runtime substitution missing %s", name)
		}
	}
	fixture, e := os.ReadFile(filepath.Join(root, "jvm/src/test/kotlin/PinnedConsumerTest.kt"))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(fixture), "fixedWriterRefusesBeforeCallbackAndPreservesWire") {
		t.Fatal("writer callback consumer missing")
	}
}
