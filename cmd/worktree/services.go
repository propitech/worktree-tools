package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/propitech/worktree-tools/internal/services"
)

// cmdServices manages the machine-global shared dev services (one Postgres +
// Redis + Mailpit per machine, shared by every worktree of every app).
//
// Ported incrementally: `status` (read-only) lands first; `start`/`stop` (the
// daemon lifecycle) follow. Until they land they return an unimplemented status
// and the shell script remains the source of truth for those actions.
func cmdServices(args []string, stdout, stderr io.Writer) int {
	action := ""
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "status":
		return servicesStatus(stdout)
	case "start", "stop":
		fmt.Fprintf(stderr, "worktree: %q is not yet ported to the Go build; use the shell tool\n", "services "+action)
		return 70 // EX_SOFTWARE — recognised action, not implemented yet
	case "":
		fmt.Fprint(stderr, "usage: worktree services {start|stop|status}\n")
		return 2
	default:
		fmt.Fprintf(stderr, "worktree services: unknown action '%s' (start|stop|status)\n", action)
		return 2
	}
}

// servicesStatus prints the shared cluster's health. Read-only; it also names a
// foreign listener squatting a configured TCP port — an old per-app daemon can
// sit on the shared port and silently accept connections.
func servicesStatus(stdout io.Writer) int {
	dataDir, runtimeDir := services.EnsureRegistry()
	cfg := services.LoadConfig()

	fmt.Fprintf(stdout, "Shared dev services (data: %s, sockets: %s)\n", dataDir, runtimeDir)

	switch {
	case services.SharedPgUp(runtimeDir, cfg.PGPort):
		fmt.Fprintf(stdout, "  postgres  up       socket %s  tcp :%d\n", runtimeDir, cfg.PGPort)
	case services.TCPPgUp("127.0.0.1", cfg.PGPort):
		fmt.Fprintf(stdout, "  postgres  FOREIGN  another Postgres holds :%d — stop it or change PG_PORT in %s/config\n",
			cfg.PGPort, services.ConfigDir())
	default:
		fmt.Fprint(stdout, "  postgres  down\n")
	}

	sock := filepath.Join(runtimeDir, "redis.sock")
	switch {
	case services.VerifyRedis(sock, cfg.RedisPort):
		fmt.Fprintf(stdout, "  redis     up       socket %s  tcp :%d\n", sock, cfg.RedisPort)
	case services.RedisForeignUp(cfg.RedisPort):
		fmt.Fprintf(stdout, "  redis     FOREIGN  another Redis holds :%d — stop it or change REDIS_PORT in %s/config\n",
			cfg.RedisPort, services.ConfigDir())
	default:
		fmt.Fprint(stdout, "  redis     down\n")
	}

	if services.MailpitRunning(runtimeDir) {
		fmt.Fprintf(stdout, "  mailpit   up       smtp :%d  ui http://localhost:%d\n", cfg.MailSMTPPort, cfg.MailUIPort)
	} else {
		fmt.Fprint(stdout, "  mailpit   down\n")
	}

	return 0
}
