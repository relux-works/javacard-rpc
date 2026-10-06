package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

// Local listeners are allowed; remote outbound dependency traffic is refused.
// Java uses IPv4 loopback IPC because macOS's localhost sandbox rule does not
// admit the mapped IPv6 socket address used by Gradle on the measured host.
const offlineProfile = `(version 1)(allow default)(deny network*)(allow network-bind)(allow network-inbound)(allow network-outbound (remote ip "localhost:*"))`

func offlineRun(args []string) int {
	if len(args) == 0 {
		return fail(fmt.Errorf("offline mode requires a command after --"))
	}
	name := args[0]
	argv := args[1:]
	env := os.Environ()
	switch runtime.GOOS {
	case "darwin":
		name = "/usr/bin/sandbox-exec"
		argv = append([]string{"-p", offlineProfile}, args...)
		options := strings.TrimSpace(os.Getenv("JAVA_TOOL_OPTIONS") + " -Djava.net.preferIPv4Stack=true")
		env = append(env, "JAVA_TOOL_OPTIONS="+options)
	case "linux":
		// Privilege is used only to create a disposable network namespace and bring
		// up loopback. Drop back to the calling UID/GID before executing the build,
		// preserving its prepared cache/toolchain locations, not root's caches.
		name = "sudo"
		argv = []string{"-n", "--preserve-env=JAVA_HOME,GRADLE_USER_HOME,GOPATH,GOCACHE,GOMODCACHE", "unshare", "--net", "--", "sh", "-c", `ip link set lo up || exit; uid=$1; gid=$2; toolpath=$3; caller_home=$4; shift 4; exec setpriv --reuid "$uid" --regid "$gid" --init-groups env PATH="$toolpath" HOME="$caller_home" "$@"`, "offline", fmt.Sprint(os.Getuid()), fmt.Sprint(os.Getgid()), os.Getenv("PATH"), os.Getenv("HOME")}
		argv = append(argv, args...)
	default:
		return fail(fmt.Errorf("offline enforcement unsupported on %s", runtime.GOOS))
	}
	c := exec.Command(name, argv...)
	c.Env = env
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if e := c.Run(); e != nil {
		if exit, ok := e.(*exec.ExitError); ok {
			if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal())
			}
			return exit.ExitCode()
		}
		return fail(e)
	}
	return 0
}
