package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/ports"
	"github.com/propitech/worktree-tools/internal/services"
	"github.com/propitech/worktree-tools/internal/slot"
)

// cmdReprovision re-runs provision() against an existing worktree, rewriting
// its .env and re-copying gitignored secrets at its recorded slot. Useful after
// a contract change or a clobbered .env. The target defaults to the cwd.
func cmdReprovision(args []string, stdout, stderr io.Writer) int {
	target := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			fmt.Fprintf(stderr, "worktree reprovision: unknown flag %s\n", arg)
			return 2
		}
		target = arg
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: %v\n", err)
		return 1
	}
	if target == "" {
		target = cwd
	}

	mainPath, err := git.Main(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: %v\n", err)
		return 1
	}
	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: %v\n", err)
		return 1
	}

	dest, ok := resolveWorktree(target, mainPath, worktrees, stderr)
	if !ok {
		return 1
	}

	sl := 0
	if dest != mainPath {
		s, ok := env.Slot(filepath.Join(dest, ".env"))
		if !ok {
			fmt.Fprintf(stderr, "worktree reprovision: %s has no recorded slot — adopt it first\n", dest)
			return 1
		}
		sl = s
	}

	fmt.Fprintf(stdout, "==> Reprovisioning %s (slot %d)\n", dest, sl)

	commonDir, err := git.CommonDir(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: %v\n", err)
		return 1
	}
	var lk slot.Lock
	if err := lk.Acquire(filepath.Join(commonDir, "worktree-slot.lock")); err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: slot lock: %v\n", err)
		return 1
	}
	defer lk.Release()

	portBases := ports.Load()
	svcCfg := services.LoadConfig()
	_, runtimeDir := services.EnsureRegistry()

	if err := provision(dest, mainPath, sl, portBases, svcCfg, runtimeDir, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: %v\n", err)
		return 1
	}
	if err := ensurePrimaryShared(mainPath, portBases, svcCfg, runtimeDir, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree reprovision: %v\n", err)
		return 1
	}

	return 0
}
