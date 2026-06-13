package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/ports"
	"github.com/propitech/worktree-tools/internal/services"
	"github.com/propitech/worktree-tools/internal/slot"
)

func cmdAdopt(args []string, stdout, stderr io.Writer) int {
	start := false
	target := ""

	for _, arg := range args {
		switch {
		case arg == "--start":
			start = true
		case arg == "--no-start":
			// default; accepted silently for symmetry with add
		case len(arg) > 2 && arg[:2] == "--":
			fmt.Fprintf(stderr, "worktree adopt: unknown flag %s\n", arg)
			return 2
		default:
			target = arg
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}

	if target == "" {
		target = cwd
	}
	if _, err := os.Stat(target); err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %s not found\n", target)
		return 1
	}
	target, err = filepath.Abs(target)
	if err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}

	mainPath, err := git.Main(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}

	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}

	registered := false
	for _, wt := range worktrees {
		if wt == target {
			registered = true
			break
		}
	}
	if !registered {
		fmt.Fprintf(stderr, "worktree adopt: %s is not a git worktree\n", target)
		return 1
	}
	if target == mainPath {
		fmt.Fprintf(stderr, "worktree adopt: refusing to adopt the primary checkout\n")
		return 1
	}

	commonDir, err := git.CommonDir(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}
	var lk slot.Lock
	if err := lk.Acquire(filepath.Join(commonDir, "worktree-slot.lock")); err != nil {
		fmt.Fprintf(stderr, "worktree adopt: slot lock: %v\n", err)
		return 1
	}
	defer lk.Release()

	if s, ok := env.Slot(filepath.Join(target, ".env")); ok {
		fmt.Fprintf(stderr, "worktree adopt: already managed (slot %d)\n", s)
		return 1
	}

	var usedSlots []int
	for _, wt := range worktrees {
		if s, ok := env.Slot(filepath.Join(wt, ".env")); ok {
			usedSlots = append(usedSlots, s)
		}
	}
	sl := slot.NextFree(usedSlots)

	portBases := ports.Load()
	svcCfg := services.LoadConfig()
	_, runtimeDir := services.EnsureRegistry()

	fmt.Fprintf(stdout, "==> Adopting %s (slot %d)\n", target, sl)

	if err := provision(target, mainPath, sl, portBases, svcCfg, runtimeDir, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}

	if err := ensurePrimaryShared(mainPath, portBases, svcCfg, runtimeDir, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree adopt: %v\n", err)
		return 1
	}

	lk.Release()

	if start {
		fmt.Fprintf(stdout, "==> Booting services (mise run start)\n")
		cmd := exec.Command("mise", "run", "start")
		cmd.Dir = target
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "worktree adopt: mise run start: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "==> Adopted. Start with: cd %s && mise run start\n", target)
	}

	return 0
}
