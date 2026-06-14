package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/services"
)

// cmdConfig dispatches the `config` subcommand. Only `show` (the default) is
// implemented; it prints the effective configuration the tool resolves,
// grouped into Global / Worktree creation / Service endpoints / This worktree.
func cmdConfig(args []string, stdout, stderr io.Writer) int {
	action := "show"
	if len(args) > 0 {
		action = args[0]
	}
	if action != "show" {
		fmt.Fprintf(stderr, "worktree config: unknown action %q (try: show)\n", action)
		return 2
	}
	return configShow(stdout, stderr)
}

const cfgRowFmt = "  %-16s %s\n"

func configShow(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree config: %v\n", err)
		return 1
	}

	svcCfg := services.LoadConfig()
	configDir := services.ConfigDir()
	dataDir := resolveDataDir()
	runtimeDir := resolveRuntimeDir()

	fmt.Fprintln(stdout, "worktree config")

	// Global ----------------------------------------------------------------
	fmt.Fprintln(stdout, "\nGlobal")
	row(stdout, "config dir", configDir)
	row(stdout, "data dir", dataDir)
	row(stdout, "runtime dir", runtimeDir)
	prefix := os.Getenv("WORKTREE_BRANCH_PREFIX")
	if prefix == "" {
		prefix = "(none)"
	}
	row(stdout, "branch prefix", prefix+"  [WORKTREE_BRANCH_PREFIX]")
	row(stdout, "default type", "feat")

	// Worktree creation -----------------------------------------------------
	fmt.Fprintln(stdout, "\nWorktree creation")
	mainPath, err := git.Main(cwd)
	if err != nil {
		row(stdout, "primary checkout", "(not in a git repo)")
	} else {
		repo := filepath.Base(mainPath)
		parent := filepath.Dir(mainPath)
		row(stdout, "primary checkout", mainPath)
		row(stdout, "parent dir", parent)
		row(stdout, "new path", filepath.Join(parent, repo+"-<slug>"))
	}

	// Service endpoints -----------------------------------------------------
	fmt.Fprintln(stdout, "\nService endpoints")
	pgState := "down"
	if services.SharedPgUp(runtimeDir, svcCfg.PGPort) {
		pgState = "up"
	}
	row(stdout, "postgres", fmt.Sprintf("127.0.0.1:%d  (%s)", svcCfg.PGPort, pgState))
	row(stdout, "redis", fmt.Sprintf("127.0.0.1:%d", svcCfg.RedisPort))
	row(stdout, "mailpit smtp", fmt.Sprintf("127.0.0.1:%d", svcCfg.MailSMTPPort))
	row(stdout, "mailpit ui", fmt.Sprintf("http://127.0.0.1:%d", svcCfg.MailUIPort))

	// This worktree ---------------------------------------------------------
	fmt.Fprintln(stdout, "\nThis worktree")
	if mainPath == "" {
		row(stdout, "", "(not in a git repo)")
		return 0
	}
	wt := containingWorktree(cwd, mainPath)
	envPath := filepath.Join(wt, ".env")
	slot, hasSlot := env.Slot(envPath)
	if !hasSlot {
		row(stdout, "path", wt)
		row(stdout, "", "(not a managed worktree — no .env slot)")
		return 0
	}
	app := env.Value(envPath, "WORKTREE_APP")
	suffix := env.Value(envPath, "WORKTREE_DB_SUFFIX")
	row(stdout, "path", wt)
	row(stdout, "slot", fmt.Sprintf("%d", slot))
	row(stdout, "app", valueOr(app, "(unset)"))
	row(stdout, "services", valueOr(env.Value(envPath, "WORKTREE_SERVICES"), "(unset)"))
	row(stdout, "db suffix", valueOr(suffix, "(none)"))
	row(stdout, "web port", valueOr(env.Value(envPath, "PORT"), "(unset)"))
	row(stdout, "redis db", valueOr(env.Value(envPath, "REDIS_DB"), "(unset)"))
	if app != "" {
		row(stdout, "databases", strings.Join(databaseNames(app, suffix), ", "))
	}
	return 0
}

// databaseNames lists the Postgres databases a worktree owns, matching the set
// `worktree rm` drops: development + its cache/queue/cable companions, and test.
func databaseNames(app, suffix string) []string {
	return []string{
		app + "_development" + suffix,
		app + "_development_cache" + suffix,
		app + "_development_queue" + suffix,
		app + "_development_cable" + suffix,
		app + "_test" + suffix,
	}
}

// containingWorktree returns the registered worktree that holds cwd (cwd may be
// a subdirectory of it), falling back to the primary checkout.
func containingWorktree(cwd, mainPath string) string {
	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		return mainPath
	}
	best := ""
	for _, wt := range worktrees {
		if cwd == wt || strings.HasPrefix(cwd, wt+string(os.PathSeparator)) {
			if len(wt) > len(best) {
				best = wt
			}
		}
	}
	if best == "" {
		return mainPath
	}
	return best
}

// resolveDataDir mirrors EnsureRegistry's data-dir resolution read-only: the
// registry value if set, else the XDG default. Unlike EnsureRegistry it never
// writes, so `config show` stays side-effect free.
func resolveDataDir() string {
	if d := services.RegistryGet("SVC_DATA_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(base, "propitech-dev")
}

// resolveRuntimeDir mirrors EnsureRegistry's runtime-dir resolution read-only.
func resolveRuntimeDir() string {
	if d := services.RegistryGet("SVC_RUNTIME_DIR"); d != "" {
		return d
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, "propitech-dev")
}

func row(w io.Writer, label, value string) {
	fmt.Fprintf(w, cfgRowFmt, label, value)
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
