package main

import (
	"fmt"
	"os"
	"os/exec"
)

// cmdService wraps a handful of systemctl / journalctl invocations
// against the shellboto unit. The wrappers are convenience sugar for
// operators who keep forgetting the exact `systemctl status shellboto`
// / `journalctl -u shellboto -f` invocation — no business logic, just
// passthroughs with proper stdio and exit-code propagation.
//
// Every sub-verb requires root for its systemctl/journalctl to succeed
// on most systems; we don't check for root here (we propagate whatever
// exit the underlying tool returns).
func cmdService(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, serviceUsage)
		return exitUsage
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(serviceUsage)
		return exitOK
	}
	verb := args[0]
	switch verb {
	case "status":
		return runExternal("systemctl", "status", "shellboto")
	case "start":
		return runExternal("systemctl", "start", "shellboto")
	case "stop":
		return runExternal("systemctl", "stop", "shellboto")
	case "restart":
		return runExternal("systemctl", "restart", "shellboto")
	case "enable":
		return runExternal("systemctl", "enable", "shellboto")
	case "disable":
		return runExternal("systemctl", "disable", "shellboto")
	case "logs":
		// Default to "follow + last 200 lines" — the most common operator
		// invocation. Extra args pass through, so e.g.
		// `shellboto service logs --since=-1h` works.
		rest := append([]string{"-u", "shellboto", "-n", "200", "-f"}, args[1:]...)
		return runExternal("journalctl", rest...)
	default:
		fmt.Fprintf(os.Stderr, "unknown service verb %q\n\n%s", verb, serviceUsage)
		return exitUsage
	}
}

// runExternal runs cmd with args, wiring stdin/stdout/stderr through so
// interactive tools (journalctl -f, systemctl status pager) behave
// naturally. Returns the child's exit code.
func runExternal(name string, args ...string) int {
	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return exitErr
	}
	return exitOK
}

const serviceUsage = `shellboto service — convenience wrappers for the shellboto systemd unit.

Usage: shellboto service <verb> [args]

Verbs:
  status      systemctl status shellboto
  start       systemctl start shellboto
  stop        systemctl stop shellboto
  restart     systemctl restart shellboto
  enable      systemctl enable shellboto
  disable     systemctl disable shellboto
  logs [...]  journalctl -u shellboto -n 200 -f [pass-through args]

Most of these need root. The exit code is whatever systemctl/journalctl
returns.
`
