package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
)

// cmdAutoadopt is the SessionStart-hook entry point. When a shell opens inside a
// Claude Code isolation worktree (.claude/worktrees/<name>), it adopts the
// worktree into a slot so its dev services don't collide with the primary
// checkout. Adopt-only: services stay DOWN.
//
// It is a silent no-op everywhere else, and on every run after the first. A
// failure must never abort the session, so it always returns 0. Wire it as
// `command: worktree autoadopt` in .claude/settings.json.
//
// There is deliberately no auto-remove counterpart: tearing a worktree down on
// session end would risk unfinished/uncommitted work. Remove explicitly with
// `worktree rm <name>`.
func cmdAutoadopt(_ []string, stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	if !strings.Contains(filepath.ToSlash(cwd), "/.claude/worktrees/") {
		return 0
	}
	if _, err := exec.LookPath("git"); err != nil {
		return 0
	}
	// Already adopted — .env records a slot. Nothing to do.
	if _, ok := env.Slot(filepath.Join(cwd, ".env")); ok {
		return 0
	}
	// adopt guards against the primary checkout / an already-managed worktree;
	// swallow its exit code so a failure never aborts the session.
	cmdAdopt(nil, stdout, stderr)
	return 0
}
