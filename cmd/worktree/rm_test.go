package main

import (
	"io"
	"testing"
)

func TestRmMissingTarget(t *testing.T) {
	t.Parallel()
	if got := run([]string{"rm"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([rm]) with no target = %d, want 2", got)
	}
}

func TestRmUnknownFlag(t *testing.T) {
	t.Parallel()
	if got := run([]string{"rm", "--bogus"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([rm --bogus]) = %d, want 2", got)
	}
}

func TestRmRefusesPrimary(t *testing.T) {
	t.Parallel()
	// Slot 0 resolves to the primary checkout; rm must refuse it. Runs inside
	// the repo's own worktree, so git topology is real.
	if got := run([]string{"rm", "0"}, io.Discard, io.Discard); got != 1 {
		t.Errorf("run([rm 0]) = %d, want 1", got)
	}
}

func TestResolveWorktree(t *testing.T) {
	t.Parallel()
	mainPath := "/repo/app"
	worktrees := []string{
		"/repo/app",      // primary
		"/repo/app-foo",  // legacy <repo>-<slug> sibling
		"/elsewhere/bar", // claude-style, unique basename
		"/other/dup",
		"/another/dup", // duplicate basename "dup"
	}
	cases := []struct {
		name string
		arg  string
		want string // "" means resolution should fail
	}{
		{"legacy slug", "foo", "/repo/app-foo"},
		{"unique basename", "bar", "/elsewhere/bar"},
		{"slot zero is primary", "0", "/repo/app"},
		{"ambiguous basename", "dup", ""},
		{"no match", "nope", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := resolveWorktree(tc.arg, mainPath, worktrees, io.Discard)
			if tc.want == "" {
				if ok {
					t.Errorf("resolveWorktree(%q) = %q, want failure", tc.arg, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("resolveWorktree(%q) = (%q, %v), want %q", tc.arg, got, ok, tc.want)
			}
		})
	}
}
