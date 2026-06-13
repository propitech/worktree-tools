package main

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/propitech/worktree-tools/internal/services"
)

// cmdServices manages the machine-global shared dev services: one Postgres +
// Redis + Mailpit per machine, shared by every worktree of every app. Isolation
// is by namespace (db name / redis index), not port; this command owns only the
// daemons themselves.
func cmdServices(args []string, stdout, stderr io.Writer) int {
	action := ""
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "status":
		return servicesStatus(stdout)
	case "start":
		return servicesStart(stdout, stderr)
	case "stop":
		return servicesStop(stdout, stderr)
	case "":
		fmt.Fprint(stderr, "usage: worktree services {start|stop|status}\n")
		return 2
	default:
		fmt.Fprintf(stderr, "worktree services: unknown action '%s' (start|stop|status)\n", action)
		return 2
	}
}

// servicesStart brings up the shared cluster, initialising Postgres/Redis data
// dirs on first use. Each daemon is started then verified (with a brief retry,
// since daemons fork before they bind); any verification failure exits 1.
func servicesStart(stdout, stderr io.Writer) int {
	var lk services.MachineLock
	if err := lk.Acquire(); err != nil {
		fmt.Fprintf(stderr, "worktree services: lock: %v\n", err)
		return 1
	}
	defer lk.Release()

	cfg, dataDir, runtimeDir, err := services.Prepare()
	if err != nil {
		fmt.Fprintf(stderr, "worktree services: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "==> Shared dev services (data: %s, sockets: %s)\n", dataDir, runtimeDir)
	rc := 0

	fmt.Fprintf(stdout, "    postgres (:%d) ... ", cfg.PGPort)
	services.StartPostgres(dataDir, runtimeDir, cfg.PGPort)
	if services.Retry(func() bool { return services.VerifyPostgres(runtimeDir, cfg.PGPort) }) {
		fmt.Fprintln(stdout, "ok")
	} else {
		fmt.Fprintln(stdout, "FAILED")
		rc = 1
	}

	sock := filepath.Join(runtimeDir, "redis.sock")
	fmt.Fprintf(stdout, "    redis    (:%d) ... ", cfg.RedisPort)
	services.StartRedis(dataDir, runtimeDir, cfg.RedisPort)
	if services.Retry(func() bool { return services.VerifyRedis(sock, cfg.RedisPort) }) {
		fmt.Fprintln(stdout, "ok")
	} else {
		fmt.Fprintln(stdout, "FAILED (port may be held by another Redis)")
		rc = 1
	}

	fmt.Fprintf(stdout, "    mailpit  (:%d smtp, :%d ui) ... ", cfg.MailSMTPPort, cfg.MailUIPort)
	services.StartMailpit(dataDir, runtimeDir, cfg.MailSMTPPort, cfg.MailUIPort)
	if services.Retry(func() bool { return services.MailpitRunning(runtimeDir) }) {
		fmt.Fprintln(stdout, "ok")
	} else {
		fmt.Fprintln(stdout, "FAILED")
		rc = 1
	}

	if rc != 0 {
		fmt.Fprintf(stderr, "worktree services: one or more services failed verification (logs in %s)\n",
			filepath.Join(dataDir, "logs"))
		return 1
	}
	fmt.Fprintf(stdout, "==> Ready. Mailpit UI: http://localhost:%d\n", cfg.MailUIPort)
	return 0
}

// servicesStop stops the shared cluster. Each daemon reports stopped / not
// running independently; a failed stop is noted but never aborts the others.
func servicesStop(stdout, stderr io.Writer) int {
	var lk services.MachineLock
	if err := lk.Acquire(); err != nil {
		fmt.Fprintf(stderr, "worktree services: lock: %v\n", err)
		return 1
	}
	defer lk.Release()

	if _, err := exec.LookPath("mise"); err != nil {
		fmt.Fprint(stderr, "worktree services: mise not found on PATH\n")
		return 1
	}
	dataDir, runtimeDir := services.EnsureRegistry()

	fmt.Fprint(stdout, "==> Stopping shared dev services\n")

	if attempted, ok := services.StopPostgres(dataDir); !attempted {
		fmt.Fprint(stdout, "    postgres ... not running\n")
	} else if ok {
		fmt.Fprint(stdout, "    postgres ... stopped\n")
	} else {
		fmt.Fprint(stdout, "    postgres ... (stop failed)\n")
	}

	if services.StopRedis(runtimeDir) {
		fmt.Fprint(stdout, "    redis    ... stopped\n")
	} else {
		fmt.Fprint(stdout, "    redis    ... not running\n")
	}

	if attempted, ok := services.StopMailpit(runtimeDir); !attempted {
		fmt.Fprint(stdout, "    mailpit  ... not running\n")
	} else if ok {
		fmt.Fprint(stdout, "    mailpit  ... stopped\n")
	} else {
		fmt.Fprint(stdout, "    mailpit  ... (stop failed)\n")
	}

	return 0
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
