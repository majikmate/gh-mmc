package ghapi

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sync"

	"github.com/AlecAivazis/survey/v2"
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
