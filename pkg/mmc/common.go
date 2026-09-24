package mmc

import (
	"fmt"
	"os"
)

const (
	mmcFolder = ".mmc"

	classroomFile = "classroom.json"
	courseFile    = "course.json"
)

func Fatal(v ...any) {
	fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}

// ChangeToFolder changes the current directory to the folder and returns a function that changes back to the
// current directory
func ChangeToFolder(folder string) (restore func(), err error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %v", err)
	}

	err = os.Chdir(folder)
	if err != nil {
		return nil, fmt.Errorf("failed to change to directory %s: %v", folder, err)
	}

	return func() {
		_ = os.Chdir(currentDir)
	}, nil
}
