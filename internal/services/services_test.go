package services

import (
	"os"
	"path/filepath"
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
