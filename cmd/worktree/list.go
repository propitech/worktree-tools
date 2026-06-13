package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
	"github.com/propitech/worktree-tools/internal/ports"
	"github.com/propitech/worktree-tools/internal/services"
)

const rowFmt = "%-4s  %-22s  %-6s  %-14s  %s\n"

func cmdList(_ []string, stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "worktree list: %v\n", err)
		return 1
	}
	mainPath, err := git.Main(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree list: %v\n", err)
		return 1
	}
	worktrees, err := git.Worktrees(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "worktree list: %v\n", err)
		return 1
	}

	svcCfg := services.LoadConfig()
	runtimeDir := services.RegistryGet("SVC_RUNTIME_DIR")
	if runtimeDir == "" {
		xdg := os.Getenv("XDG_RUNTIME_DIR")
		if xdg == "" {
			xdg = "/tmp"
		}
		runtimeDir = filepath.Join(xdg, "propitech-dev")
	}

	sharedUp := services.SharedPgUp(runtimeDir, svcCfg.PGPort)
	portBases := ports.Load()

	fmt.Fprintf(stdout, rowFmt, "SLOT", "SLUG", "WEB", "STATUS", "PATH")

	for _, wt := range worktrees {
		slug := git.SlugOf(mainPath, wt)
		if slug == "" {
			slug = "-"
		}

		envPath := filepath.Join(wt, ".env")
		slot, hasSlot := env.Slot(envPath)

		if !hasSlot {
			if wt == mainPath {
				slot = 0
			} else {
				fmt.Fprintf(stdout, rowFmt, "-", slug, "-", "unmanaged", wt)
				continue
			}
		}

		p := portBases.ForSlot(slot)
		webPort := env.Value(envPath, "PORT")
		if webPort == "" {
			webPort = strconv.Itoa(p.Web)
		}

		contract := env.Value(envPath, "WORKTREE_SERVICES")
		var status string
		switch {
		case contract == "shared":
			status = sharedStatus(wt, runtimeDir, svcCfg.PGPort, sharedUp)
		case services.AppEnvVar(wt, "WORKTREE_SERVICES") == "shared":
			status = "stale-contract"
		default:
			status = "legacy"
		}

		fmt.Fprintf(stdout, rowFmt, strconv.Itoa(slot), slug, webPort, status, wt)
	}

	if !sharedUp && services.TCPPgUp("127.0.0.1", svcCfg.PGPort) {
		fmt.Fprintf(stdout, "! foreign Postgres holds :%d (not the shared cluster) — stop it or change PG_PORT in %s/config\n",
			svcCfg.PGPort, services.ConfigDir())
	}

	return 0
}

func sharedStatus(wtPath, runtimeDir string, pgPort int, sharedUp bool) string {
	if !sharedUp {
		return "services-down"
	}
	envPath := filepath.Join(wtPath, ".env")
	app := env.Value(envPath, "WORKTREE_APP")
	if app == "" {
		return "no-app"
	}
	suffix := env.Value(envPath, "WORKTREE_DB_SUFFIX")
	if services.DBExists(runtimeDir, pgPort, app+"_development"+suffix) {
		return "up"
	}
	return "db-missing"
}
