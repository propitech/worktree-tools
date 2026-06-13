package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/services"
)

// artifacts provision() plants; rm drops them so an adopted worktree (Claude
// ones included) carries no untracked files that would force --force removal.
// Real uncommitted work still blocks removal without --force.
var provisionedArtifacts = []string{".env", "mise.local.toml", "config/master.key", "Procfile.dev"}

func cmdRm(args []string, stdout, stderr io.Writer) int {
	target := ""
	deleteBranch := false
	force := false

	for _, arg := range args {
		switch {
		case arg == "--delete-branch":
			deleteBranch = true
		case arg == "--force":
			force = true
		case strings.HasPrefix(arg, "--"):
			fmt.Fprintf(stderr, "worktree rm: unknown flag %s\n", arg)
			return 2
		default:
			target = arg
		}
	}
	if target == "" {
		fmt.Fprint(stderr, "usage: worktree rm <slug|name|path|slot> [--delete-branch] [--force]\n")
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree rm: %v\n", err)
		return 1
	}

	mainPath, err := git.Main(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree rm: %v\n", err)
		return 1
	}
	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree rm: %v\n", err)
		return 1
	}

	dest, ok := resolveWorktree("rm", target, mainPath, worktrees, stderr)
	if !ok {
		return 1
	}
	if dest == mainPath {
		fmt.Fprint(stderr, "worktree rm: refusing to remove the primary checkout\n")
		return 1
	}

	branch := git.CurrentBranch(dest)

	// Shared-services worktrees: drop just this worktree's namespaces; the
	// shared daemons stay up for every other worktree. Legacy worktrees stop
	// their own per-worktree daemons as before.
	if env.Value(filepath.Join(dest, ".env"), "WORKTREE_SERVICES") == "shared" {
		cleanupShared(dest, stdout, stderr)
	} else {
		fmt.Fprintf(stdout, "==> Stopping services in %s\n", dest)
		stop := exec.Command("mise", "run", "stop")
		stop.Dir = dest
		stop.Stdout = stdout
		stop.Stderr = stderr
		_ = stop.Run()
	}

	for _, f := range provisionedArtifacts {
		_ = os.Remove(filepath.Join(dest, f))
	}

	fmt.Fprintf(stdout, "==> Removing worktree %s\n", dest)
	if err := git.Remove(cwd, dest, force, stdout); err != nil {
		fmt.Fprintf(stderr, "worktree rm: %v\n", err)
		return 1
	}

	// "HEAD" means the worktree was detached — no branch to report.
	switch {
	case branch == "" || branch == "HEAD":
		// nothing to report
	case deleteBranch:
		fmt.Fprintf(stdout, "==> Deleting branch %s\n", branch)
		if err := git.DeleteBranch(cwd, branch, stdout); err != nil {
			fmt.Fprintf(stderr, "worktree rm: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stdout, "    Branch '%s' kept. Delete with: git branch -D %s\n", branch, branch)
	}

	return 0
}

// resolveTarget is the cwd-aware wrapper around resolveWorktree: it reads the
// repo's worktree topology starting from cwd, then maps arg to a worktree path.
// It is the entry point for commands (cd, run) that only need the resolved path
// and not the surrounding topology. cmd names the caller for error messages.
func resolveTarget(cmd, arg, cwd string, stderr io.Writer) (string, bool) {
	mainPath, err := git.Main(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree %s: %v\n", cmd, err)
		return "", false
	}
	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree %s: %v\n", cmd, err)
		return "", false
	}
	return resolveWorktree(cmd, arg, mainPath, worktrees, stderr)
}

// resolveWorktree maps a slug | worktree dir name | path | slot number to a
// single registered worktree path. On miss or ambiguity it prints candidates
// to stderr and returns ok=false. This is what lets rm reach Claude isolation
// worktrees under .claude/worktrees/<name>, which don't follow the
// <repo>-<slug> naming.
func resolveWorktree(cmd, arg, mainPath string, worktrees []string, stderr io.Writer) (string, bool) {
	registered := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		registered[wt] = true
	}

	// 1. A path (absolute or relative) that is itself a registered worktree.
	if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
		if abs, err := filepath.Abs(arg); err == nil && registered[abs] {
			return abs, true
		}
	}

	// 2. Legacy slug -> <parent>/<repo>-<slug> sibling.
	legacy := filepath.Join(filepath.Dir(mainPath), filepath.Base(mainPath)+"-"+arg)
	if registered[legacy] {
		return legacy, true
	}

	// 3. Slot number -> the worktree whose .env records it (0 == primary).
	if n, err := strconv.Atoi(arg); err == nil {
		if n == 0 {
			return mainPath, true
		}
		for _, wt := range worktrees {
			if s, ok := env.Slot(filepath.Join(wt, ".env")); ok && s == n {
				return wt, true
			}
		}
	}

	// 4. Worktree directory basename (e.g. a Claude worktree's name).
	var matches []string
	for _, wt := range worktrees {
		if filepath.Base(wt) == arg {
			matches = append(matches, wt)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], true
	case 0:
		fmt.Fprintf(stderr, "worktree %s: no worktree matches '%s'. Known worktrees:\n", cmd, arg)
		for _, wt := range worktrees {
			fmt.Fprintf(stderr, "  %s\n", wt)
		}
	default:
		fmt.Fprintf(stderr, "worktree %s: '%s' matches multiple worktrees:\n", cmd, arg)
		for _, wt := range matches {
			fmt.Fprintf(stderr, "  %s\n", wt)
		}
	}
	return "", false
}

// cleanupShared drops this worktree's shared-services namespaces: its per-app
// Postgres databases and its Redis db index. The shared daemons keep running
// for every other worktree.
func cleanupShared(dest string, stdout, stderr io.Writer) {
	envFile := filepath.Join(dest, ".env")
	app := env.Value(envFile, "WORKTREE_APP")
	suffix := env.Value(envFile, "WORKTREE_DB_SUFFIX")
	redisDB := env.Value(envFile, "REDIS_DB")

	_, runtimeDir := services.EnsureRegistry()
	cfg := services.LoadConfig()

	slotLabel := "?"
	if s, ok := env.Slot(envFile); ok {
		slotLabel = strconv.Itoa(s)
	}
	appLabel := app
	if appLabel == "" {
		appLabel = "?"
	}
	fmt.Fprintf(stdout, "==> Dropping shared-services namespaces (app %s, slot %s)\n", appLabel, slotLabel)

	switch {
	case app == "":
		fmt.Fprint(stderr, "    (no WORKTREE_APP in .env — skipping database cleanup)\n")
	case services.SharedPgUp(runtimeDir, cfg.PGPort):
		for _, db := range []string{
			app + "_development" + suffix,
			app + "_development_cache" + suffix,
			app + "_development_queue" + suffix,
			app + "_development_cable" + suffix,
			app + "_test" + suffix,
		} {
			if services.DropDB(runtimeDir, cfg.PGPort, db) {
				fmt.Fprintf(stdout, "    Dropped %s\n", db)
			}
		}
	default:
		fmt.Fprint(stdout, "    (shared Postgres down — skipped database drop)\n")
	}

	// Redis: flush only this slot's DB index. Warn first if a client is still
	// on it — a zombie Sidekiq would otherwise contaminate a reused slot.
	sock := filepath.Join(runtimeDir, "redis.sock")
	if n, err := strconv.Atoi(redisDB); err == nil && services.RedisUp(sock) {
		if clients := services.RedisClientsOnDB(sock, n); clients > 0 {
			fmt.Fprintf(stdout, "    Warning: %d live Redis client(s) on db %d (zombie process?)\n", clients, n)
		}
		if services.RedisFlushDB(sock, n) {
			fmt.Fprintf(stdout, "    Flushed Redis db %d\n", n)
		}
	}
}
