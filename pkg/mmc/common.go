package mmc

import (
	"fmt"
	"os"
)

const (
	mmcFolder = ".mmc"

	classroomFile = "classroom.json"
	assigmentFile = "assignment.json"
)

func Fatal(v ...any) {
	fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}
