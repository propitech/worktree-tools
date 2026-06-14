package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/propitech/worktree-tools/internal/env"
	"github.com/propitech/worktree-tools/internal/git"
)

// ownersDir is the per-repo registry (under the shared git common dir) that
// records which Claude Code session owns each isolation worktree. It lives
// outside every working tree, so it survives a worktree's .env being wiped and
// never shows up as an untracked file.
const ownersDir = "claude-worktree-owners"

// decision is the outcome of the autoadopt ownership policy.
type decision int

const (
	// decideAdopt: this worktree is unclaimed (or claimed by us) and not yet
	// provisioned — claim and adopt it.
	decideAdopt decision = iota
	// decideSkip: nothing to do (already provisioned for us, or we can't tell
	// who owns it). Stay a silent no-op.
	decideSkip
	// decideForeign: the worktree belongs to a different Claude session — do not
	// adopt or re-provision it; leave the owner's namespace alone.
	decideForeign
)

// decideAutoadopt is the pure ownership policy. A session must only adopt a
// worktree it created (or already owns); it must never claim one that belongs
// to another live session — that is what let two sessions share one checkout
// and trample each other's branch (PRO-200).
//
//	hasSlot   — the worktree's .env already records a slot (already provisioned)
//	sessionID — this session's Claude id ("" when the hook gave us no payload)
//	owner     — the session id recorded for this worktree ("" when unclaimed)
//
// When we have no session id (manual run, legacy hook payload) the policy is
// deliberately permissive — it falls back to the historical behaviour of
// adopting the current worktree — so the change is a pure tightening with no
// regression where ownership can't be established.
func decideAutoadopt(hasSlot bool, sessionID, owner string) decision {
	foreign := sessionID != "" && owner != "" && owner != sessionID

	if hasSlot {
		if foreign {
			return decideForeign
		}
		// Already provisioned for us, or owner unknown — adopt is a no-op anyway.
		return decideSkip
	}
	if foreign {
		// Unprovisioned but already claimed by another session (its slot/.env
		// was lost or never written). Refuse rather than steal it.
		return decideForeign
	}
	return decideAdopt
}

// hookPayload is the slice of the Claude Code SessionStart hook JSON (delivered
// on stdin) that we care about.
type hookPayload struct {
	SessionID string `json:"session_id"`
}

// sessionIDFrom extracts the Claude session id from a SessionStart hook stdin
// payload. Any problem (empty, not a pipe, malformed JSON) yields "" — the
// caller then takes the permissive no-session path.
func sessionIDFrom(r io.Reader) string {
	data, err := io.ReadAll(r)
	if err != nil || len(data) == 0 {
		return ""
	}
	var p hookPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.SessionID)
}

// readOwner returns the session id recorded for the worktree, or "" if none.
func readOwner(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeOwner records sessionID as the owner of the worktree, best-effort.
func writeOwner(path, sessionID string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(sessionID+"\n"), 0o644)
}

// ownerPathFor returns the owner-registry file for the worktree at wtPath,
// keyed by its directory name under the shared git common dir.
func ownerPathFor(commonDir, wtPath string) string {
	return filepath.Join(commonDir, ownersDir, filepath.Base(wtPath))
}

// cmdAutoadopt is the SessionStart-hook entry point. When a shell opens inside a
// Claude Code isolation worktree (.claude/worktrees/<name>), it adopts the
// worktree into a slot so its dev services don't collide with the primary
// checkout. Adopt-only: services stay DOWN.
//
// It only adopts a worktree this session owns: the Claude session id from the
// hook's stdin payload is recorded on first adopt, and a later run by a
// different session refuses rather than re-claiming it (PRO-200). It is a silent
// no-op everywhere else, and on every run after the first. A failure must never
// abort the session, so it always returns 0. Wire it as
// `command: worktree autoadopt` in .claude/settings.json.
//
// There is deliberately no auto-remove counterpart: tearing a worktree down on
// session end would risk unfinished/uncommitted work. Remove explicitly with
// `worktree rm <name>`.
func cmdAutoadopt(_ []string, stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	if !strings.Contains(filepath.ToSlash(cwd), "/.claude/worktrees/") {
		return 0
	}
	if _, err := exec.LookPath("git"); err != nil {
		return 0
	}

	sessionID := sessionIDFrom(stdinPayload())

	commonDir, err := git.CommonDir(cwd)
	if err != nil {
		// Can't locate the registry — fall back to the historical adopt-cwd path
		// rather than blocking the session.
		cmdAdopt(nil, stdout, stderr)
		return 0
	}
	ownerPath := ownerPathFor(commonDir, cwd)
	owner := readOwner(ownerPath)
	_, hasSlot := env.Slot(filepath.Join(cwd, ".env"))

	switch decideAutoadopt(hasSlot, sessionID, owner) {
	case decideSkip:
		return 0
	case decideForeign:
		fmt.Fprintf(stderr,
			"==> Not adopting %s: it belongs to another Claude session (%s). Create your own worktree instead.\n",
			cwd, owner)
		return 0
	case decideAdopt:
		// adopt guards against the primary checkout / an already-managed worktree;
		// swallow its exit code so a failure never aborts the session.
		if cmdAdopt(nil, stdout, stderr) == 0 && sessionID != "" {
			writeOwner(ownerPath, sessionID)
		}
		return 0
	}
	return 0
}

// stdinPayload returns os.Stdin only when it is a pipe carrying the hook JSON;
// for an interactive terminal it returns an empty reader so a manual
// `worktree autoadopt` never blocks waiting on a tty.
func stdinPayload() io.Reader {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return strings.NewReader("")
	}
	return os.Stdin
}
