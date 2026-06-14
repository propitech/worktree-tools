package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDirXDGSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := ConfigDir(); got != "/xdg/propitech-dev" {
		t.Errorf("ConfigDir = %q, want /xdg/propitech-dev", got)
	}
}

func TestConfigDirXDGUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/alice")
	if got := ConfigDir(); got != "/home/alice/.config/propitech-dev" {
		t.Errorf("ConfigDir = %q, want /home/alice/.config/propitech-dev", got)
	}
}

func TestRegistryGet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	reg := filepath.Join(dir, "propitech-dev", "registry")
	if err := os.MkdirAll(filepath.Dir(reg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reg, []byte("SVC_DATA_DIR=/data\nSVC_RUNTIME_DIR=/run\nSVC_DATA_DIR=/data2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RegistryGet("SVC_DATA_DIR"); got != "/data2" {
		t.Errorf("RegistryGet SVC_DATA_DIR = %q, want /data2 (last wins)", got)
	}
	if got := RegistryGet("SVC_RUNTIME_DIR"); got != "/run" {
		t.Errorf("RegistryGet SVC_RUNTIME_DIR = %q, want /run", got)
	}
	if got := RegistryGet("ABSENT"); got != "" {
		t.Errorf("RegistryGet ABSENT = %q, want empty", got)
	}
}

func TestRegistryGetMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := RegistryGet("ANY"); got != "" {
		t.Errorf("missing registry = %q, want empty", got)
	}
}

func TestRegistrySet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := RegistrySet("FOO", "bar"); err != nil {
		t.Fatalf("RegistrySet: %v", err)
	}
	if got := RegistryGet("FOO"); got != "bar" {
		t.Errorf("after set FOO=bar, got %q", got)
	}

	// overwrite — last value wins
	if err := RegistrySet("FOO", "baz"); err != nil {
		t.Fatalf("RegistrySet overwrite: %v", err)
	}
	if got := RegistryGet("FOO"); got != "baz" {
		t.Errorf("after overwrite FOO=baz, got %q", got)
	}

	// second key doesn't clobber first
	if err := RegistrySet("BAR", "qux"); err != nil {
		t.Fatalf("RegistrySet BAR: %v", err)
	}
	if got := RegistryGet("FOO"); got != "baz" {
		t.Errorf("FOO after BAR set = %q, want baz", got)
	}
	if got := RegistryGet("BAR"); got != "qux" {
		t.Errorf("BAR = %q, want qux", got)
	}
}

func TestEnsureRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_STATE_HOME", dir+"/state")
	t.Setenv("XDG_RUNTIME_DIR", dir+"/run")
	t.Setenv("HOME", dir+"/home")

	data, runtime := EnsureRegistry()
	if data == "" || runtime == "" {
		t.Fatalf("EnsureRegistry returned empty paths: data=%q runtime=%q", data, runtime)
	}
	// stable: second call returns same paths
	data2, runtime2 := EnsureRegistry()
	if data2 != data || runtime2 != runtime {
		t.Errorf("EnsureRegistry not stable: first=%q/%q second=%q/%q", data, runtime, data2, runtime2)
	}
}

func TestRuntimeAndDataDirCleanStoredValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	reg := filepath.Join(dir, "propitech-dev", "registry")
	if err := os.MkdirAll(filepath.Dir(reg), 0o755); err != nil {
		t.Fatal(err)
	}
	// A shell-era doubled-slash value, as written when XDG_RUNTIME_DIR ended
	// in a trailing slash. Both readers must canonicalise it.
	if err := os.WriteFile(reg, []byte("SVC_DATA_DIR=/home/a/.local/state//propitech-dev\nSVC_RUNTIME_DIR=/run/user/1000//propitech-dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, want := RuntimeDir(), "/run/user/1000/propitech-dev"; got != want {
		t.Errorf("RuntimeDir = %q, want %q (cleaned)", got, want)
	}
	if got, want := DataDir(), "/home/a/.local/state/propitech-dev"; got != want {
		t.Errorf("DataDir = %q, want %q (cleaned)", got, want)
	}
}

func TestRuntimeAndDataDirFallbackDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty registry → fall back to XDG
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000/")
	t.Setenv("XDG_STATE_HOME", "/state/")

	if got, want := RuntimeDir(), "/run/user/1000/propitech-dev"; got != want {
		t.Errorf("RuntimeDir fallback = %q, want %q", got, want)
	}
	if got, want := DataDir(), "/state/propitech-dev"; got != want {
		t.Errorf("DataDir fallback = %q, want %q", got, want)
	}
}

func TestRuntimeDirFallbackTmp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got, want := RuntimeDir(), "/tmp/propitech-dev"; got != want {
		t.Errorf("RuntimeDir with no XDG_RUNTIME_DIR = %q, want %q", got, want)
	}
}

func TestSetConfigPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Fresh machine: file seeded from defaults, then one key changed.
	if err := SetConfigPort("PG_PORT", 5500); err != nil {
		t.Fatalf("SetConfigPort: %v", err)
	}
	cfg := LoadConfig()
	if cfg.PGPort != 5500 {
		t.Errorf("PGPort = %d, want 5500", cfg.PGPort)
	}
	// Other keys keep their template defaults — the rewrite is surgical.
	if cfg.RedisPort != 6379 || cfg.MailSMTPPort != 1025 || cfg.MailUIPort != 8025 {
		t.Errorf("untouched keys changed: %+v", cfg)
	}

	// The commented header survives the rewrite.
	raw, err := os.ReadFile(filepath.Join(dir, "propitech-dev", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# propitech-dev shared services") {
		t.Errorf("comment header lost:\n%s", raw)
	}

	// A second change to a different key leaves the first intact.
	if err := SetConfigPort("REDIS_PORT", 6400); err != nil {
		t.Fatalf("SetConfigPort REDIS_PORT: %v", err)
	}
	cfg = LoadConfig()
	if cfg.PGPort != 5500 || cfg.RedisPort != 6400 {
		t.Errorf("after second set: %+v, want PG 5500 / Redis 6400", cfg)
	}
}

func TestRepoValueRoundTripAndIsolation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	const a = "/work/a/property_management/.git"
	const b = "/work/b/property_management/.git" // same repo name, different clone

	if err := SetRepoValue(a, "WORKTREE_ROOT", "/wt/a"); err != nil {
		t.Fatalf("SetRepoValue a: %v", err)
	}
	if got := RepoValue(a, "WORKTREE_ROOT"); got != "/wt/a" {
		t.Errorf("RepoValue a = %q, want /wt/a", got)
	}
	// A same-named fork (different common dir) must not see a's value.
	if got := RepoValue(b, "WORKTREE_ROOT"); got != "" {
		t.Errorf("RepoValue b = %q, want empty (no collision across forks)", got)
	}
	if err := SetRepoValue(b, "WORKTREE_ROOT", "/wt/b"); err != nil {
		t.Fatalf("SetRepoValue b: %v", err)
	}
	if got := RepoValue(a, "WORKTREE_ROOT"); got != "/wt/a" {
		t.Errorf("after b set, RepoValue a = %q, want /wt/a", got)
	}
}

func TestRepoKeyDistinctAndStable(t *testing.T) {
	ka := RepoKey("/work/a/.git", "WORKTREE_ROOT")
	kb := RepoKey("/work/b/.git", "WORKTREE_ROOT")
	if ka == kb {
		t.Errorf("RepoKey collided across common dirs: %q", ka)
	}
	// Stable + insensitive to a trailing slash (cleaned before hashing).
	if RepoKey("/work/a/.git/", "WORKTREE_ROOT") != ka {
		t.Errorf("RepoKey not stable under trailing slash")
	}
}

func TestAppBase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	base0, err := AppBase("myapp")
	if err != nil {
		t.Fatalf("AppBase myapp: %v", err)
	}
	if base0 != 0 {
		t.Errorf("first app base = %d, want 0", base0)
	}

	// stable for same app
	base0again, err := AppBase("myapp")
	if err != nil {
		t.Fatalf("AppBase myapp again: %v", err)
	}
	if base0again != 0 {
		t.Errorf("repeated AppBase = %d, want 0", base0again)
	}

	// second app gets stride-16 offset
	base1, err := AppBase("otherapp")
	if err != nil {
		t.Fatalf("AppBase otherapp: %v", err)
	}
	if base1 != 16 {
		t.Errorf("second app base = %d, want 16", base1)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgDir := filepath.Join(dir, "propitech-dev")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("custom ports", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(`
# comment
PG_PORT=5432
REDIS_PORT=6380
MAIL_SMTP_PORT=1026
MAIL_UI_PORT=8026
`), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := LoadConfig()
		if cfg.PGPort != 5432 {
			t.Errorf("PGPort = %d, want 5432", cfg.PGPort)
		}
		if cfg.RedisPort != 6380 {
			t.Errorf("RedisPort = %d, want 6380", cfg.RedisPort)
		}
		if cfg.MailSMTPPort != 1026 {
			t.Errorf("MailSMTPPort = %d, want 1026", cfg.MailSMTPPort)
		}
		if cfg.MailUIPort != 8026 {
			t.Errorf("MailUIPort = %d, want 8026", cfg.MailUIPort)
		}
	})

	t.Run("missing file defaults", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		cfg := LoadConfig()
		if cfg.PGPort != 5431 || cfg.RedisPort != 6379 || cfg.MailSMTPPort != 1025 || cfg.MailUIPort != 8025 {
			t.Errorf("defaults wrong: %+v", cfg)
		}
	})
}
