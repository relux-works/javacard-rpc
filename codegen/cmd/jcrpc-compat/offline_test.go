package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The OS guard permits local build-tool IPC but refuses an actual remote HTTPS
// dependency connection. A preparation/setup failure cannot attest this control.
func TestOfflineGuardRejectsDependencyAccess(t *testing.T) {
	guard := filepath.Join(t.TempDir(), "jcrpc-compat")
	build := exec.Command("go", "build", "-o", guard, ".")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build guard: %v %s", e, b)
	}
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("local IPC accepted")) }))
	defer local.Close()
	c := exec.Command(guard, "--mode", "offline", "--", "curl", "--fail", "--silent", "--show-error", local.URL)
	b, e := c.CombinedOutput()
	if e != nil || string(b) != "local IPC accepted" {
		t.Fatalf("offline local IPC control: %v %s", e, b)
	}
	c = exec.Command(guard, "--mode", "offline", "--", "curl", "--fail", "--silent", "--show-error", "--connect-timeout", "5", "--max-time", "10", "--noproxy", "*", "--resolve", "repo.maven.apache.org:443:104.18.18.12", "https://repo.maven.apache.org/maven2/org/jetbrains/kotlin/kotlin-stdlib/2.1.10/kotlin-stdlib-2.1.10.pom")
	b, e = c.CombinedOutput()
	exit, ok := e.(*exec.ExitError)
	if !ok || exit.ExitCode() != 7 || !(strings.Contains(string(b), "Failed to connect") || strings.Contains(string(b), "Couldn't connect")) {
		t.Fatalf("network dependency refusal missing: %v %s", e, b)
	}
	t.Logf("actual dependency access refused (curl exit 7): %s", b)
}
