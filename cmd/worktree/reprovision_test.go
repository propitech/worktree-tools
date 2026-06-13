package main

import (
	"io"
	"testing"
)

func TestReprovisionUnknownFlag(t *testing.T) {
	t.Parallel()
	if got := run([]string{"reprovision", "--bogus"}, io.Discard, io.Discard); got != 2 {
		t.Errorf("run([reprovision --bogus]) = %d, want 2", got)
	}
}

func TestReprovisionUnresolvableTarget(t *testing.T) {
	t.Parallel()
	// A target that resolves to no registered worktree → exit 1. Runs inside
	// the repo, so git topology is real.
	if got := run([]string{"reprovision", "no-such-worktree-xyz"}, io.Discard, io.Discard); got != 1 {
		t.Errorf("run([reprovision <miss>]) = %d, want 1", got)
	}
}
