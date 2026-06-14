package services

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// scrubbedEnvKeys are app-level env vars that must not leak into the shared
// daemons: the manager runs under an app's `mise exec`, so without scrubbing
// the app's DB_PORT / SOCK_DIR / REDIS_PORT… would point the shared daemons at
// the wrong place. Mirrors the shell svc_run -u list.
var scrubbedEnvKeys = []string{
	"DB_PORT", "DB_HOST", "DB_USER", "DB_PASS", "DB_NAME",
	"REDIS_PORT", "REDIS_URL", "REDIS_DB",
	"PORT", "MAIL_SMTP_PORT", "MAIL_UI_PORT",
	"PG_DATA_DIR", "SOCK_DIR", "SOCK_PREFIX", "WORKTREE_SLOT",
	"WORKTREE_DB_SUFFIX", "WORKTREE_APP", "WORKTREE_SERVICES",
}

// svcRun builds a command that runs name+args through the pinned-version mise
// config (`mise -C <cfg> x --`) with the app's env scrubbed, so the shared
// Postgres/Redis/Mailpit always run one canonical version set regardless of
// what any app pins.
func svcRun(name string, args ...string) *exec.Cmd {
	full := append([]string{"-C", ConfigDir(), "x", "--", name}, args...)
	cmd := exec.Command("mise", full...)
	cmd.Env = scrubbedEnv()
	return cmd
}

func scrubbedEnv() []string {
	drop := make(map[string]bool, len(scrubbedEnvKeys))
	for _, k := range scrubbedEnvKeys {
		drop[k] = true
	}
	var out []string
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && drop[k] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func logDir(dataDir string) string    { return filepath.Join(dataDir, "logs") }
func pgData(dataDir string) string    { return filepath.Join(dataDir, "postgres") }
func redisSock(rt string) string      { return filepath.Join(rt, "redis.sock") }
func redisPidf(rt string) string      { return filepath.Join(rt, "redis.pid") }
func mailpitPidf(rt string) string    { return filepath.Join(rt, "mailpit.pid") }
func mailpitDB(dataDir string) string { return filepath.Join(dataDir, "mailpit.db") }

// MachineLock is the machine-global lock around start/stop. A repo-scoped slot
// lock cannot serialise two different apps' service managers.
type MachineLock struct{ f *os.File }

// Acquire takes an exclusive flock on <config>/lock, blocking until granted.
func (l *MachineLock) Acquire() error {
	cfg := ConfigDir()
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(cfg, "lock"), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return err
	}
	l.f = f
	return nil
}

// Release drops the lock. Safe to call on a zero-value or already-released lock.
func (l *MachineLock) Release() {
	if l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}

const configTemplate = `# propitech-dev shared services — machine-global ports.
# Edit to dodge a squatted well-known port; applies to every app on this machine.
PG_PORT=5431
REDIS_PORT=6379
MAIL_SMTP_PORT=1025
MAIL_UI_PORT=8025
`

const miseTemplate = `# propitech-dev shared services — pinned daemon versions.
# One canonical set for every app's shared Postgres/Redis/Mailpit. Edit to bump.
[tools]
postgres = "16"
redis = "7.4"
"ubi:axllent/mailpit" = "latest"
`

// WriteDefaults writes the machine config and pinned-version mise.toml if
// absent; both become user-editable thereafter.
func WriteDefaults() error {
	cfg := ConfigDir()
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return err
	}
	if err := writeIfAbsent(filepath.Join(cfg, "config"), configTemplate); err != nil {
		return err
	}
	return writeIfAbsent(filepath.Join(cfg, "mise.toml"), miseTemplate)
}

func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// SetConfigPort writes key=value into the machine config file, rewriting the
// matching KEY= line in place (and preserving every other line and comment) or
// appending it if absent. The file is seeded from defaults first when missing,
// so a fresh machine gets a complete, commented file rather than a one-line stub.
func SetConfigPort(key string, value int) error {
	if err := WriteDefaults(); err != nil {
		return err
	}
	path := filepath.Join(ConfigDir(), "config")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	repl := fmt.Sprintf("%s=%d", key, value)
	found := false
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if k, _, ok := strings.Cut(t, "="); ok && strings.TrimSpace(k) == key {
			lines[i] = repl
			found = true
			break
		}
	}
	if !found {
		// Insert before any trailing blank line so the file keeps one final newline.
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines[n-1] = repl
			lines = append(lines, "")
		} else {
			lines = append(lines, repl)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// Prepare ensures mise is present, the config dir and defaults exist, the
// pinned tool versions are installed and trusted, and the data/runtime roots
// are recorded. Returns the loaded config and resolved roots.
func Prepare() (cfg Config, dataDir, runtimeDir string, err error) {
	if _, e := exec.LookPath("mise"); e != nil {
		return cfg, "", "", errors.New("mise not found on PATH")
	}
	if e := WriteDefaults(); e != nil {
		return cfg, "", "", e
	}
	cfgDir := ConfigDir()
	// Idempotent; re-arms after a user edits the file.
	_ = exec.Command("mise", "trust", filepath.Join(cfgDir, "mise.toml")).Run()
	_ = exec.Command("mise", "-C", cfgDir, "install").Run()
	dataDir, runtimeDir = EnsureRegistry()
	return LoadConfig(), dataDir, runtimeDir, nil
}

// retry runs fn until it returns true or ~3s elapses (daemons fork before they
// bind). Returns fn's last result.
func retry(fn func() bool) bool {
	for i := 0; i < 30; i++ {
		if fn() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// StartPostgres initialises the cluster on first use and starts it listening on
// both the unix socket (probes) and TCP 127.0.0.1 (apps). No-op if already up.
func StartPostgres(dataDir, runtimeDir string, pgPort int) {
	pgd := pgData(dataDir)
	_ = os.MkdirAll(pgd, 0o755)
	_ = os.MkdirAll(logDir(dataDir), 0o755)
	_ = os.MkdirAll(runtimeDir, 0o755)

	if _, err := os.Stat(filepath.Join(pgd, "PG_VERSION")); err != nil {
		user := osUser()
		_ = svcRun("initdb", "-D", pgd, "-U", user, "-E", "UTF8", "--auth=trust").Run()
	}
	if svcRun("pg_ctl", "-D", pgd, "status").Run() == nil {
		return // already running
	}
	opts := fmt.Sprintf("-c unix_socket_directories=%s -c listen_addresses=127.0.0.1 -c port=%d", runtimeDir, pgPort)
	_ = svcRun("pg_ctl", "-D", pgd,
		"-l", filepath.Join(logDir(dataDir), "postgres.log"), "-w",
		"-o", opts, "start").Run()
}

// VerifyPostgres reports whether the shared Postgres answers on the socket.
func VerifyPostgres(runtimeDir string, pgPort int) bool {
	return SharedPgUp(runtimeDir, pgPort)
}

// StartRedis starts the shared Redis (socket + TCP). No-op if already up.
func StartRedis(dataDir, runtimeDir string, redisPort int) {
	datadir := filepath.Join(dataDir, "redis")
	_ = os.MkdirAll(datadir, 0o755)
	_ = os.MkdirAll(logDir(dataDir), 0o755)
	_ = os.MkdirAll(runtimeDir, 0o755)

	sock := redisSock(runtimeDir)
	if exec.Command("redis-cli", "-s", sock, "ping").Run() == nil {
		return
	}
	_ = svcRun("redis-server",
		"--daemonize", "yes",
		"--dir", datadir,
		"--unixsocket", sock,
		"--bind", "127.0.0.1",
		"--port", strconv.Itoa(redisPort),
		"--databases", strconv.Itoa(RedisDatabases),
		"--pidfile", redisPidf(runtimeDir),
		"--logfile", filepath.Join(logDir(dataDir), "redis.log")).Run()
}

// StartMailpit backgrounds mailpit under nohup and records its pid. Mailpit has
// no unix socket and its detach behaviour varies by version, so we own the
// backgrounding. No-op if already running.
func StartMailpit(dataDir, runtimeDir string, smtpPort, uiPort int) {
	_ = os.MkdirAll(logDir(dataDir), 0o755)
	_ = os.MkdirAll(runtimeDir, 0o755)
	if MailpitRunning(runtimeDir) {
		return
	}
	script := fmt.Sprintf(
		"nohup mailpit --db-file '%s' --smtp '127.0.0.1:%d' --listen '127.0.0.1:%d' >> '%s' 2>&1 & echo $! > '%s'",
		mailpitDB(dataDir), smtpPort, uiPort,
		filepath.Join(logDir(dataDir), "mailpit.log"), mailpitPidf(runtimeDir))
	_ = svcRun("sh", "-c", script).Run()
}

// StopPostgres stops the cluster if running. Returns (attempted, ok): attempted
// is false when it was not running.
func StopPostgres(dataDir string) (attempted, ok bool) {
	pgd := pgData(dataDir)
	if svcRun("pg_ctl", "-D", pgd, "status").Run() != nil {
		return false, false
	}
	return true, svcRun("pg_ctl", "-D", pgd, "-w", "stop").Run() == nil
}

// StopRedis shuts the shared Redis down if it answers on the socket.
func StopRedis(runtimeDir string) (attempted bool) {
	sock := redisSock(runtimeDir)
	if exec.Command("redis-cli", "-s", sock, "ping").Run() != nil {
		return false
	}
	_ = exec.Command("redis-cli", "-s", sock, "shutdown", "nosave").Run()
	return true
}

// StopMailpit kills our mailpit if running and removes the pidfile. Returns
// (attempted, ok).
func StopMailpit(runtimeDir string) (attempted, ok bool) {
	if !MailpitRunning(runtimeDir) {
		return false, false
	}
	ok = true
	data, err := os.ReadFile(mailpitPidf(runtimeDir))
	if err == nil {
		if pid, e := strconv.Atoi(strings.TrimSpace(string(data))); e == nil {
			ok = syscall.Kill(pid, syscall.SIGTERM) == nil
		}
	}
	_ = os.Remove(mailpitPidf(runtimeDir))
	return true, ok
}

// Retry exposes the internal retry helper to callers verifying a freshly
// started daemon.
func Retry(fn func() bool) bool { return retry(fn) }

func osUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if out, err := exec.Command("id", "-un").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
