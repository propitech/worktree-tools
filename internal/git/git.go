// Package git inspects the repo's worktree topology via `git worktree list`.
// The primary working tree (slot 0) is the first entry and holds the real .git
// dir; bin/worktree creates siblings named "<repo>-<slug>".
//
// We shell out to the git binary rather than use a Go git library: go-git has
// no support for linked worktrees — it can neither enumerate them
// (`git worktree list`) nor create/remove them (`git worktree add|remove`),
// which are the tool's core operations. A pure-Go library would only cover
// ancillary reads, so it would add a heavy dependency without removing the git
// dependency. The git CLI is already a hard requirement of the tool.
package git

import (
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktrees returns the registered worktree paths, primary first, in the order
// `git worktree list --porcelain` reports them. git is run from dir.
func Worktrees(dir string) ([]string, error) {
	out, err := porcelain(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// Main returns the primary working tree path — the first `git worktree list`
// entry, whose basename is the repo name and whose parent holds the siblings.
func Main(dir string) (string, error) {
	paths, err := Worktrees(dir)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", errors.New("git: no worktrees found")
	}
	return paths[0], nil
}

// SlugOf returns the token `rm` accepts for a worktree: the add-slug (dir
// basename minus the "<repo>-" prefix) or, for other worktrees (e.g. Claude
// isolation ones), the bare basename. Empty for the primary checkout.
func SlugOf(main, path string) string {
	if path == main {
		return ""
	}
	base := filepath.Base(path)
	if slug, ok := strings.CutPrefix(base, filepath.Base(main)+"-"); ok {
		return slug
	}
	return base
}

// CommonDir returns the absolute path to the shared git common directory (the
// real .git dir shared across all linked worktrees).
func CommonDir(from string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = from
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(from, p)
	}
	return p, nil
}

// Add creates a new linked worktree at dest on a new branch named branch.
// Git progress output is written to w.
func Add(from, dest, branch string, w io.Writer) error {
	cmd := exec.Command("git", "worktree", "add", dest, "-b", branch)
	cmd.Dir = from
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

func porcelain(dir string) (string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
