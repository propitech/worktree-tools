package main

import (
	"io"
	"testing"
)

func TestCdMissingTarget(t *testing.T) {
	t.Parallel()
	if got := run([]string{"cd"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([cd]) with no target = %d, want 2", got)
	}
}

func TestCdUnknownFlag(t *testing.T) {
	t.Parallel()
	if got := run([]string{"cd", "--bogus"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([cd --bogus]) = %d, want 2", got)
	}
}

func TestCdNoMatch(t *testing.T) {
	t.Parallel()
	// Runs inside the repo's own worktree, so git topology is real; an
	// unmatched target resolves to nothing and exits 1.
	if got := run([]string{"cd", "definitely-not-a-worktree-xyz"}, io.Discard, io.Discard); got != 1 {
		t.Errorf("run([cd <no match>]) = %d, want 1", got)
	}
}

func TestRunMissingTarget(t *testing.T) {
	t.Parallel()
	if got := run([]string{"run"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([run]) with no target = %d, want 2", got)
	}
}

func TestRunMissingCommand(t *testing.T) {
	t.Parallel()
	// A target with no command to run is a usage error.
	if got := run([]string{"run", "somewt"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([run somewt]) with no command = %d, want 2", got)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	t.Parallel()
	if got := run([]string{"run", "--bogus", "echo"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([run --bogus echo]) = %d, want 2", got)
	}
}

func TestRunNoMatch(t *testing.T) {
	t.Parallel()
	// Real git topology; unmatched target exits 1 before exec.
	if got := run([]string{"run", "definitely-not-a-worktree-xyz", "echo", "hi"}, io.Discard, io.Discard); got != 1 {
		t.Errorf("run([run <no match> echo hi]) = %d, want 1", got)
	}
}
