package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// cmdRun executes a command inside the target worktree via `mise exec --`, so
// the worktree's slot .env (PORT, DB suffix, Redis index, …) is loaded just as
// it is for `mise run start`. The target is resolved like rm: slug | name | path
// | slot (0 == primary checkout).
//
//	worktree run <target> <cmd> [args...]
//
// The command's exit code is propagated as worktree's own.
func cmdRun(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, "usage: worktree run <slug|name|path|slot> <cmd> [args...]\n")
		return 2
	}
	target := args[0]
	if strings.HasPrefix(target, "--") {
		fmt.Fprintf(stderr, "worktree run: unknown flag %s\n", target)
		return 2
	}
	rest := args[1:]
	if len(rest) == 0 {
		fmt.Fprint(stderr, "usage: worktree run <slug|name|path|slot> <cmd> [args...]\n")
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree run: %v\n", err)
		return 1
	}
	dest, ok := resolveTarget("run", target, cwd, stderr)
	if !ok {
		return 1
	}

	if _, err := exec.LookPath("mise"); err != nil {
		fmt.Fprint(stderr, "worktree run: mise not found on PATH\n")
		return 1
	}

	cmd := exec.Command("mise", append([]string{"exec", "--"}, rest...)...)
	cmd.Dir = dest
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return runExit(cmd)
}

// runExit runs cmd and returns the child's exit code as worktree's own, so a
// failing command propagates its status. A non-exit failure (e.g. binary not
// found) maps to 1.
func runExit(cmd *exec.Cmd) int {
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}
