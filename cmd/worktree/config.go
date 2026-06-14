package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/services"
)

// portKeys are the machine config-file ports `config set` accepts; dirKeys are
// the registry-backed state locations. Both lists double as the validation
// allowlist and the help text printed on an unknown key.
var (
	portKeys = []string{"PG_PORT", "REDIS_PORT", "MAIL_SMTP_PORT", "MAIL_UI_PORT"}
	dirKeys  = []string{"SVC_DATA_DIR", "SVC_RUNTIME_DIR"}
	repoKeys = []string{"WORKTREE_ROOT"}
)

// cmdConfig dispatches the `config` subcommand: `show` (the default) prints the
// effective configuration; `set <key> <value>` writes a machine-global port or
// state-location setting.
func cmdConfig(args []string, stdout, stderr io.Writer) int {
	action := "show"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "show":
		return configShow(stdout, stderr)
	case "set":
		return configSet(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "worktree config: unknown action %q (try: show, set)\n", action)
		return 2
	}
}

// configSet writes one machine-global setting. Port keys go to the config file,
// directory keys to the registry. It captures whether the shared daemons are up
// before applying the change so the post-write restart warning is accurate.
func configSet(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprint(stderr, "usage: worktree config set <key> <value>\n")
		printSetKeys(stderr)
		return 2
	}
	key, val := args[0], args[1]

	// WORKTREE_ROOT is per-repo (registry, keyed by clone identity), not a
	// machine-global port/dir, so it has its own path and returns early — no
	// services-restart warning, since placement never touches the daemons.
	if contains(repoKeys, key) {
		return configSetRepo(key, val, stdout, stderr)
	}

	// Snapshot running state against the *current* (pre-change) port/runtime dir
	// so a port/dir change doesn't make us probe the new, not-yet-live endpoint.
	wasUp := services.SharedPgUp(services.RuntimeDir(), services.LoadConfig().PGPort)

	switch {
	case contains(portKeys, key):
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 65535 {
			fmt.Fprintf(stderr, "worktree config set: %s must be a port number in 1..65535\n", key)
			return 2
		}
		if err := services.SetConfigPort(key, n); err != nil {
			fmt.Fprintf(stderr, "worktree config set: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "set %s=%d in %s\n", key, n, filepath.Join(services.ConfigDir(), "config"))
	case contains(dirKeys, key):
		if !filepath.IsAbs(val) {
			fmt.Fprintf(stderr, "worktree config set: %s must be an absolute path\n", key)
			return 2
		}
		clean := filepath.Clean(val)
		if err := services.RegistrySet(key, clean); err != nil {
			fmt.Fprintf(stderr, "worktree config set: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "set %s=%s in the registry\n", key, clean)
		fmt.Fprintf(stdout, "note: existing data is not moved — relocate the old %s contents manually if needed\n", key)
	default:
		fmt.Fprintf(stderr, "worktree config set: unknown key %q\n", key)
		printSetKeys(stderr)
		return 2
	}

	if wasUp {
		fmt.Fprintln(stdout, "warning: shared services are running — restart for the change to take effect:")
		fmt.Fprintln(stdout, "  worktree services stop && worktree services start")
	}
	return 0
}

// configSetRepo stores a per-repo setting (currently only WORKTREE_ROOT) in the
// registry, namespaced by the current clone's git common dir.
func configSetRepo(key, val string, stdout, stderr io.Writer) int {
	if !filepath.IsAbs(val) {
		fmt.Fprintf(stderr, "worktree config set: %s must be an absolute path\n", key)
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree config set: %v\n", err)
		return 1
	}
	commonDir, err := git.CommonDir(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree config set: %s is per-repo — run inside a git repository\n", key)
		return 1
	}
	clean := filepath.Clean(val)
	if err := services.SetRepoValue(commonDir, key, clean); err != nil {
		fmt.Fprintf(stderr, "worktree config set: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "set %s=%s for this repo (registry)\n", key, clean)
	return 0
}

func printSetKeys(w io.Writer) {
	fmt.Fprintf(w, "  ports: %s\n", strings.Join(portKeys, ", "))
	fmt.Fprintf(w, "  dirs:  %s\n", strings.Join(dirKeys, ", "))
	fmt.Fprintf(w, "  repo:  %s\n", strings.Join(repoKeys, ", "))
}

// resolveWorktreeRoot returns the directory new worktrees are created under and a
// label for where the value came from. Precedence excludes the `add --root`
// flag (applied by the caller): the per-repo WORKTREE_ROOT in the registry, else
// the parent of the primary checkout (the default sibling layout).
func resolveWorktreeRoot(cwd, mainPath string) (root, source string) {
	if commonDir, err := git.CommonDir(cwd); err == nil {
		if v := services.RepoValue(commonDir, "WORKTREE_ROOT"); v != "" {
			return v, "repo config"
		}
	}
	return filepath.Dir(mainPath), "default (sibling of primary)"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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
	dataDir := services.DataDir()
	runtimeDir := services.RuntimeDir()

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
		root, source := resolveWorktreeRoot(cwd, mainPath)
		row(stdout, "primary checkout", mainPath)
		row(stdout, "worktrees root", fmt.Sprintf("%s  (%s)", root, source))
		row(stdout, "new path", filepath.Join(root, repo+"-<slug>"))
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

func row(w io.Writer, label, value string) {
	fmt.Fprintf(w, cfgRowFmt, label, value)
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
