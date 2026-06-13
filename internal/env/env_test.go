package env

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEnv writes content to a .env in a fresh temp dir and returns its path.
func writeEnv(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestValue(t *testing.T) {
	t.Parallel()
	env := writeEnv(t, `# a comment
WORKTREE_APP = "myapp"
WORKTREE_DB_SUFFIX=_s3
REDIS_URL=redis://localhost:6379/19
EMPTY=
QUOTED='shared'

WORKTREE_APP=overwritten
`)
	cases := map[string]string{
		"WORKTREE_APP":       "overwritten", // last assignment wins
		"WORKTREE_DB_SUFFIX": "_s3",
		"REDIS_URL":          "redis://localhost:6379/19", // colons/slashes survive
		"EMPTY":              "",
		"QUOTED":             "shared", // surrounding quotes stripped
		"ABSENT":             "",
	}
	for k, want := range cases {
		if got := Value(env, k); got != want {
			t.Errorf("Value(%q) = %q, want %q", k, got, want)
		}
	}
}

func TestValueMissingFile(t *testing.T) {
	t.Parallel()
	if got := Value(filepath.Join(t.TempDir(), "nope", ".env"), "X"); got != "" {
		t.Errorf("missing file = %q, want empty", got)
	}
}

func TestSlot(t *testing.T) {
	t.Parallel()
	if n, ok := Slot(writeEnv(t, "WORKTREE_SLOT=3\n")); !ok || n != 3 {
		t.Errorf("Slot = %d,%v want 3,true", n, ok)
	}
	if _, ok := Slot(writeEnv(t, "WORKTREE_SLOT=abc\n")); ok {
		t.Error("non-numeric slot should report ok=false")
	}
	if _, ok := Slot(writeEnv(t, "DB_PORT=5431\n")); ok {
		t.Error("absent slot should report ok=false")
	}
}
