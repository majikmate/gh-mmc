package mmc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsCloneOf checks if the folder is a git repository whose origin is the repository with the full name, i.e.
// owner/name
func IsCloneOf(folder string, fullName string) bool {
	if _, err := os.Stat(filepath.Join(folder, ".git")); err != nil {
		return false
	}

	out, err := exec.Command("git", "-C", folder, "remote", "get-url", "origin").Output()
	if err != nil {
		return false
	}

	url := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(string(out)), ".git"))
	fullName = strings.ToLower(fullName)
	return strings.HasSuffix(url, "/"+fullName) || strings.HasSuffix(url, ":"+fullName)
}

// HasLocalChanges checks if the git repository in the folder has uncommitted changes or commits that are not pushed
func HasLocalChanges(folder string) bool {
	out, err := exec.Command("git", "-C", folder, "status", "--porcelain", "--branch").Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			if strings.Contains(line, "[ahead ") {
				return true
			}
		} else if line != "" {
			return true
		}
	}

	return false
}
