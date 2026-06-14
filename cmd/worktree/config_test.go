package main

import (
	"io"
	"strings"
	"testing"
)

func TestConfigUnknownAction(t *testing.T) {
	t.Parallel()
	if got := run([]string{"config", "bogus"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([config bogus]) = %d, want 2", got)
	}
}

func TestConfigShowSections(t *testing.T) {
	t.Parallel()
	// Run from inside the repo: every section header prints, and the resolved
	// directories / endpoints appear regardless of whether the shared daemons
	// are up (probes fail gracefully).
	var out strings.Builder
	if got := run([]string{"config", "show"}, &out, io.Discard); got != 0 {
		t.Fatalf("run([config show]) = %d, want 0", got)
	}
	s := out.String()
	for _, want := range []string{
		"Global", "Worktree creation", "Service endpoints", "This worktree",
		"config dir", "primary checkout", "postgres", "mailpit ui",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("config show output missing %q\n---\n%s", want, s)
		}
	}
}

func TestConfigDefaultsToShow(t *testing.T) {
	t.Parallel()
	// Bare `config` (no action) defaults to show.
	var out strings.Builder
	if got := run([]string{"config"}, &out, io.Discard); got != 0 {
		t.Fatalf("run([config]) = %d, want 0", got)
	}
	if !strings.Contains(out.String(), "worktree config") {
		t.Errorf("bare config did not default to show:\n%s", out.String())
	}
}

func TestDatabaseNames(t *testing.T) {
	t.Parallel()
	got := databaseNames("propitech", "_s1")
	want := []string{
		"propitech_development_s1",
		"propitech_development_cache_s1",
		"propitech_development_queue_s1",
		"propitech_development_cable_s1",
		"propitech_test_s1",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("databaseNames = %v, want %v", got, want)
	}
}

func TestConfigSetRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
	}{
		{"wrong arity", []string{"config", "set", "PG_PORT"}},
		{"unknown key", []string{"config", "set", "BOGUS", "1"}},
		{"non-numeric port", []string{"config", "set", "PG_PORT", "abc"}},
		{"port out of range", []string{"config", "set", "PG_PORT", "99999"}},
		{"relative dir", []string{"config", "set", "SVC_DATA_DIR", "rel/path"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := run(tc.args, io.Discard, io.Discard); got != 2 {
				t.Errorf("run(%v) = %d, want 2", tc.args, got)
			}
		})
	}
}

func TestConfigSetPortPersists(t *testing.T) {
	// t.Setenv forbids t.Parallel. Isolate the machine config in a temp dir.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := run([]string{"config", "set", "MAIL_UI_PORT", "8030"}, io.Discard, io.Discard); got != 0 {
		t.Fatalf("config set MAIL_UI_PORT = %d, want 0", got)
	}
	var out strings.Builder
	if run([]string{"config", "show"}, &out, io.Discard); !strings.Contains(out.String(), "8030") {
		t.Errorf("config show did not reflect the set port:\n%s", out.String())
	}
}

func TestConfigSetDirCleansAbsolute(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := run([]string{"config", "set", "SVC_DATA_DIR", "/srv//state/"}, io.Discard, io.Discard); got != 0 {
		t.Fatalf("config set SVC_DATA_DIR = %d, want 0", got)
	}
	var out strings.Builder
	if run([]string{"config", "show"}, &out, io.Discard); !strings.Contains(out.String(), "/srv/state") {
		t.Errorf("config show did not reflect the cleaned dir:\n%s", out.String())
	}
}

func TestConfigSetWorktreeRoot(t *testing.T) {
	// Per-repo root: relative rejected, absolute round-trips through show.
	// Tests run inside this git worktree, so git.CommonDir resolves.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if got := run([]string{"config", "set", "WORKTREE_ROOT", "rel/path"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("relative WORKTREE_ROOT = %d, want 2", got)
	}
	if got := run([]string{"config", "set", "WORKTREE_ROOT", "/srv//wt/"}, io.Discard, io.Discard); got != 0 {
		t.Fatalf("absolute WORKTREE_ROOT = %d, want 0", got)
	}
	var out strings.Builder
	run([]string{"config", "show"}, &out, io.Discard)
	if !strings.Contains(out.String(), "/srv/wt") || !strings.Contains(out.String(), "repo config") {
		t.Errorf("config show did not reflect the per-repo root:\n%s", out.String())
	}
}

func TestAddRelativeRootRejected(t *testing.T) {
	t.Parallel()
	if got := run([]string{"add", "myslug", "--root", "rel/path"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("add --root rel/path = %d, want 2", got)
	}
}

func TestValueOr(t *testing.T) {
	t.Parallel()
	if valueOr("", "fallback") != "fallback" {
		t.Error("valueOr(empty) should return fallback")
	}
	if valueOr("set", "fallback") != "set" {
		t.Error("valueOr(set) should return the value")
	}
}
