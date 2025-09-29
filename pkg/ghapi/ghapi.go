package ghapi

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
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

type GitHubClassroom struct {
	Id           int                `json:"id"`
	Name         string             `json:"name"`
	Archived     bool               `json:"archived"`
	Url          string             `json:"url"`
	Organization GitHubOrganization `json:"organization"`
}

func GetOrganization(client *api.RESTClient, orgName string) (GitHubOrganization, error) {
	var response GitHubOrganization

	err := client.Get(fmt.Sprintf("orgs/%s", orgName), &response)
	if err != nil {
		return GitHubOrganization{}, err
	}

	return response, nil
}

func GetClassroom(client *api.RESTClient, classroomID int) (GitHubClassroom, error) {
	var response GitHubClassroom

	err := client.Get(fmt.Sprintf("classrooms/%v", classroomID), &response)
	if err != nil {
		return GitHubClassroom{}, err
	}

	return response, nil
}

func ListClassrooms(client *api.RESTClient, page int, perPage int) ([]GitHubClassroom, error) {
	var response []GitHubClassroom

	err := client.Get(fmt.Sprintf("classrooms?page=%v&per_page=%v", page, perPage), &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func PromptForClassroom(client *api.RESTClient) (classroomId GitHubClassroom, err error) {
	classrooms, err := ListClassrooms(client, 1, 100)
	if err != nil {
		return GitHubClassroom{}, err
	}

	if len(classrooms) == 0 {
		return GitHubClassroom{}, errors.New("no classrooms found")
	}

	optionMap := make(map[string]GitHubClassroom)
	options := make([]string, 0, len(classrooms))

	for _, classroom := range classrooms {
		optionMap[classroom.Name] = classroom
		options = append(options, classroom.Name)
	}

	var qs = []*survey.Question{
		{
			Name: "classroom",
			Prompt: &survey.Select{
				Message: "Select a classroom:",
				Options: options,
			},
		},
	}

	answer := struct {
		Classroom string
	}{}

	err = survey.Ask(qs, &answer)

	if err != nil {
		return GitHubClassroom{}, err
	}

	return optionMap[answer.Classroom], nil
}

func ListOrganizations(client *api.RESTClient, page int, perPage int) ([]GitHubOrganization, error) {
	var response []GitHubOrganization

	err := client.Get(fmt.Sprintf("user/orgs?page=%v&per_page=%v", page, perPage), &response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func PromptForOrganization(client *api.RESTClient) (GitHubOrganization, error) {
	organizations, err := ListOrganizations(client, 1, 100)
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

func PromptForCodespaceSelection(codespaces []GitHubCodespace) ([]GitHubCodespace, error) {
	// Filter out running codespaces - only show non-running ones
	nonRunningCodespaces := make([]GitHubCodespace, 0)
	for _, cs := range codespaces {
		if cs.State != "Available" {
			nonRunningCodespaces = append(nonRunningCodespaces, cs)
		}
	}

	if len(nonRunningCodespaces) == 0 {
		return nil, errors.New("no non-running codespaces available")
	}

	optionMap := make(map[string]GitHubCodespace)
	options := make([]string, 0, len(nonRunningCodespaces))

	// Calculate column widths for table alignment
	maxNameWidth := len("NAME")       // Start with header width
	maxRepoWidth := len("REPOSITORY") // Start with header width
	for _, cs := range nonRunningCodespaces {
		if len(cs.DisplayName) > maxNameWidth {
			maxNameWidth = len(cs.DisplayName)
		}
		if len(cs.Repository.FullName) > maxRepoWidth {
			maxRepoWidth = len(cs.Repository.FullName)
		}
	}

	for _, cs := range nonRunningCodespaces {
		// Format last used time
		lastUsed := "Never"
		if cs.LastUsedAt != nil && *cs.LastUsedAt != "" {
			if t, err := time.Parse(time.RFC3339, *cs.LastUsedAt); err == nil {
				lastUsed = t.Format("Mon 2006-01-02 15:04")
			}
		}

		// Create table-formatted display string (no state indicator)
		displayName := fmt.Sprintf("%-*s  %-*s  %s",
			maxNameWidth, cs.DisplayName,
			maxRepoWidth, cs.Repository.FullName,
			lastUsed)

		optionMap[displayName] = cs
		options = append(options, displayName)
	}

	// Prepare table headers as display-only options
	tableHeader := fmt.Sprintf("       %-*s  %-*s  %s",
		maxNameWidth, "NAME",
		maxRepoWidth, "REPOSITORY",
		"LAST USED")
	tableSeparator := fmt.Sprintf("       %s",
		strings.Repeat("-", maxNameWidth+2+maxRepoWidth+2+len("LAST USED")))

	// Add headers and separator before the actual options
	allOptions := make([]string, 0, len(options)+3)
	allOptions = append(allOptions, "") // Empty line after survey instructions
	allOptions = append(allOptions, tableHeader)
	allOptions = append(allOptions, tableSeparator)
	allOptions = append(allOptions, options...)

	var qs = []*survey.Question{
		{
			Name: "codespaces",
			Prompt: &survey.MultiSelect{
				Message: "Select non-running codespaces to delete:\n\nUse space to select, enter to confirm, Ctrl+C to cancel",
				Options: allOptions,
				VimMode: false, // Disable vim mode so ESC doesn't toggle it
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
		// Skip header/separator options
		if strings.HasPrefix(selectedOption, "Name") || strings.HasPrefix(selectedOption, "────") || selectedOption == "" {
			continue
		}
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
	DefaultBranch string `json:"default_branch"`
}

type GitHubAssignment struct {
	Id                          int              `json:"id"`
	PublicRepo                  bool             `json:"public_repo"`
	Title                       string           `json:"title"`
	AssignmentType              string           `json:"type"`
	InviteLink                  string           `json:"invite_link"`
	InvitationsEnabled          bool             `json:"invitations_enabled"`
	Slug                        string           `json:"slug"`
	StudentsAreRepoAdmins       bool             `json:"students_are_repo_admins"`
	FeedbackPullRequestsEnabled bool             `json:"feedback_pull_requests_enabled"`
	MaxTeams                    int              `json:"max_teams"`
	MaxMembers                  int              `json:"max_members"`
	Editor                      string           `json:"editor"`
	Accepted                    int              `json:"accepted"`
	Submissions                 int              `json:"submissions"`
	Passing                     int              `json:"passing"`
	Language                    string           `json:"language"`
	Deadline                    string           `json:"deadline"`
	GitHubClassroom             GitHubClassroom  `json:"classroom"`
	StarterCodeRepository       GithubRepository `json:"starter_code_repository"`
}

type GitHubAssignmentList struct {
	Assignments     []GitHubAssignment
	GitHubClassroom GitHubClassroom
	Count           int
}

func GetAssignment(client *api.RESTClient, assignmentID int) (GitHubAssignment, error) {
	var response GitHubAssignment
	err := client.Get(fmt.Sprintf("assignments/%v", assignmentID), &response)
	if err != nil {
		return GitHubAssignment{}, err
	}
	return response, nil
}

func ListAssignments(client *api.RESTClient, classroomID int, page int, perPage int) (GitHubAssignmentList, error) {
	var response []GitHubAssignment
	err := client.Get(fmt.Sprintf("classrooms/%v/assignments?page=%v&per_page=%v", classroomID, page, perPage), &response)
	if err != nil {
		return GitHubAssignmentList{}, err
	}

	if len(response) == 0 {
		return GitHubAssignmentList{}, nil
	}

	assignmentList := NewAssignmentList(response)

	return assignmentList, nil
}

func PromptForAssignment(client *api.RESTClient, classroomId int) (assignment GitHubAssignment, err error) {
	assignmentList, err := ListAssignments(client, classroomId, 1, 100)
	if err != nil {
		return GitHubAssignment{}, err
	}

	optionMap := make(map[string]GitHubAssignment)
	options := make([]string, 0, len(assignmentList.Assignments))

	for _, assignment := range assignmentList.Assignments {
		optionMap[assignment.Title] = assignment
		options = append(options, assignment.Title)
	}

	if len(options) == 0 {
		return GitHubAssignment{}, errors.New("no assignments found for this classroom")
	}

	var qs = []*survey.Question{
		{
			Name: "assignment",
			Prompt: &survey.Select{
				Message: "Select an assignment:",
				Options: options,
			},
		},
	}

	answer := struct {
		Assignment string
	}{}

	err = survey.Ask(qs, &answer)

	if err != nil {
		return GitHubAssignment{}, err
	}

	return optionMap[answer.Assignment], nil
}

func NewAssignmentList(assignments []GitHubAssignment) GitHubAssignmentList {
	if len(assignments) == 0 {
		return GitHubAssignmentList{
			Assignments:     []GitHubAssignment{},
			GitHubClassroom: GitHubClassroom{},
			Count:           0,
		}
	}

	classroom := assignments[0].GitHubClassroom
	count := len(assignments)

	return GitHubAssignmentList{
		Assignments:     assignments,
		GitHubClassroom: classroom,
		Count:           count,
	}
}

type GitHubStudent struct {
	Id        int    `json:"id"`
	Login     string `json:"login"`
	AvatarUrl string `json:"avatar_url"`
	HtmlUrl   string `json:"html_url"`
}

type GitHubAcceptedAssignment struct {
	Id                     int              `json:"id"`
	Submitted              bool             `json:"submitted"`
	Passing                bool             `json:"passing"`
	CommitCount            int              `json:"commit_count"`
	Grade                  string           `json:"grade"`
	FeedbackPullRequestUrl string           `json:"feedback_pull_request_url"`
	Students               []GitHubStudent  `json:"students"`
	Repository             GithubRepository `json:"repository"`
	Assignment             GitHubAssignment `json:"assignment"`
}

type GitHubAcceptedAssignmentList struct {
	AcceptedAssignments []GitHubAcceptedAssignment
	GitHubClassroom     GitHubClassroom
	Assignment          GitHubAssignment
	Count               int
}

type assignmentList struct {
	assignments []GitHubAcceptedAssignment
	Error       error
}

func NewAcceptedAssignmentList(assignments []GitHubAcceptedAssignment) GitHubAcceptedAssignmentList {
	if len(assignments) == 0 {
		return GitHubAcceptedAssignmentList{
			AcceptedAssignments: []GitHubAcceptedAssignment{},
			GitHubClassroom:     GitHubClassroom{},
			Assignment:          GitHubAssignment{},
			Count:               0,
		}
	}

	classroom := assignments[0].Assignment.GitHubClassroom
	assignment := assignments[0].Assignment
	count := len(assignments)

	return GitHubAcceptedAssignmentList{
		AcceptedAssignments: assignments,
		GitHubClassroom:     classroom,
		Assignment:          assignment,
		Count:               count,
	}
}

func GetAssignmentList(client *api.RESTClient, assignmentID int, page int, perPage int) ([]GitHubAcceptedAssignment, error) {
	var response []GitHubAcceptedAssignment

	err := client.Get(fmt.Sprintf("assignments/%v/accepted_assignments?page=%v&per_page=%v", assignmentID, page, perPage), &response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func NumberOfAcceptedAssignmentsAndPages(client *api.RESTClient, assignmentID int, perPage int) (numPages, totalAccepted int) {
	assignment, err := GetAssignment(client, assignmentID)
	if err != nil {
		log.Fatal(err)
	}
	numPages = int(math.Ceil(float64(assignment.Accepted) / float64(perPage)))
	totalAccepted = assignment.Accepted
	return
}

func ListAllAcceptedAssignments(client *api.RESTClient, assignmentID int, perPage int) (GitHubAcceptedAssignmentList, error) {

	numPages, totalAccepted := NumberOfAcceptedAssignmentsAndPages(client, assignmentID, perPage)

	ch := make(chan assignmentList)
	var wg sync.WaitGroup
	for page := 1; page <= numPages; page++ {
		wg.Add(1)
		go func(pg int) {
			defer wg.Done()
			response, err := GetAssignmentList(client, assignmentID, pg, perPage)
			ch <- assignmentList{
				assignments: response,
				Error:       err,
			}
		}(page)
	}

	var mu sync.Mutex
	assignments := make([]GitHubAcceptedAssignment, 0, totalAccepted)
	var hadErr error = nil
	for page := 1; page <= numPages; page++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := <-ch
			if result.Error != nil {
				hadErr = result.Error
			} else {
				mu.Lock()
				assignments = append(assignments, result.assignments...)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	close(ch)

	if hadErr != nil {
		return GitHubAcceptedAssignmentList{}, hadErr
	}

	return NewAcceptedAssignmentList(assignments), nil
}

type GitHubCodespacesResponse struct {
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
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name"`
	OperatingSystem      string `json:"operating_system"`
	StorageInBytes       int64  `json:"storage_in_bytes"`
	MemoryInBytes        int64  `json:"memory_in_bytes"`
	CPUs                 int    `json:"cpus"`
	PrebuildAvailability string `json:"prebuild_availability"`
}

type GitHubCodespaceGitStatus struct {
	Ahead                 int    `json:"ahead"`
	Behind                int    `json:"behind"`
	HasUnpushedChanges    bool   `json:"has_unpushed_changes"`
	HasUncommittedChanges bool   `json:"has_uncommitted_changes"`
	Ref                   string `json:"ref"`
}

type GitHubCodespaceEnvironment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ContainerImageInfo struct {
	Registry string `json:"registry"`
	Image    string `json:"image"`
	Tag      string `json:"tag"`
}

type RegistryTagsResponse struct {
	Results []RegistryTag `json:"results"`
}

type RegistryTag struct {
	Name        string    `json:"name"`
	LastUpdated time.Time `json:"last_updated"`
}

// CodespaceVersionInfo holds version metadata extracted from a codespace container
type CodespaceVersionInfo struct {
	Version     string `json:"version"`
	RefName     string `json:"refName"`
	Revision    string `json:"revision"`
	Digest      string `json:"digest"`
	ImageID     string `json:"imageID"`
	DefaultInfo string `json:"defaultInfo"`
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

	var response GitHubCodespacesResponse

	// Use the organization codespaces endpoint
	endpoint := fmt.Sprintf("orgs/%s/codespaces", orgName)
	err = client.Get(endpoint, &response)
	if err != nil {
		// Check if it's a 404 error to provide more helpful information
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			return nil, fmt.Errorf("failed to fetch codespaces for org %s: organization exists but codespaces endpoint not available. This could mean:\n"+
				"1. GitHub Codespaces is not enabled for this organization\n"+
				"2. Your GitHub token doesn't have 'admin:org' scope\n"+
				"3. You don't have permission to manage codespaces in this organization\n\n"+
				"To fix permission issues, try refreshing your GitHub CLI authentication:\n"+
				"   gh auth refresh --scopes admin:org\n\n"+
				"Original error: %v", orgName, err)
		}
		return nil, fmt.Errorf("failed to fetch codespaces for org %s: %v", orgName, err)
	}

	return response.Codespaces, nil
}
