package mmc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type student struct {
	Name       string
	Email      string
	GithubUser string
}

func (a *student) RepoName() string {
	name := strings.Split(a.Email, "@")[0]
	parts := strings.Split(name, ".")
	if len(parts) == 2 {
		return parts[1] + "." + parts[0]
	} else {
		return name
	}
}

// FolderName returns the name of the local folder of the student, i.e. lastname.firstname made from the part of
// the email before the @, which is firstname.lastname, or the GitHub user if there is no email
func (a *student) FolderName() string {
	if name := a.RepoName(); name != "" {
		return name
	}
	return a.GithubUser
}

type org struct {
	Id    int
	Login string
}

type classroom struct {
	Id   int
	Name string
}

type mmc struct {
	Organization org
	Classroom    classroom `json:",omitzero"`
	Students     []student
}

var (
	ErrClassroomNotFound = errors.New("no classroom found: run `gh mmc init` to create a classroom or change to a classroom folder")
)

func NewClassroom() *mmc {
	return &mmc{}
}

// FindClassroomFolder searches upwards from the current directory to find the classroom folder root
// Returns the absolute path to the classroom folder, or an error if not found
func FindClassroomFolder() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %v", err)
	}

	for {
		p := filepath.Join(currentDir, mmcFolder, classroomFile)
		if _, err := os.Stat(p); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", ErrClassroomNotFound
		}

		currentDir = parentDir
	}
}

func LoadClassroom() (*mmc, error) {
	classroomFolder, err := FindClassroomFolder()
	if err != nil {
		return nil, err
	}

	p := filepath.Join(classroomFolder, mmcFolder, classroomFile)
	file, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s file: %v", p, err)
	}
	defer file.Close() //nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s file: %v", p, err)
	}

	c := NewClassroom()
	err = json.Unmarshal(data, &c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s file: %v", p, err)
	}

	// The name of the folder containing the .mmc folder is the name of the classroom
	c.Classroom.Name = filepath.Base(classroomFolder)

	return c, nil
}

func (c *mmc) SetOrganization(id int, login string) {
	c.Organization = org{
		Id:    id,
		Login: login,
	}
}

func (c *mmc) AddStudent(name, email, githubUser string) {
	c.Students = append(c.Students, student{
		Name:       name,
		Email:      email,
		GithubUser: githubUser,
	})
}

// GetRepoName returns the name of the local folder of the student with the GitHub user, i.e. lastname.firstname
func (c *mmc) GetRepoName(githubUser string) (string, error) {
	for _, s := range c.Students {
		if strings.EqualFold(s.GithubUser, githubUser) {
			return s.FolderName(), nil
		}
	}
	return "", fmt.Errorf("GitHub user %s not found", githubUser)
}

func (c *mmc) Save(path string) error {
	var err error
	if path == "" {
		path, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %v", err)
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	f := filepath.Join(path, mmcFolder)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		err := os.Mkdir(f, 0755)
		if err != nil {
			return fmt.Errorf("failed to create %s directory: %v", f, err)
		}
	}

	j, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal classroom: %v", err)
	}

	p := filepath.Join(f, classroomFile)
	file, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("failed to create %s file: %v", p, err)
	}
	defer file.Close() //nolint:errcheck

	_, err = file.Write(j)
	if err != nil {
		return fmt.Errorf("failed to write %s file: %v", p, err)
	}

	return nil
}
