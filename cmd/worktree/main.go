// Command worktree manages git worktrees backed by shared dev services.
//
// This is the Go rewrite (v2) of the POSIX-sh `worktree` script. Subcommands are
// ported incrementally (PRO-135+); until one lands here it returns an
// unimplemented status and the shell script remains the source of truth, so the
// `mise exec -- worktree` binstub keeps working throughout the migration.
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
	case "autoadopt", "rm", "reprovision", "services":
		fmt.Fprintf(stderr, "worktree: %q is not yet ported to the Go build; use the shell tool\n", args[0])
		return 70 // EX_SOFTWARE — recognised subcommand, not implemented yet
	default:
		fmt.Fprintf(stderr, "worktree: unknown subcommand %q\n", args[0])
		return 2
	}
}
