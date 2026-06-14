package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDecideAutoadopt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		hasSlot   bool
		sessionID string
		owner     string
		want      decision
	}{
		{"unclaimed unprovisioned adopts", false, "S1", "", decideAdopt},
		{"own unprovisioned adopts", false, "S1", "S1", decideAdopt},
		{"foreign unprovisioned refuses", false, "S2", "S1", decideForeign},
		{"already provisioned for us skips", true, "S1", "S1", decideSkip},
		{"already provisioned foreign flagged", true, "S2", "S1", decideForeign},
		{"provisioned unknown owner skips", true, "S1", "", decideSkip},
		// No session id → permissive fallback (historical behaviour).
		{"no session unprovisioned adopts", false, "", "S1", decideAdopt},
		{"no session provisioned skips", true, "", "S1", decideSkip},
		{"unclaimed no session adopts", false, "", "", decideAdopt},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := decideAutoadopt(c.hasSlot, c.sessionID, c.owner); got != c.want {
				t.Errorf("decideAutoadopt(%v, %q, %q) = %d, want %d",
					c.hasSlot, c.sessionID, c.owner, got, c.want)
			}
		})
	}
}

func TestSessionIDFrom(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		in   string
		want string
	}{
		"extracts session_id": {`{"session_id":"abc-123","cwd":"/x"}`, "abc-123"},
		"trims whitespace":    {`{"session_id":"  abc  "}`, "abc"},
		"empty payload":       {"", ""},
		"absent key":          {`{"cwd":"/x"}`, ""},
		"malformed json":      {`not json`, ""},
		"empty session_id":    {`{"session_id":""}`, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := sessionIDFrom(strings.NewReader(c.in)); got != c.want {
				t.Errorf("sessionIDFrom(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestOwnerRoundTrip(t *testing.T) {
	t.Parallel()
	common := t.TempDir()
	wt := "/repos/app/.claude/worktrees/keen-badger"
	path := ownerPathFor(common, wt)

	if got := readOwner(path); got != "" {
		t.Fatalf("unwritten owner = %q, want empty", got)
	}
	writeOwner(path, "session-xyz")
	if got := readOwner(path); got != "session-xyz" {
		t.Errorf("owner = %q, want session-xyz", got)
	}
	if base := filepath.Base(path); base != "keen-badger" {
		t.Errorf("owner file = %q, want keyed by worktree name keen-badger", base)
	}
}
