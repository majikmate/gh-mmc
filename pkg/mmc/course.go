package mmc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type course struct {
	// Name is the name of the folder containing the .mmc folder with the course file
	Name string `json:"-"`
	// TemplateRepository is the URL of the template repository the starter repository is created from
	TemplateRepository string
	// StarterRepository is the URL of the starter repository the student repositories are forked from
	StarterRepository string
}

var (
	ErrCourseNotFound = errors.New("no course found: run `gh mmc pull` in a classroom folder to create a course or change to a course folder")
)

// StarterRepositoryName returns the name of the starter repository of a course in a classroom
func StarterRepositoryName(classroom, course string) string {
	return classroom + "-" + course
}

// StudentRepositoryName returns the name of the repository of a student for a course in a classroom
func StudentRepositoryName(classroom, course, githubUser string) string {
	return StarterRepositoryName(classroom, course) + "-" + githubUser
}

func NewCourse(templateRepository string, starterRepository string) *course {
	return &course{
		TemplateRepository: templateRepository,
		StarterRepository:  starterRepository,
	}
}

// FindCourseFolder searches upwards from the current directory to find the course folder root
// Returns the absolute path to the course folder, or an error if not found
func FindCourseFolder() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %v", err)
	}

	for {
		p := filepath.Join(currentDir, mmcFolder, courseFile)
		if _, err := os.Stat(p); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", ErrCourseNotFound
		}

		currentDir = parentDir
	}
}

func LoadCourse() (*course, error) {
	courseFolder, err := FindCourseFolder()
	if err != nil {
		return nil, err
	}

	return loadCourse(courseFolder)
}

// ListCourses returns the courses of the classroom folder, i.e. its folders containing a .mmc folder with the
// course file
func ListCourses(classroomFolder string) ([]*course, error) {
	entries, err := os.ReadDir(classroomFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to read classroom folder %s: %v", classroomFolder, err)
	}

	courses := []*course{}
	for _, entry := range entries {
		folder := filepath.Join(classroomFolder, entry.Name())
		if _, err := os.Stat(filepath.Join(folder, mmcFolder, courseFile)); !entry.IsDir() || err != nil {
			continue
		}
		c, err := loadCourse(folder)
		if err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}

	return courses, nil
}

// loadCourse loads the course of the course folder
func loadCourse(courseFolder string) (*course, error) {
	p := filepath.Join(courseFolder, mmcFolder, courseFile)
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s file: %v", p, err)
	}
	defer f.Close() //nolint:errcheck

	j, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s file: %v", p, err)
	}

	c := NewCourse("", "")
	err = json.Unmarshal(j, &c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s file: %v", p, err)
	}

	// The name of the folder containing the .mmc folder is the name of the course
	c.Name = filepath.Base(courseFolder)

	return c, nil
}

func (c *course) Save(path string) error {
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
		return fmt.Errorf("failed to marshal course: %v", err)
	}

	p := filepath.Join(f, courseFile)
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
