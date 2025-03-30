// cmd/gh-classroom/clone/utils/clone-repository.go

package utils

import (
	"bytes"
	"fmt"
	"os"
)

// This abstraction allows for easier testing and decoupling from the actual CLI.
// Exec invokes a gh command in a subprocess and captures the output and error streams.
type GitHubExec func(args ...string) (stdout, stderr bytes.Buffer, err error)

// CloneRepository attempts to clone a repository into the specified path.
// It returns an error if the cloning process fails.
func CloneRepository(clonePath string, repoFullName string, ghExec GitHubExec) error {
	if _, err := os.Stat(clonePath); os.IsNotExist(err) {
		fmt.Printf("Cloning into: %s (%s)\n", clonePath, repoFullName)
		_, _, err := ghExec("repo", "clone", repoFullName, "--", clonePath)
		if err != nil {
			return fmt.Errorf("error cloning %s: %v", repoFullName, err)
		}

		return nil // Success
	}

	fmt.Printf("Skip existing repo: %s, use gh mmc pull to get new commits\n", clonePath)
	return fmt.Errorf("repository already exists: %s", clonePath)
}
