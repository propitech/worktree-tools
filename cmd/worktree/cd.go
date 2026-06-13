package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
)

// cmdCd opens an interactive subshell whose working directory is the target
// worktree. A child process cannot change its parent shell's cwd, so a subshell
// is the portable way to "cd" into a worktree from the binary: the user lands in
// a fresh $SHELL there and returns to the original directory by exiting it.
//
// The target is resolved the same way `rm` resolves it — slug | name | path |
// slot (0 == primary checkout). Unlike adopt/rm, the primary checkout is a valid
// target.
func cmdCd(args []string, stdout, stderr io.Writer) int {
	target := ""
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--"):
			fmt.Fprintf(stderr, "worktree cd: unknown flag %s\n", arg)
			return 2
		default:
			target = arg
		}
	}
	if target == "" {
		fmt.Fprint(stderr, "usage: worktree cd <slug|name|path|slot>\n")
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree cd: %v\n", err)
		return 1
	}
	dest, ok := resolveTarget("cd", target, cwd, stderr)
	if !ok {
		return 1
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	label := filepath.Base(dest)
	if s, ok := env.Slot(filepath.Join(dest, ".env")); ok {
		fmt.Fprintf(stdout, "==> worktree %s (slot %d) — exit to return\n", label, s)
	} else {
		fmt.Fprintf(stdout, "==> worktree %s (unmanaged) — exit to return\n", label)
	}

	cmd := exec.Command(shell)
	cmd.Dir = dest
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// WORKTREE_CD lets a user's prompt show which worktree the subshell is in.
	cmd.Env = append(os.Environ(), "WORKTREE_CD="+label)
	return runExit(cmd)
}
