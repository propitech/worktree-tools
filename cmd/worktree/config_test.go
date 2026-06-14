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

func TestValueOr(t *testing.T) {
	t.Parallel()
	if valueOr("", "fallback") != "fallback" {
		t.Error("valueOr(empty) should return fallback")
	}
	if valueOr("set", "fallback") != "set" {
		t.Error("valueOr(set) should return the value")
	}
}
