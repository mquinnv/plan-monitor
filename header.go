package main

import (
	"os/exec"
	"path/filepath"
	"strings"
)

type GitStatus struct {
	Branch string
	Dirty  bool
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
	return GitStatus{Branch: strings.TrimSpace(branch), Dirty: dirty}
}

func runGitCmd(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	return string(out), err
}
