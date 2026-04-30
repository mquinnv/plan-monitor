package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProjectBasename(t *testing.T) {
	if got := projectBasename("/Users/michael/Projects/plan-monitor"); got != "plan-monitor" {
		t.Errorf("got %q", got)
	}
}

func TestGitInfoNoRepo(t *testing.T) {
	info := gitInfo(t.TempDir())
	if info.Branch != "" || info.Dirty {
		t.Errorf("non-repo: %+v", info)
	}
}

func TestGitInfoCleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	info := gitInfo(dir)
	if info.Branch != "main" {
		t.Errorf("branch = %q, want main", info.Branch)
	}
	if info.Dirty {
		t.Errorf("clean repo should not be dirty")
	}
}

func TestGitInfoDirtyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@test")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("hi"), 0o644)
	info := gitInfo(dir)
	if !info.Dirty {
		t.Errorf("dirty repo should be dirty")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
