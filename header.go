package main

import (
	"os/exec"
	"path/filepath"
	"strings"
)

type GitStatus struct {
	Branch     string
	Dirty      bool
	IsWorktree bool
}

func projectBasename(cwd string) string {
	return filepath.Base(cwd)
}

func gitInfo(cwd string) GitStatus {
	branch, err := runGitCmd(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return GitStatus{}
	}
	out, err := runGitCmd(cwd, "status", "--porcelain")
	dirty := err == nil && strings.TrimSpace(out) != ""
	return GitStatus{
		Branch:     strings.TrimSpace(branch),
		Dirty:      dirty,
		IsWorktree: isLinkedWorktree(cwd),
	}
}

// isLinkedWorktree reports whether cwd is inside a linked worktree (created
// via `git worktree add`), as opposed to the primary working tree. Detection:
// `git rev-parse --git-dir` points at .git/worktrees/<name> for linked
// worktrees, while `--git-common-dir` always points at the shared .git.
// They are identical for the primary worktree, distinct for linked ones.
func isLinkedWorktree(cwd string) bool {
	gitDir, err := runGitCmd(cwd, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	commonDir, err := runGitCmd(cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}
	return strings.TrimSpace(gitDir) != strings.TrimSpace(commonDir)
}

func runGitCmd(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	return string(out), err
}
