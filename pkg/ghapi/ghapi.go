package ghapi

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/go-gh/v2/pkg/api"
)

type GitHubOrganization struct {
	Id        int    `json:"id"`
	Login     string `json:"login"`
	NodeID    string `json:"node_id"`
	HtmlUrl   string `json:"html_url"`
	Name      string `json:"name"`
	AvatarUrl string `json:"avatar_url"`
}

func GetOrganization(client *api.RESTClient, orgName string) (GitHubOrganization, error) {
	var response GitHubOrganization

	err := client.Get(fmt.Sprintf("orgs/%s", orgName), &response)
	if err != nil {
		return GitHubOrganization{}, err
	}

	return response, nil
}

func ListOrganizations(client *api.RESTClient, page int, perPage int) ([]GitHubOrganization, error) {
	var response []GitHubOrganization

	err := client.Get(fmt.Sprintf("user/orgs?page=%v&per_page=%v", page, perPage), &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// ListAllOrganizations returns all organizations the current user is a member of
func ListAllOrganizations(client *api.RESTClient) ([]GitHubOrganization, error) {
	var allOrganizations []GitHubOrganization
	page := 1
	perPage := 100

	for {
		organizations, err := ListOrganizations(client, page, perPage)
		if err != nil {
			return nil, err
		}

		allOrganizations = append(allOrganizations, organizations...)

		// If we got fewer organizations than the page size, we've reached the end
		if len(organizations) < perPage {
			break
		}

		page++
	}

	return allOrganizations, nil
}

func PromptForOrganization(client *api.RESTClient) (GitHubOrganization, error) {
	organizations, err := ListAllOrganizations(client)
	if err != nil {
		return GitHubOrganization{}, err
	}

	if len(organizations) == 0 {
		return GitHubOrganization{}, errors.New("no organizations found")
	}

	optionMap := make(map[string]GitHubOrganization)
	options := make([]string, 0, len(organizations))

	for _, org := range organizations {
		displayName := org.Login
		if org.Name != "" {
			displayName = fmt.Sprintf("%s (%s)", org.Name, org.Login)
		}
		optionMap[displayName] = org
		options = append(options, displayName)
	}

	var qs = []*survey.Question{
		{
			Name: "organization",
			Prompt: &survey.Select{
				Message: "Select an organization (ESC or Ctrl+C to cancel):",
				Options: options,
			},
		},
	}

	answer := struct {
		Organization string
	}{}

	err = survey.Ask(qs, &answer)
	if err != nil {
		// Handle user cancellation (Ctrl+C, ESC, etc.)
		if err == terminal.InterruptErr ||
			err.Error() == "interrupt" ||
			err.Error() == "unexpected escape sequence from terminal" ||
			strings.Contains(err.Error(), "escape sequence") {
			return GitHubOrganization{}, errors.New("operation cancelled by user")
		}
		return GitHubOrganization{}, err
	}

	return optionMap[answer.Organization], nil
}

// GetStateIndicator returns a colored emoji indicator for the codespace state
func GetStateIndicator(state string) string {
	switch state {
	case "Available":
		return "●"
	case "Shutdown":
		return "○"
	default:
		return "◐"
	}
}

func PromptForCodespaceSelection(codespaces []GitHubCodespace, orgName string, getUserDisplayName func(string) string) ([]GitHubCodespace, error) {
	// Filter out running codespaces and those with uncommitted/unpushed changes
	cleanNonRunningCodespaces := make([]GitHubCodespace, 0)
	for _, cs := range codespaces {
		if cs.State != "Available" && !cs.GitStatus.HasUncommittedChanges && !cs.GitStatus.HasUnpushedChanges {
			cleanNonRunningCodespaces = append(cleanNonRunningCodespaces, cs)
		}
	}

	if len(cleanNonRunningCodespaces) == 0 {
		return nil, errors.New("no clean non-running codespaces available")
	}

	optionMap := make(map[string]GitHubCodespace)
	options := make([]string, 0, len(cleanNonRunningCodespaces))

	// Calculate column widths for table alignment
	maxNameWidth := len("NAME")       // Start with header width
	maxRepoWidth := len("REPOSITORY") // Start with header width
	maxUserWidth := len("USER")       // Start with header width
	for _, cs := range cleanNonRunningCodespaces {
		if len(cs.DisplayName) > maxNameWidth {
			maxNameWidth = len(cs.DisplayName)
		}

		// Strip organization prefix for width calculation
		repoDisplayName := cs.Repository.FullName
		if orgPrefix := orgName + "/"; strings.HasPrefix(repoDisplayName, orgPrefix) {
			repoDisplayName = repoDisplayName[len(orgPrefix):]
		}
		if len(repoDisplayName) > maxRepoWidth {
			maxRepoWidth = len(repoDisplayName)
		}

		// Calculate user display name width
		userDisplayName := getUserDisplayName(cs.Owner.Login)
		if len(userDisplayName) > maxUserWidth {
			maxUserWidth = len(userDisplayName)
		}
	}

	for _, cs := range cleanNonRunningCodespaces {
		// Format last used time
		lastUsed := "Never"
		if cs.LastUsedAt != nil && *cs.LastUsedAt != "" {
			if t, err := time.Parse(time.RFC3339, *cs.LastUsedAt); err == nil {
				lastUsed = t.Format("Mon 2006-01-02 15:04")
			}
		}

		// Format idle timeout
		idleTimeout := fmt.Sprintf("%dm", cs.IdleTimeoutMinutes)

		// Strip organization prefix from repository name
		repoDisplayName := cs.Repository.FullName
		if orgPrefix := orgName + "/"; strings.HasPrefix(repoDisplayName, orgPrefix) {
			repoDisplayName = repoDisplayName[len(orgPrefix):]
		}

		// Get user display name
		userDisplayName := getUserDisplayName(cs.Owner.Login)

		// Create table-formatted display string with user information
		displayName := fmt.Sprintf("%-*s  %-*s  %-*s  %-s  %s",
			maxNameWidth, cs.DisplayName,
			maxRepoWidth, repoDisplayName,
			maxUserWidth, userDisplayName,
			idleTimeout,
			lastUsed)

		optionMap[displayName] = cs
		options = append(options, displayName)
	}

	var qs = []*survey.Question{
		{
			Name: "codespaces",
			Prompt: &survey.MultiSelect{
				Message:  "Select clean non-running codespaces to delete:",
				Options:  options,
				PageSize: 20,                                                                                      // Show at least 20 items before scrolling
				VimMode:  false,                                                                                   // Disable vim mode so ESC doesn't toggle it
				Help:     "[Use arrows to move, space to select, <right> to all, <left> to none, type to filter]", // Remove default help to prevent duplication
			},
		},
	}

	answer := struct {
		Codespaces []string
	}{}

	err := survey.Ask(qs, &answer)
	if err != nil {
		// Handle user cancellation (Ctrl+C, ESC, etc.)
		if err == terminal.InterruptErr ||
			err.Error() == "interrupt" ||
			err.Error() == "unexpected escape sequence from terminal" ||
			strings.Contains(err.Error(), "escape sequence") {
			return nil, errors.New("operation cancelled by user")
		}
		return nil, err
	}

	if len(answer.Codespaces) == 0 {
		return nil, errors.New("no codespaces selected")
	}

	selectedCodespaces := make([]GitHubCodespace, 0, len(answer.Codespaces))
	for _, selectedOption := range answer.Codespaces {
		if cs, exists := optionMap[selectedOption]; exists {
			selectedCodespaces = append(selectedCodespaces, cs)
		}
	}

	return selectedCodespaces, nil
}

type GithubRepository struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HtmlUrl       string `json:"html_url"`
	NodeId        string `json:"node_id"`
	Private       bool   `json:"private"`
	Visibility    string `json:"visibility"`
	DefaultBranch string `json:"default_branch"`
	IsTemplate    bool   `json:"is_template"`
	// TemplateRepository is only returned when getting a single repository
	TemplateRepository *GithubRepository `json:"template_repository"`
}

type GitHubCodespacesResponse struct {
	TotalCount int               `json:"total_count"`
	Codespaces []GitHubCodespace `json:"codespaces"`
}

type GitHubCodespace struct {
	ID                     int                       `json:"id"`
	Name                   string                    `json:"name"`
	DisplayName            string                    `json:"display_name"`
	State                  string                    `json:"state"`
	Repository             GitHubCodespaceRepository `json:"repository"`
	Owner                  GitHubCodespaceUser       `json:"owner"`
	BillableOwner          GitHubCodespaceUser       `json:"billable_owner"`
	Machine                GitHubCodespaceMachine    `json:"machine"`
	Prebuild               bool                      `json:"prebuild"`
	CreatedAt              string                    `json:"created_at"`
	UpdatedAt              string                    `json:"updated_at"`
	LastUsedAt             *string                   `json:"last_used_at"`
	EnvironmentID          string                    `json:"environment_id"`
	DevcontainerPath       string                    `json:"devcontainer_path"`
	GitStatus              GitHubCodespaceGitStatus  `json:"git_status"`
	IdleTimeoutMinutes     int                       `json:"idle_timeout_minutes"`
	Location               string                    `json:"location"`
	WebURL                 string                    `json:"web_url"`
	URL                    string                    `json:"url"`
	RetentionExpiresAt     *string                   `json:"retention_expires_at"`
	RetentionPeriodMinutes int                       `json:"retention_period_minutes"`
	PendingOperation       bool                      `json:"pending_operation"`
}

type GitHubCodespaceRepository struct {
	ID          int                 `json:"id"`
	Name        string              `json:"name"`
	FullName    string              `json:"full_name"`
	Owner       GitHubCodespaceUser `json:"owner"`
	Private     bool                `json:"private"`
	Description *string             `json:"description"`
	HTMLURL     string              `json:"html_url"`
	URL         string              `json:"url"`
}

type GitHubCodespaceUser struct {
	Login     string `json:"login"`
	ID        int    `json:"id"`
	Type      string `json:"type"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
	SiteAdmin bool   `json:"site_admin"`
}

type GitHubCodespaceMachine struct {
	Name                 string  `json:"name"`
	DisplayName          string  `json:"display_name"`
	OperatingSystem      string  `json:"operating_system"`
	StorageInBytes       int64   `json:"storage_in_bytes"`
	MemoryInBytes        int64   `json:"memory_in_bytes"`
	CPUs                 int     `json:"cpus"`
	PrebuildAvailability *string `json:"prebuild_availability"`
}

type GitHubCodespaceGitStatus struct {
	Ahead                 int    `json:"ahead"`
	Behind                int    `json:"behind"`
	HasUnpushedChanges    bool   `json:"has_unpushed_changes"`
	HasUncommittedChanges bool   `json:"has_uncommitted_changes"`
	Ref                   string `json:"ref"`
}

func GetCodespacesForOrg(client *api.RESTClient, orgName string) ([]GitHubCodespace, error) {
	// First, verify the organization exists
	_, err := GetOrganization(client, orgName)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			return nil, fmt.Errorf("organization '%s' not found. Please check the organization name is correct and you have access to it", orgName)
		}
		return nil, fmt.Errorf("failed to verify organization %s: %v", orgName, err)
	}

	var allCodespaces []GitHubCodespace
	page := 1
	perPage := 100 // Request 100, but GitHub may limit to 50

	for {
		var response GitHubCodespacesResponse

		// Use the organization codespaces endpoint with pagination
		endpoint := fmt.Sprintf("orgs/%s/codespaces?page=%d&per_page=%d", orgName, page, perPage)
		err = client.Get(endpoint, &response)
		if err != nil {
			// Check if it's a 404 error to provide more helpful information
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				return nil, fmt.Errorf("failed to fetch codespaces for org %s: organization exists but codespaces endpoint not available. This could mean:\n"+
					"1. GitHub Codespaces is not enabled for this organization\n"+
					"2. Your GitHub token doesn't have the 'admin:org' scope or permission to manage codespaces\n\n"+
					"Original error: %v", orgName, err)
			}
			return nil, fmt.Errorf("failed to fetch codespaces for org %s: %v", orgName, err)
		}

		// Add the codespaces from this page to our collection
		allCodespaces = append(allCodespaces, response.Codespaces...)

		// Continue fetching if:
		// 1. We got exactly the maximum number of items (likely means there are more)
		// 2. Or we have a total_count and haven't reached it yet
		shouldContinue := false
		if len(response.Codespaces) >= 50 { // GitHub appears to limit to 50 per page
			shouldContinue = true
		}
		if response.TotalCount > 0 && len(allCodespaces) < response.TotalCount {
			shouldContinue = true
		}

		if !shouldContinue {
			break
		}

		page++
	}

	return allCodespaces, nil
}
