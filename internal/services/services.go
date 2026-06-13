// Package services reads the machine-global propitech-dev config and probes
// shared service daemons (Postgres, Redis). The config directory follows XDG
// conventions: ~/.config/propitech-dev by default.
package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// LegacyPgUp probes the per-slot unix socket that legacy (non-shared) worktrees
// use. sockPrefix is read from the primary worktree's mise env (SOCK_PREFIX).
func LegacyPgUp(xdgRuntime, sockPrefix string, slot, dbPort int) bool {
	socketDir := filepath.Join(xdgRuntime, fmt.Sprintf("%s-%d", sockPrefix, slot))
	return pgIsReady(socketDir, dbPort)
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
