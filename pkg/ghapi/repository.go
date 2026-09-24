package ghapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

type GitHubUser struct {
	Id    int    `json:"id"`
	Login string `json:"login"`
}

type GitHubCollaborator struct {
	Login       string `json:"login"`
	Permissions struct {
		Push bool `json:"push"`
	} `json:"permissions"`
}

type GitHubInvitation struct {
	Invitee     GitHubUser `json:"invitee"`
	Permissions string     `json:"permissions"`
}

// getAllPages fetches all pages of a list endpoint
func getAllPages[T any](client *api.RESTClient, path string) ([]T, error) {
	var allItems []T
	perPage := 100

	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	for page := 1; ; page++ {
		var items []T
		err := client.Get(fmt.Sprintf("%s%spage=%v&per_page=%v", path, separator, page, perPage), &items)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, items...)

		// If we got fewer items than the page size, we've reached the end
		if len(items) < perPage {
			return allItems, nil
		}
	}
}

// GetCurrentUser returns the logged in user
func GetCurrentUser(client *api.RESTClient) (GitHubUser, error) {
	var response GitHubUser

	err := client.Get("user", &response)
	if err != nil {
		return GitHubUser{}, err
	}

	return response, nil
}

// ListAllOrganizationRepositories returns all repositories of an organization the current user has access to
func ListAllOrganizationRepositories(client *api.RESTClient, orgName string) ([]GithubRepository, error) {
	return getAllPages[GithubRepository](client, fmt.Sprintf("orgs/%s/repos?type=all", orgName))
}

// IsPrivate checks if the repository is private, i.e. neither public nor internal
func (r GithubRepository) IsPrivate() bool {
	if r.Visibility != "" {
		return r.Visibility == "private"
	}
	return r.Private
}

// SetRepositoryPrivate changes the visibility of the repository to private
func SetRepositoryPrivate(client *api.RESTClient, repository GithubRepository) (GithubRepository, error) {
	body, err := json.Marshal(map[string]any{
		"visibility": "private",
	})
	if err != nil {
		return GithubRepository{}, err
	}

	var response GithubRepository
	err = client.Patch(fmt.Sprintf("repos/%s", repository.FullName), bytes.NewReader(body), &response)
	if err != nil {
		return GithubRepository{}, err
	}

	return response, nil
}

// MergeUpstream synchronizes the branch of the forked repository with its upstream repository. It returns how the
// changes were merged, i.e. fast-forward or merge, or none if the branch was up to date already.
func MergeUpstream(client *api.RESTClient, repository GithubRepository, branch string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"branch": branch,
	})
	if err != nil {
		return "", err
	}

	var response struct {
		MergeType string `json:"merge_type"`
	}
	err = client.Post(fmt.Sprintf("repos/%s/merge-upstream", repository.FullName), bytes.NewReader(body), &response)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
			return "", errors.New("the branch has merge conflicts with the upstream repository, merge it manually")
		}
		return "", err
	}

	return response.MergeType, nil
}

// DeleteRepository deletes the repository, which requires the delete_repo scope. A deleted repository cannot be
// restored if it is part of a fork network.
func DeleteRepository(client *api.RESTClient, repository GithubRepository) error {
	return client.Delete(fmt.Sprintf("repos/%s", repository.FullName), nil)
}

// ListForks returns all forks of the repository
func ListForks(client *api.RESTClient, repository GithubRepository) ([]GithubRepository, error) {
	return getAllPages[GithubRepository](client, fmt.Sprintf("repos/%s/forks", repository.FullName))
}

// IsNotFound checks if the error is a Not Found response of the GitHub API
func IsNotFound(err error) bool {
	var httpErr *api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// RepositoryURL returns the URL of the repository with the full name, i.e. owner/name, on the default GitHub host
func RepositoryURL(fullName string) string {
	host, _ := auth.DefaultHost()
	return fmt.Sprintf("https://%s/%s", host, fullName)
}

// FullNameFromURL returns the full name, i.e. owner/name, of the repository with the URL
func FullNameFromURL(repositoryURL string) (string, error) {
	u, err := url.Parse(repositoryURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL %q: %v", repositoryURL, err)
	}

	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if u.Host == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid repository URL %q", repositoryURL)
	}

	return parts[0] + "/" + parts[1], nil
}

// GetRepository returns a repository by its full name, i.e. owner/name
func GetRepository(client *api.RESTClient, fullName string) (GithubRepository, error) {
	var response GithubRepository

	err := client.Get(fmt.Sprintf("repos/%s", fullName), &response)
	if err != nil {
		return GithubRepository{}, err
	}

	return response, nil
}

// ListUsersWithWriteAccess returns the logins of all users that were granted at least write access to the
// repository directly, either as collaborator or by a pending invitation. Access by organization ownership or
// by teams is not included.
func ListUsersWithWriteAccess(client *api.RESTClient, repository GithubRepository) ([]string, error) {
	collaborators, err := getAllPages[GitHubCollaborator](client, fmt.Sprintf("repos/%s/collaborators?affiliation=direct", repository.FullName))
	if err != nil {
		return nil, err
	}

	invitations, err := getAllPages[GitHubInvitation](client, fmt.Sprintf("repos/%s/invitations", repository.FullName))
	if err != nil {
		return nil, err
	}

	logins := make([]string, 0)
	for _, collaborator := range collaborators {
		if collaborator.Permissions.Push {
			logins = append(logins, collaborator.Login)
		}
	}
	for _, invitation := range invitations {
		switch invitation.Permissions {
		case "write", "maintain", "admin":
			logins = append(logins, invitation.Invitee.Login)
		}
	}

	return logins, nil
}

// AddCollaborator grants the user the permission (pull, triage, push, maintain or admin) to the repository
func AddCollaborator(client *api.RESTClient, repository GithubRepository, username string, permission string) error {
	body, err := json.Marshal(map[string]any{
		"permission": permission,
	})
	if err != nil {
		return err
	}

	return client.Put(fmt.Sprintf("repos/%s/collaborators/%s", repository.FullName, username), bytes.NewReader(body), nil)
}

// ListTemplateRepositories returns all template repositories in all organizations the current user is a member of.
// Organizations whose repositories cannot be listed are skipped with a warning.
func ListTemplateRepositories(client *api.RESTClient) ([]GithubRepository, error) {
	organizations, err := ListAllOrganizations(client)
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	repositories := make([][]GithubRepository, len(organizations))
	errs := make([]error, len(organizations))
	for i, org := range organizations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			repositories[i], errs[i] = ListAllOrganizationRepositories(client, org.Login)
		}()
	}
	wg.Wait()

	templates := make([]GithubRepository, 0)
	for i, orgRepositories := range repositories {
		if errs[i] != nil {
			fmt.Fprintf(os.Stderr, "Skipping organization %s: %v\n", organizations[i].Login, errs[i])
			continue
		}
		for _, repository := range orgRepositories {
			if repository.IsTemplate {
				templates = append(templates, repository)
			}
		}
	}

	sort.Slice(templates, func(i, j int) bool {
		return strings.ToLower(templates[i].FullName) < strings.ToLower(templates[j].FullName)
	})

	return templates, nil
}

func PromptForTemplateRepository(templates []GithubRepository) (GithubRepository, error) {
	if len(templates) == 0 {
		return GithubRepository{}, errors.New("no template repositories found")
	}

	optionMap := make(map[string]GithubRepository)
	options := make([]string, 0, len(templates))

	for _, template := range templates {
		optionMap[template.FullName] = template
		options = append(options, template.FullName)
	}

	var answer string
	err := survey.AskOne(&survey.Select{
		Message:  "Select a template repository (ESC or Ctrl+C to cancel):",
		Options:  options,
		PageSize: 20,
	}, &answer)
	if err != nil {
		if isCancelled(err) {
			return GithubRepository{}, errors.New("operation cancelled by user")
		}
		return GithubRepository{}, err
	}

	return optionMap[answer], nil
}

// PromptForName asks for a name, proposing defaultName, until validate accepts it
func PromptForName(message string, defaultName string, validate func(name string) error) (string, error) {
	var name string
	err := survey.AskOne(&survey.Input{
		Message: message,
		Default: defaultName,
	}, &name, survey.WithValidator(func(ans interface{}) error {
		return validate(strings.TrimSpace(ans.(string)))
	}))
	if err != nil {
		if isCancelled(err) {
			return "", errors.New("operation cancelled by user")
		}
		return "", err
	}

	return strings.TrimSpace(name), nil
}

// CreateRepositoryFromTemplate creates a private repository with the full name, i.e. owner/name, from a template
// repository
func CreateRepositoryFromTemplate(client *api.RESTClient, template GithubRepository, fullName string) (GithubRepository, error) {
	owner, name, _ := strings.Cut(fullName, "/")
	body, err := json.Marshal(map[string]any{
		"owner":   owner,
		"name":    name,
		"private": true,
	})
	if err != nil {
		return GithubRepository{}, err
	}

	var response GithubRepository
	err = client.Post(fmt.Sprintf("repos/%s/generate", template.FullName), bytes.NewReader(body), &response)
	if err != nil {
		return GithubRepository{}, err
	}

	return response, nil
}

// WaitForRepositoryContent waits until the repository contains at least one commit. Repositories
// created from a template are populated asynchronously and cannot be forked before.
func WaitForRepositoryContent(client *api.RESTClient, repository GithubRepository, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var commits []struct {
			Sha string `json:"sha"`
		}
		err := client.Get(fmt.Sprintf("repos/%s/commits?per_page=1", repository.FullName), &commits)
		if err == nil && len(commits) > 0 {
			return nil
		}

		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("repository %s has no content after %v: %v", repository.FullName, timeout, err)
			}
			return fmt.Errorf("repository %s has no content after %v", repository.FullName, timeout)
		}

		time.Sleep(time.Second)
	}
}

// CreateFork forks the repository into the organization using the given name
func CreateFork(client *api.RESTClient, repository GithubRepository, org string, name string) (GithubRepository, error) {
	body, err := json.Marshal(map[string]any{
		"organization": org,
		"name":         name,
	})
	if err != nil {
		return GithubRepository{}, err
	}

	var response GithubRepository
	err = client.Post(fmt.Sprintf("repos/%s/forks", repository.FullName), bytes.NewReader(body), &response)
	if err != nil {
		return GithubRepository{}, err
	}

	return response, nil
}

// isCancelled checks if a prompt was cancelled by the user (Ctrl+C, ESC, etc.)
func isCancelled(err error) bool {
	return err == terminal.InterruptErr ||
		err.Error() == "interrupt" ||
		err.Error() == "unexpected escape sequence from terminal" ||
		strings.Contains(err.Error(), "escape sequence")
}
