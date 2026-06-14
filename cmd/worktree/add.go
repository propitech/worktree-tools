package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/ports"
	"github.com/propitech/worktree-tools/internal/services"
	"github.com/propitech/worktree-tools/internal/slot"
)

func cmdAdd(args []string, stdout, stderr io.Writer) int {
	slug := ""
	wtype := "feat"
	start := true
	prefix := ""
	prefixFromCLI := false
	rootFlag := ""

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--no-start":
			start = false
		case args[i] == "--prefix" && i+1 < len(args):
			i++
			prefix = args[i]
			prefixFromCLI = true
		case strings.HasPrefix(args[i], "--prefix="):
			prefix = strings.TrimPrefix(args[i], "--prefix=")
			prefixFromCLI = true
		case args[i] == "--root" && i+1 < len(args):
			i++
			rootFlag = args[i]
		case strings.HasPrefix(args[i], "--root="):
			rootFlag = strings.TrimPrefix(args[i], "--root=")
		case strings.HasPrefix(args[i], "--"):
			fmt.Fprintf(stderr, "worktree add: unknown flag %s\n", args[i])
			return 2
		case slug == "":
			slug = args[i]
		default:
			wtype = args[i]
		}
	}

	if slug == "" {
		fmt.Fprint(stderr, "usage: worktree add <slug> [<type>] [--no-start] [--prefix <ns>] [--root <dir>]\n")
		return 2
	}
	if strings.Contains(slug, "/") {
		fmt.Fprintf(stderr, "worktree add: slug must not contain '/'\n")
		return 2
	}
	if rootFlag != "" && !filepath.IsAbs(rootFlag) {
		fmt.Fprintf(stderr, "worktree add: --root must be an absolute path\n")
		return 2
	}

	if !prefixFromCLI {
		prefix = os.Getenv("WORKTREE_BRANCH_PREFIX")
	}
	prefix = strings.Trim(prefix, "/")

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree add: %v\n", err)
		return 1
	}
	mainPath, err := git.Main(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree add: %v\n", err)
		return 1
	}

	repo := filepath.Base(mainPath)
	// Root precedence: --root override → per-repo WORKTREE_ROOT → sibling default.
	root := rootFlag
	if root == "" {
		root, _ = resolveWorktreeRoot(cwd, mainPath)
	}
	dest := filepath.Join(root, repo+"-"+slug)

	var branch string
	if prefix != "" {
		branch = prefix + "/" + wtype + "/" + slug
	} else {
		branch = wtype + "/" + slug
	}

	if _, err := os.Stat(dest); err == nil {
		fmt.Fprintf(stderr, "worktree add: %s already exists\n", dest)
		return 1
	}

	commonDir, err := git.CommonDir(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree add: %v\n", err)
		return 1
	}
	var lk slot.Lock
	if err := lk.Acquire(filepath.Join(commonDir, "worktree-slot.lock")); err != nil {
		fmt.Fprintf(stderr, "worktree add: slot lock: %v\n", err)
		return 1
	}
	defer lk.Release()

	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree add: %v\n", err)
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
	p := portBases.ForSlot(sl)
	svcCfg := services.LoadConfig()
	_, runtimeDir := services.EnsureRegistry()

	fmt.Fprintf(stdout, "==> Creating worktree %q (slot %d)\n", slug, sl)
	fmt.Fprintf(stdout, "    path:   %s\n", dest)
	fmt.Fprintf(stdout, "    branch: %s\n", branch)

	// git worktree add creates only the leaf dir, so the chosen root must exist.
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Fprintf(stderr, "worktree add: create root %s: %v\n", root, err)
		return 1
	}

	if err := git.Add(cwd, dest, branch, stderr); err != nil {
		fmt.Fprintf(stderr, "worktree add: git worktree add: %v\n", err)
		return 1
	}

	if err := provision(dest, mainPath, sl, portBases, svcCfg, runtimeDir, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree add: %v\n", err)
		return 1
	}

	if err := ensurePrimaryShared(mainPath, portBases, svcCfg, runtimeDir, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree add: %v\n", err)
		return 1
	}

	lk.Release()

	if start {
		fmt.Fprintf(stdout, "==> Booting services (mise run start)\n")
		cmd := exec.Command("mise", "run", "start")
		cmd.Dir = dest
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "worktree add: mise run start: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "==> Skipped boot (--no-start). Start later with: cd %s && mise run start\n", dest)
	}

	fmt.Fprintf(stdout, "==> Done. Web server: http://localhost:%d\n", p.Web)
	return 0
}
