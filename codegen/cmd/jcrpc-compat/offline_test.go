package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The helper starts both the listener and curl after the guard has launched it.
// On Linux they therefore share the disposable namespace, like Gradle's IPC.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--offline-local-ipc-control" {
		os.Exit(offlineLocalIPCControl())
	}
	os.Exit(m.Run())
}

func offlineLocalIPCControl() int {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("local IPC accepted")) }))
	defer local.Close()
	c := exec.Command("curl", "--fail", "--silent", "--show-error", "--connect-timeout", "5", "--max-time", "10", "--noproxy", "*", local.URL)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if e := c.Run(); e != nil {
		return fail(e)
	}
	return 0
}

func buildOfflineGuard(t *testing.T) string {
	t.Helper()
	guard := filepath.Join(t.TempDir(), "jcrpc-compat")
	build := exec.Command("go", "build", "-o", guard, ".")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build guard: %v %s", e, b)
	}
	return guard
}

func childLocalIPCCommand(t *testing.T, guard string) *exec.Cmd {
	t.Helper()
	helper, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	return exec.Command(guard, "--mode", "offline", "--", helper, "--offline-local-ipc-control")
}

// Both processes use real TCP loopback under the production CLI guard. This also
// exercises the Linux helper protocol on macOS; it cannot certify Linux isolation.
func TestOfflineGuardAllowsChildLocalIPC(t *testing.T) {
	c := childLocalIPCCommand(t, buildOfflineGuard(t))
	if b, e := c.CombinedOutput(); e != nil || string(b) != "local IPC accepted" {
		t.Fatalf("offline child local IPC control: %v %s", e, b)
	}
}

// The OS guard permits local build-tool IPC but refuses an actual remote HTTPS
// dependency connection. A preparation/setup failure cannot attest this control.
func TestOfflineGuardRejectsDependencyAccess(t *testing.T) {
	guard := buildOfflineGuard(t)
	var c *exec.Cmd
	if runtime.GOOS == "linux" {
		// A host listener is outside this namespace and is not supported IPC.
		c = childLocalIPCCommand(t, guard)
	} else {
		// Keep macOS's true host-loopback control for its sandbox contract.
		local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("local IPC accepted")) }))
		defer local.Close()
		c = exec.Command(guard, "--mode", "offline", "--", "curl", "--fail", "--silent", "--show-error", local.URL)
	}
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
