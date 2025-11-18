package similarity

import (
"bufio"
"fmt"
"os"
"path/filepath"
"strings"
)

// AssignmentComparison stores the max similarity for a specific assignment
type AssignmentComparison struct {
	AssignmentName string
	MaxSimilarity  float64
	File1          string
	File2          string
}

// ComparisonResult stores similarity results across all assignments
type ComparisonResult struct {
	// student1 -> student2 -> assignment -> max similarity
	Results map[string]map[string]map[string]*AssignmentComparison
	// List of all assignments found
	Assignments []string
}

// CalculateSimilarity calculates the similarity percentage between two files
func CalculateSimilarity(file1, file2 string) (float64, error) {
	content1, err := readFile(file1)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %v", file1, err)
	}

	content2, err := readFile(file2)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %v", file2, err)
	}

	// Handle empty files
	if len(content1) == 0 && len(content2) == 0 {
		return 100.0, nil
	}
	if len(content1) == 0 || len(content2) == 0 {
		return 0.0, nil
	}

	return jaccardSimilarity(content1, content2), nil
}

// readFile reads a file and returns its lines without normalization
func readFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// jaccardSimilarity calculates Jaccard similarity coefficient
func jaccardSimilarity(lines1, lines2 []string) float64 {
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)

	for _, line := range lines1 {
		set1[line] = true
	}

	for _, line := range lines2 {
		set2[line] = true
	}

	intersection := 0
	for line := range set1 {
		if set2[line] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0.0
	}

	return (float64(intersection) / float64(union)) * 100.0
}

// FindStudentFolders finds all student folders in a classroom directory
func FindStudentFolders(classroomPath string, starterFolderPrefix string) ([]string, error) {
	entries, err := os.ReadDir(classroomPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read classroom directory: %v", err)
	}

	var studentFolders []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if name == starterFolderPrefix {
			continue
		}

		studentFolders = append(studentFolders, name)
	}

	return studentFolders, nil
}

// FindAssignments finds all assignment folders in a student's 20-assignments directory
func FindAssignments(studentPath string) ([]string, error) {
assignmentsPath := filepath.Join(studentPath, "20-assignments")

if _, err := os.Stat(assignmentsPath); os.IsNotExist(err) {
return nil, nil // No assignments folder
}

entries, err := os.ReadDir(assignmentsPath)
if err != nil {
return nil, fmt.Errorf("failed to read assignments directory: %v", err)
}

var assignments []string
for _, entry := range entries {
if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
assignments = append(assignments, entry.Name())
}
}

return assignments, nil
}

// FindFilesWithExtension finds all files with a specific extension in a directory
func FindFilesWithExtension(dirPath string, extension string) ([]string, error) {
var files []string

err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
if err != nil {
return err
}

if !info.IsDir() && strings.HasSuffix(info.Name(), extension) {
files = append(files, path)
}

return nil
})

if err != nil {
return nil, fmt.Errorf("failed to walk directory %s: %v", dirPath, err)
}

return files, nil
}

// CompareAssignments compares files across all students and all assignments
func CompareAssignments(classroomPath string, fileExtension string, starterFolder string) (*ComparisonResult, error) {
studentFolders, err := FindStudentFolders(classroomPath, starterFolder)
if err != nil {
return nil, err
}

if len(studentFolders) < 2 {
return nil, fmt.Errorf("need at least 2 student folders to compare")
}

result := &ComparisonResult{
Results:     make(map[string]map[string]map[string]*AssignmentComparison),
Assignments: []string{},
}

// Initialize results structure
for _, student := range studentFolders {
result.Results[student] = make(map[string]map[string]*AssignmentComparison)
for _, otherStudent := range studentFolders {
if student != otherStudent {
result.Results[student][otherStudent] = make(map[string]*AssignmentComparison)
}
}
}

// Get all unique assignments across all students
assignmentSet := make(map[string]bool)
for _, student := range studentFolders {
studentPath := filepath.Join(classroomPath, student)
assignments, err := FindAssignments(studentPath)
if err != nil {
fmt.Printf("Warning: failed to get assignments for %s: %v\n", student, err)
continue
}
for _, assignment := range assignments {
assignmentSet[assignment] = true
}
}

// Convert to sorted slice
for assignment := range assignmentSet {
result.Assignments = append(result.Assignments, assignment)
}

// For each assignment, compare all students
for _, assignment := range result.Assignments {
fmt.Printf("Analyzing assignment: %s\n", assignment)

// Compare each pair of students for this assignment
for i, student1 := range studentFolders {
student1AssignmentPath := filepath.Join(classroomPath, student1, "20-assignments", assignment)

// Check if this student has this assignment
if _, err := os.Stat(student1AssignmentPath); os.IsNotExist(err) {
continue
}

// Get all files for student1 in this assignment
files1, err := FindFilesWithExtension(student1AssignmentPath, fileExtension)
if err != nil {
fmt.Printf("Warning: failed to find files for %s/%s: %v\n", student1, assignment, err)
continue
}

if len(files1) == 0 {
continue
}

// Compare with all other students
for j := i + 1; j < len(studentFolders); j++ {
student2 := studentFolders[j]
student2AssignmentPath := filepath.Join(classroomPath, student2, "20-assignments", assignment)

// Check if student2 has this assignment
if _, err := os.Stat(student2AssignmentPath); os.IsNotExist(err) {
continue
}

// Get all files for student2 in this assignment
files2, err := FindFilesWithExtension(student2AssignmentPath, fileExtension)
if err != nil {
fmt.Printf("Warning: failed to find files for %s/%s: %v\n", student2, assignment, err)
continue
}

if len(files2) == 0 {
continue
}

// Find maximum similarity between any pair of files
maxSim := 0.0
var maxFile1, maxFile2 string

for _, file1 := range files1 {
for _, file2 := range files2 {
sim, err := CalculateSimilarity(file1, file2)
if err != nil {
fmt.Printf("Warning: failed to compare %s and %s: %v\n", file1, file2, err)
continue
}

if sim > maxSim {
maxSim = sim
maxFile1 = file1
maxFile2 = file2
}
}
}

// Store the result for both directions
result.Results[student1][student2][assignment] = &AssignmentComparison{
AssignmentName: assignment,
MaxSimilarity:  maxSim,
File1:          maxFile1,
File2:          maxFile2,
}

result.Results[student2][student1][assignment] = &AssignmentComparison{
AssignmentName: assignment,
MaxSimilarity:  maxSim,
File1:          maxFile2,
File2:          maxFile1,
}
}
}
}

return result, nil
}
