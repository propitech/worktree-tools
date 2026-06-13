// Package services reads and writes the machine-global propitech-dev config,
// probes shared service daemons (Postgres, Redis), and allocates per-app slot
// bases in the machine registry. The config directory follows XDG conventions:
// ~/.config/propitech-dev by default.
package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ConfigDir returns the machine-global propitech-dev config directory,
// honouring XDG_CONFIG_HOME.
func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "propitech-dev")
}

// RegistryGet reads the last value recorded for key in the flat KEY=value
// machine registry. Returns "" when the file or key is absent.
func RegistryGet(key string) string {
	f, err := os.Open(filepath.Join(ConfigDir(), "registry"))
	if err != nil {
		return ""
	}
	defer f.Close()
	var last string
	sc := bufio.NewScanner(f)
	prefix := key + "="
	for sc.Scan() {
		if l := sc.Text(); strings.HasPrefix(l, prefix) {
			last = strings.TrimPrefix(l, prefix)
		}
	}
	return last
}

// Config holds the machine-global service ports from
// ~/.config/propitech-dev/config. Absent or unreadable file → defaults.
type Config struct {
	PGPort       int
	RedisPort    int
	MailSMTPPort int
	MailUIPort   int
}

// LoadConfig reads the machine config file. Missing file → defaults.
func LoadConfig() Config {
	cfg := Config{PGPort: 5431, RedisPort: 6379, MailSMTPPort: 1025, MailUIPort: 8025}
	f, err := os.Open(filepath.Join(ConfigDir(), "config"))
	if err != nil {
		return cfg
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		n, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		switch k {
		case "PG_PORT":
			cfg.PGPort = n
		case "REDIS_PORT":
			cfg.RedisPort = n
		case "MAIL_SMTP_PORT":
			cfg.MailSMTPPort = n
		case "MAIL_UI_PORT":
			cfg.MailUIPort = n
		}
	}
	return cfg
}

// SharedPgUp probes whether the shared Postgres unix socket is reachable via
// pg_isready. runtimeDir is SVC_RUNTIME_DIR from the machine registry.
func SharedPgUp(runtimeDir string, pgPort int) bool {
	return pgIsReady(runtimeDir, pgPort)
}

// TCPPgUp probes a TCP postgres endpoint (used to detect a foreign Postgres
// squatting the shared port).
func TCPPgUp(host string, port int) bool {
	return pgIsReady(host, port)
}

func pgIsReady(host string, port int) bool {
	cmd := exec.Command("pg_isready", "-h", host, "-p", strconv.Itoa(port), "-q")
	return cmd.Run() == nil
}

// DBExists probes whether dbName exists in the shared cluster via psql. The
// -X flag suppresses ~/.psqlrc so timing output or other config cannot corrupt
// the query result.
func DBExists(runtimeDir string, pgPort int, dbName string) bool {
	cmd := exec.Command("psql",
		"-X", "-h", runtimeDir, "-p", strconv.Itoa(pgPort),
		"-d", "postgres", "-tAc",
		"SELECT 1 FROM pg_database WHERE datname='"+dbName+"'")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

// RedisDatabases is the database count the shared Redis runs with — REDIS_DB =
// app_base + slot, so each app owns a contiguous 16-slot band (64 apps).
const RedisDatabases = 1024

// VerifyRedis reports whether the shared Redis is the one we own: it must answer
// PING on the unix socket AND report `databases == RedisDatabases` over TCP.
// `redis-server --daemonize` exits 0 even when the TCP bind fails (a foreign
// Redis squatting the port), and apps connect by TCP, so a socket-only check
// would pass while apps hit the wrong server.
func VerifyRedis(sock string, redisPort int) bool {
	out, err := exec.Command("redis-cli", "-s", sock, "ping").Output()
	if err != nil || strings.TrimSpace(string(out)) != "PONG" {
		return false
	}
	out, err = exec.Command("redis-cli", "-h", "127.0.0.1", "-p", strconv.Itoa(redisPort),
		"config", "get", "databases").Output()
	if err != nil {
		return false
	}
	// `config get` returns the key then the value; the value is the last field.
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return false
	}
	return fields[len(fields)-1] == strconv.Itoa(RedisDatabases)
}

// RedisForeignUp reports whether some Redis answers PING on the shared TCP port
// — used to flag a foreign Redis squatting it.
func RedisForeignUp(redisPort int) bool {
	return exec.Command("redis-cli", "-h", "127.0.0.1", "-p", strconv.Itoa(redisPort), "ping").Run() == nil
}

// MailpitRunning reports whether our mailpit is alive. Mailpit has no unix
// socket, so health is pidfile + liveness + a process-name check guarding
// against pid reuse.
func MailpitRunning(runtimeDir string) bool {
	data, err := os.ReadFile(filepath.Join(runtimeDir, "mailpit.pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	return strings.Contains(procComm(pid), "mailpit")
}

// procComm returns the process command name for pid, portably: /proc on Linux
// (WSL2), ps elsewhere (macOS).
func procComm(pid int) string {
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		return strings.TrimSpace(string(data))
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// DropDB drops dbName from the shared cluster with FORCE (terminating live
// connections). Returns true when the DROP succeeds. IF EXISTS makes it a
// no-op for already-absent databases — those still report success.
func DropDB(runtimeDir string, pgPort int, dbName string) bool {
	cmd := exec.Command("psql",
		"-X", "-h", runtimeDir, "-p", strconv.Itoa(pgPort),
		"-d", "postgres", "-c",
		`DROP DATABASE IF EXISTS "`+dbName+`" WITH (FORCE)`)
	return cmd.Run() == nil
}

// RedisUp reports whether the shared Redis answers PING on the unix socket.
func RedisUp(sock string) bool {
	if _, err := os.Stat(sock); err != nil {
		return false
	}
	return exec.Command("redis-cli", "-s", sock, "ping").Run() == nil
}

// RedisClientsOnDB counts clients currently selected onto redis db index db —
// a zombie Sidekiq would otherwise contaminate the slot when it's reused.
func RedisClientsOnDB(sock string, db int) int {
	out, err := exec.Command("redis-cli", "-s", sock, "CLIENT", "LIST").Output()
	if err != nil {
		return 0
	}
	return strings.Count(string(out), " db="+strconv.Itoa(db)+" ")
}

// RedisFlushDB flushes only the given redis db index. Returns true on success.
func RedisFlushDB(sock string, db int) bool {
	cmd := exec.Command("redis-cli", "-s", sock, "-n", strconv.Itoa(db), "FLUSHDB")
	return cmd.Run() == nil
}

// RegistrySet atomically writes key=value to the machine registry, replacing
// any existing entry for key.
func RegistrySet(key, value string) error {
	cfg := ConfigDir()
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return err
	}
	reg := filepath.Join(cfg, "registry")
	tmp := reg + ".tmp." + strconv.Itoa(os.Getpid())

	var lines []string
	if f, err := os.Open(reg); err == nil {
		sc := bufio.NewScanner(f)
		prefix := key + "="
		for sc.Scan() {
			if l := sc.Text(); !strings.HasPrefix(l, prefix) {
				lines = append(lines, l)
			}
		}
		f.Close()
	}
	lines = append(lines, key+"="+value)

	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, reg)
}

// EnsureRegistry ensures SVC_DATA_DIR and SVC_RUNTIME_DIR are recorded in the
// machine registry, initialising them from XDG conventions on first use.
// Returns (dataDir, runtimeDir).
func EnsureRegistry() (dataDir, runtimeDir string) {
	dataDir = RegistryGet("SVC_DATA_DIR")
	runtimeDir = RegistryGet("SVC_RUNTIME_DIR")
	if dataDir == "" {
		stateHome := os.Getenv("XDG_STATE_HOME")
		if stateHome == "" {
			stateHome = filepath.Join(os.Getenv("HOME"), ".local", "state")
		}
		dataDir = filepath.Join(stateHome, "propitech-dev")
		_ = RegistrySet("SVC_DATA_DIR", dataDir)
	}
	if runtimeDir == "" {
		xdg := os.Getenv("XDG_RUNTIME_DIR")
		if xdg == "" {
			xdg = "/tmp"
		}
		runtimeDir = filepath.Join(xdg, "propitech-dev")
		_ = RegistrySet("SVC_RUNTIME_DIR", runtimeDir)
	}
	return dataDir, runtimeDir
}

// AppBase returns the per-app Redis DB index base (stride 16), allocated once
// in the machine registry and stable thereafter. The machine config lock
// serialises concurrent allocation so each app gets a unique band.
func AppBase(app string) (int, error) {
	key := "APP_BASE_" + app
	if v := RegistryGet(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n, nil
		}
	}
	cfg := ConfigDir()
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		return 0, err
	}
	lf, err := os.OpenFile(filepath.Join(cfg, "lock"), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return 0, err
	}
	defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()

	if v := RegistryGet(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n, nil
		}
	}

	reg := filepath.Join(cfg, "registry")
	n := 0
	if f, err := os.Open(reg); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "APP_BASE_") {
				n++
			}
		}
		f.Close()
	}
	base := n * 16
	if err := RegistrySet(key, strconv.Itoa(base)); err != nil {
		return 0, err
	}
	return base, nil
}

// CreateDevDB creates the user's convenience database on the shared Postgres
// cluster if it doesn't already exist. Best-effort; silently skipped on error.
func CreateDevDB(runtimeDir string, pgPort int) {
	user := os.Getenv("USER")
	if user == "" {
		if out, err := exec.Command("id", "-un").Output(); err == nil {
			user = strings.TrimSpace(string(out))
		}
	}
	if user == "" {
		return
	}
	cmd := exec.Command("psql", "-X", "-h", runtimeDir, "-p", strconv.Itoa(pgPort),
		"-d", "postgres", "-tAc",
		"SELECT 1 FROM pg_database WHERE datname='"+user+"'")
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) == "1" {
		return
	}
	_ = exec.Command("createdb", "-h", runtimeDir, "-p", strconv.Itoa(pgPort), user).Run()
}

// AppEnvVar reads a single variable from the app's resolved mise [env] at
// dir. Returns "" when mise is unavailable or the variable is unset. Used to
// detect stale-contract worktrees and the legacy SOCK_PREFIX.
func AppEnvVar(dir, varName string) string {
	cmd := exec.Command("mise", "x", "--",
		"sh", "-c", fmt.Sprintf(`printf '%%s' "${%s:-}"`, varName))
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
