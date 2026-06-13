// Command worktree manages git worktrees backed by shared dev services.
//
// This is the Go rewrite (v2) of the POSIX-sh `worktree` script. Every
// subcommand (add, adopt, autoadopt, list, rm, reprovision, services) is now
// ported (PRO-135), so this binary is the source of truth; the `mise exec --
// worktree` binstub dispatches to it.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

const usage = `usage: worktree {add|adopt|autoadopt|list|rm|reprovision|services} ...

  add <slug> [<type>] [--no-start] [--prefix <ns>]
  adopt [<path>] [--start]
  autoadopt
  list
  rm <slug|name|path|slot> [--delete-branch] [--force]
  reprovision [<target>]
  services <start|stop|status>
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches a subcommand and returns the process exit code. I/O is
// injected so it stays unit-testable.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "list":
		return cmdList(args[1:], stdout, stderr)
	case "add":
		return cmdAdd(args[1:], stdout, stderr)
	case "adopt":
		return cmdAdopt(args[1:], stdout, stderr)
	case "autoadopt":
		return cmdAutoadopt(args[1:], stdout, stderr)
	case "rm":
		return cmdRm(args[1:], stdout, stderr)
	case "reprovision":
		return cmdReprovision(args[1:], stdout, stderr)
	case "services":
		return cmdServices(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "worktree: unknown subcommand %q\n", args[0])
		return 2
	}
}
