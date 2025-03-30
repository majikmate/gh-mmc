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
	Classroom    classroom
	Students     []student
}

var (
	ErrClassroomNotFound = errors.New("no classroom found: run `gh mmc init` to create a classroom or change to a classroom folder")
)

func IsClassroomFolder() (bool, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("failed to get current directory: %v", err)
	}

	p := filepath.Join(currentDir, mmcFolder, classroomFile)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false, nil
	}

	return true, nil
}

func NewClassroom() *mmc {
	return &mmc{}
}

func LoadClassroom() (*mmc, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %v", err)
	}

	var p string
	for {
		p = filepath.Join(currentDir, mmcFolder, classroomFile)
		if _, err := os.Stat(p); err == nil {
			break
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return nil, ErrClassroomNotFound
		}

		currentDir = parentDir
	}

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

	return c, nil
}

func (c *mmc) SetOrganization(id int, login string) {
	c.Organization = org{
		Id:    id,
		Login: login,
	}
}

func (c *mmc) SetClassroom(id int, name string) {
	c.Classroom = classroom{
		Name: name,
		Id:   id,
	}
}

func (c *mmc) AddStudent(name, email, githubUser string) {
	c.Students = append(c.Students, student{
		Name:       name,
		Email:      email,
		GithubUser: githubUser,
	})
}

func (c *mmc) GetRepoName(githubUser string) (string, error) {
	for _, s := range c.Students {
		if s.GithubUser == githubUser {
			return s.RepoName(), nil
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
