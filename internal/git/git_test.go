package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSlugOf(t *testing.T) {
	t.Parallel()
	const main = "/home/me/app"
	cases := []struct{ path, want string }{
		{"/home/me/app", ""},                                      // primary checkout
		{"/home/me/app-lease-renewal", "lease-renewal"},           // add-slug sibling
		{"/home/me/.claude/worktrees/curious-fox", "curious-fox"}, // bare basename
		{"/elsewhere/app-feat/deep", "deep"},                      // no repo- prefix
	}
	for _, c := range cases {
		if got := SlugOf(main, c.path); got != c.want {
			t.Errorf("SlugOf(%q, %q) = %q, want %q", main, c.path, got, c.want)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestWorktreesAndMain(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	repo := filepath.Join(parent, "app")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-qm", "init")
	runGit(t, repo, "worktree", "add", "-q", filepath.Join(parent, "app-feat1"), "-b", "feat/feat1")

	paths, err := Worktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("Worktrees = %v, want 2 entries", paths)
	}

	main, err := Main(repo)
	if err != nil {
		t.Fatal(err)
	}
	// Compare basenames: git reports realpaths, which may differ from the temp
	// path via symlinks (e.g. /private on macOS).
	if filepath.Base(main) != "app" {
		t.Errorf("Main basename = %q, want %q", filepath.Base(main), "app")
	}
	if got := SlugOf(main, filepath.Join(parent, "app-feat1")); got != "feat1" {
		t.Errorf("SlugOf(add worktree) = %q, want %q", got, "feat1")
	}
}
