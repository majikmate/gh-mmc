package check

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/majikmate/gh-mmc/pkg/similarity"
	"github.com/spf13/cobra"
)

const (
	orderByAssignment = "assignment"
	orderByStudent    = "student"
)

func NewCmdCheck(f *cmdutil.Factory) *cobra.Command {
	var aId int
	var fileExtensions []string
	var threshold float64
	var starterFolder string
	var ignoreFiles []string
	var showDiff bool
	var verbose bool
	var orderBy string
	var filterStudent string
	var filterAssignment string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check similarity of student submissions for plagiarism detection",
		Long: heredoc.Doc(`
			Analyzes student submissions across all assignments to detect potential plagiarism
			by comparing files of a specific type within the same assignment folder.

			This command will:
			- Scan all student folders for assignments (20-assignments/*)
			- Compare files within each assignment separately
			- Find maximum similarity for each file pair
			- Display results per assignment showing potential plagiarism
			- Highlight similarities above the threshold

			Files are normalized before comparison by:
			- Removing empty lines and basic comments
			- Normalizing whitespace
			- Using line-based Jaccard similarity detection
			
			The similarity percentage ranges from 0% (completely different) to 100% 
			(identical content).`),
		Example: heredoc.Doc(`
			# Check HTML files across all assignments
			$ gh mmc check --extension .html

			# Check CSS files with custom 80% threshold
			$ gh mmc check -e .css -t 80

			# Check JavaScript files
			$ gh mmc check -e .js -t 75`),
		Run: func(cmd *cobra.Command, args []string) {
			startingDir, err := os.Getwd()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get current directory: %v", err))
			}
			defer func() {
				_ = os.Chdir(startingDir)
			}()

			c, err := mmc.LoadClassroom()
			if err != nil {
				mmc.Fatal(err)
			}

			// Try to find assignment folder first (module-html-css level with students)
			// If not found, try classroom folder
			var searchPath string
			assignmentFolder, err := mmc.FindAssignmentFolder()
			if err == nil {
				// We're in or below an assignment folder - use it as the search path
				searchPath = assignmentFolder
				err = os.Chdir(assignmentFolder)
				if err != nil {
					mmc.Fatal(fmt.Errorf("failed to change to assignment directory: %v", err))
				}
			} else {
				// Not in assignment, try classroom folder
				classroomFolder, err := mmc.FindClassroomFolder()
				if err != nil {
					mmc.Fatal("No classroom or assignment found. Run `gh mmc init` to initialize a classroom folder or change to a classroom/assignment folder.")
				}
				searchPath = classroomFolder
				err = os.Chdir(classroomFolder)
				if err != nil {
					mmc.Fatal(fmt.Errorf("failed to change to classroom directory: %v", err))
				}
			}

			// Determine starter folder name from classroom
			if starterFolder == "" {
				starterFolder = c.Classroom.Name
			}

			// Ensure file extensions start with a dot
			for i, ext := range fileExtensions {
				if ext != "" && !strings.HasPrefix(ext, ".") {
					fileExtensions[i] = "." + ext
				}
			}

			if verbose {
				fmt.Printf("Checking classroom: %s\n", c.Classroom.Name)
				fmt.Printf("Search path: %s\n", searchPath)
				fmt.Printf("File extensions: %v\n", fileExtensions)
				fmt.Printf("Threshold: %.0f%%\n", threshold)
				if len(ignoreFiles) > 0 {
					fmt.Printf("Ignoring files: %v\n", ignoreFiles)
				}
				fmt.Println()
			}

			// Run the comparison
			result, err := similarity.CompareAssignments(searchPath, fileExtensions, starterFolder, ignoreFiles, verbose)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to compare assignments: %v", err))
			}

			// Get sorted list of students
			students := make([]string, 0, len(result.Results))
			for student := range result.Results {
				students = append(students, student)
			}
			sort.Strings(students)

			if len(students) == 0 {
				fmt.Println("No student submissions found.")
				return
			}

			// Sort assignments
			sort.Strings(result.Assignments)

			// Validate orderBy parameter
			if orderBy != orderByStudent && orderBy != orderByAssignment {
				mmc.Fatal(fmt.Errorf("invalid order-by value: %s. Must be '%s' or '%s'", orderBy, orderByStudent, orderByAssignment))
			}

			// Print overall summary and get pairs
			pairs := printOverallSummary(students, result, threshold, fileExtensions, ignoreFiles, c.Classroom.Name, orderBy, filterStudent, filterAssignment)

			// If diff mode is enabled, prompt for case selection
			if showDiff && len(pairs) > 0 {
				promptAndShowDiff(pairs, threshold, orderBy)
			}
		},
	}

	cmd.Flags().IntVarP(&aId, "assignment-id", "a", 0, "ID of the assignment to check (unused in new version)")
	cmd.Flags().StringSliceVarP(&fileExtensions, "extension", "e", []string{".html"}, "File extensions to compare (e.g., .html,.css,.js)")
	cmd.Flags().Float64VarP(&threshold, "threshold", "t", 70.0, "Similarity threshold percentage for warnings (0-100)")
	cmd.Flags().StringVarP(&starterFolder, "starter-folder", "s", "", "Name of the starter code folder to exclude (defaults to classroom name)")
	cmd.Flags().StringSliceVarP(&ignoreFiles, "ignore", "i", []string{}, "File names (without extension) to ignore (e.g., reset,normalize)")
	cmd.Flags().BoolVarP(&showDiff, "diff", "d", false, "Interactive mode to show diffs for selected cases")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().StringVarP(&orderBy, "order-by", "o", orderByAssignment, "Order results by 'assignment' or 'student' (default: assignment)")
	cmd.Flags().StringVarP(&filterStudent, "student", "u", "", "Filter to show only pairs involving this student")
	cmd.Flags().StringVarP(&filterAssignment, "assignment", "n", "", "Filter to show only pairs involving this assignment")

	return cmd
}

// FileComparisonDetail stores file comparison details for display
type FileComparisonDetail struct {
	File1      string
	File2      string
	Similarity float64
}

// AssignmentDetail stores assignment details for display
type AssignmentDetail struct {
	Name            string
	MaxSimilarity   float64
	FileComparisons []FileComparisonDetail
}

// StudentPair stores student pair details for display
type StudentPair struct {
	Student1           string
	Student2           string
	FlaggedAssignments []AssignmentDetail
	MaxSimilarity      float64
}

// printOverallSummary prints a summary across all assignments and returns the pairs
func printOverallSummary(students []string, result *similarity.ComparisonResult, threshold float64, fileExtensions []string, ignoreFiles []string, classroomName string, orderBy string, filterStudent string, filterAssignment string) []StudentPair {

	// Print header with parameters
	fmt.Printf("Checking classroom: %s\n", classroomName)
	fmt.Printf("File extensions: %v\n", fileExtensions)
	fmt.Printf("Threshold: %.0f%%\n", threshold)
	if len(ignoreFiles) > 0 {
		fmt.Printf("Ignoring files: %v\n", ignoreFiles)
	}
	if filterStudent != "" {
		fmt.Printf("Filtered by student: %s\n", filterStudent)
	}
	if filterAssignment != "" {
		fmt.Printf("Filtered by assignment: %s\n", filterAssignment)
	}
	fmt.Println("\nAssignments analyzed:")
	for _, assignment := range result.Assignments {
		fmt.Printf("  - %s\n", assignment)
	}
	fmt.Println()

	pairMap := make(map[string]*StudentPair)

	// Count flagged assignments per student pair
	for i, student1 := range students {
		for j := i + 1; j < len(students); j++ {
			student2 := students[j]
			key := student1 + "|" + student2

			pair := &StudentPair{
				Student1:           student1,
				Student2:           student2,
				FlaggedAssignments: []AssignmentDetail{},
			}

			for _, assignment := range result.Assignments {
				// Skip assignment if filter is set and doesn't match
				if filterAssignment != "" && assignment != filterAssignment {
					continue
				}
				if comp, exists := result.Results[student1][student2][assignment]; exists && comp != nil {
					if comp.MaxSimilarity >= threshold {
						// Collect all file comparisons above threshold
						var fileComps []FileComparisonDetail
						for _, fc := range comp.FileComparisons {
							if fc.Similarity >= threshold {
								fileComps = append(fileComps, FileComparisonDetail{
									File1:      fc.File1,
									File2:      fc.File2,
									Similarity: fc.Similarity,
								})
							}
						}

						if len(fileComps) > 0 {
							pair.FlaggedAssignments = append(pair.FlaggedAssignments, AssignmentDetail{
								Name:            assignment,
								MaxSimilarity:   comp.MaxSimilarity,
								FileComparisons: fileComps,
							})
							if comp.MaxSimilarity > pair.MaxSimilarity {
								pair.MaxSimilarity = comp.MaxSimilarity
							}
						}
					}
				}
			}

			if len(pair.FlaggedAssignments) > 0 {
				// Apply student filter if specified
				if filterStudent == "" || pair.Student1 == filterStudent || pair.Student2 == filterStudent {
					pairMap[key] = pair
				}
			}
		}
	}

	if len(pairMap) == 0 {
		if filterStudent != "" {
			fmt.Printf("No similarities found for student: %s\n", filterStudent)
		}
		if filterAssignment != "" {
			fmt.Printf("No similarities found for assignment: %s\n", filterAssignment)
		}
		return []StudentPair{}
	}

	// Convert to slice and sort
	pairs := make([]StudentPair, 0, len(pairMap))
	for _, pair := range pairMap {
		pairs = append(pairs, *pair)
	}

	sort.Slice(pairs, func(i, j int) bool {
		if len(pairs[i].FlaggedAssignments) != len(pairs[j].FlaggedAssignments) {
			return len(pairs[i].FlaggedAssignments) > len(pairs[j].FlaggedAssignments)
		}
		return pairs[i].MaxSimilarity > pairs[j].MaxSimilarity
	})

	if orderBy == orderByAssignment {
		printPairListByAssignment(pairs, threshold, result.Assignments)
	} else {
		printPairListByStudent(pairs, threshold)
	}

	return pairs
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// getColorCode returns ANSI color code based on similarity level
func getColorCode(similarity, threshold float64) string {
	if similarity >= 90 {
		return "\033[1;31m" // Bright red - very high similarity
	} else if similarity >= threshold {
		return "\033[1;33m" // Yellow - above threshold
	}
	return "\033[0;32m" // Green - below threshold
}

// resetColor returns ANSI code to reset color
func resetColor() string {
	return "\033[0m"
}

// printPairListByAssignment shows results organized by assignment
func printPairListByAssignment(pairs []StudentPair, threshold float64, assignments []string) {
	fmt.Printf("Similarity results by assignment:\n")
	fmt.Println(strings.Repeat("-", 80))

	// Create a map of assignment -> list of (caseNum, assignmentDetailIndex, pair)
	type AssignmentCase struct {
		CaseNum             int
		AssignmentNumInCase int
		Pair                StudentPair
		Detail              AssignmentDetail
	}

	assignmentMap := make(map[string][]AssignmentCase)

	// Collect all cases by assignment
	for caseNum, pair := range pairs {
		for assignmentNum, detail := range pair.FlaggedAssignments {
			assignmentMap[detail.Name] = append(assignmentMap[detail.Name], AssignmentCase{
				CaseNum:             caseNum + 1,
				AssignmentNumInCase: assignmentNum + 1,
				Pair:                pair,
				Detail:              detail,
			})
		}
	}

	// Print by assignment
	for _, assignment := range assignments {
		cases, exists := assignmentMap[assignment]
		if !exists || len(cases) == 0 {
			continue
		}

		// Sort cases by similarity (highest first)
		sort.Slice(cases, func(i, j int) bool {
			return cases[i].Detail.MaxSimilarity > cases[j].Detail.MaxSimilarity
		})

		// Print assignment header
		fmt.Printf("\n\033[1;35m%s\033[0m\n", assignment)

		for _, ac := range cases {
			// Print case reference and student pair
			fmt.Printf("\033[0;36m%-3s%-37s%-37s\033[0m\n",
				fmt.Sprintf("%d.%d", ac.CaseNum, ac.AssignmentNumInCase),
				truncateString(ac.Pair.Student1, 37),
				truncateString(ac.Pair.Student2, 37))

			// Print file comparisons
			for _, fc := range ac.Detail.FileComparisons {
				fcColor := getColorCode(fc.Similarity, threshold)
				fmt.Printf("%s   %-37s%-37s(%.1f%%)%s\n",
					fcColor,
					filepath.Base(fc.File1),
					filepath.Base(fc.File2),
					fc.Similarity,
					resetColor())
			}
			fmt.Println()
		}
	}
}

// printPairListByStudent shows results organized by student pairs (original format)
func printPairListByStudent(pairs []StudentPair, threshold float64) {
	fmt.Printf("Student pairs with high similarity:\n")
	fmt.Println(strings.Repeat("-", 80))

	for i, p := range pairs {
		// Format without pipes: "1  student1Name                student2Name"
		fmt.Printf("\033[0;36m%-3d%-37s%-37s\033[0m\n",
			i+1,
			truncateString(p.Student1, 37),
			truncateString(p.Student2, 37))

		// Print details for each flagged assignment with sub-numbering
		for j, detail := range p.FlaggedAssignments {
			detailColor := getColorCode(detail.MaxSimilarity, threshold)
			fmt.Printf("\n%s%d.%d %s: %.1f%%%s\n",
				detailColor, i+1, j+1, detail.Name, detail.MaxSimilarity, resetColor())

			// Print all file comparisons above threshold for this assignment
			for _, fc := range detail.FileComparisons {
				fcColor := getColorCode(fc.Similarity, threshold)
				// Align with student names: 3 spaces (for number) + 37 chars + 37 chars
				fmt.Printf("%s   %-37s%-37s(%.1f%%)%s\n",
					fcColor,
					filepath.Base(fc.File1),
					filepath.Base(fc.File2),
					fc.Similarity,
					resetColor())
			}
		}
		fmt.Println()
	}
}

// promptAndShowDiff prompts user for case selection and shows diffs
func promptAndShowDiff(pairs []StudentPair, threshold float64, orderBy string) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("\n%s\n", strings.Repeat("=", 80))
		fmt.Printf("Enter case number (e.g., 1 or 1.2) to show diffs, 'p' to show summary again, or 'q' to quit: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			return
		}

		input = strings.TrimSpace(input)

		// Check for quit
		if input == "q" || input == "Q" {
			fmt.Println("Exiting diff mode.")
			return
		}

		// Check for print/list command
		if input == "p" || input == "P" || input == "print" {
			fmt.Printf("\n%s\n", strings.Repeat("=", 80))
			if orderBy == orderByAssignment {
				// We need assignments list - extract from pairs
				assignmentSet := make(map[string]bool)
				for _, pair := range pairs {
					for _, detail := range pair.FlaggedAssignments {
						assignmentSet[detail.Name] = true
					}
				}
				assignments := make([]string, 0, len(assignmentSet))
				for assignment := range assignmentSet {
					assignments = append(assignments, assignment)
				}
				sort.Strings(assignments)
				printPairListByAssignment(pairs, threshold, assignments)
			} else {
				printPairListByStudent(pairs, threshold)
			}
			fmt.Printf("%s\n", strings.Repeat("=", 80))
			continue
		}

		// Parse case number (supports both "1" and "1.2" format)
		parts := strings.Split(input, ".")
		if len(parts) == 0 || len(parts) > 2 {
			fmt.Println("Invalid format. Use format like '1' for all assignments or '1.2' for specific assignment.")
			continue
		}

		// Parse case number (first part)
		caseNum, err := strconv.Atoi(parts[0])
		if err != nil || caseNum < 1 || caseNum > len(pairs) {
			fmt.Printf("Invalid case number. Please enter a number between 1 and %d.\n", len(pairs))
			continue
		}

		pair := pairs[caseNum-1]

		// Check if specific assignment is requested
		if len(parts) == 2 {
			assignmentNum, err := strconv.Atoi(parts[1])
			if err != nil || assignmentNum < 1 || assignmentNum > len(pair.FlaggedAssignments) {
				fmt.Printf("Invalid assignment number. Case %d has %d assignment(s).\n", caseNum, len(pair.FlaggedAssignments))
				continue
			}
			// Show diff for specific assignment
			showDiffForAssignment(pair, pair.FlaggedAssignments[assignmentNum-1], threshold, caseNum, assignmentNum)
		} else {
			// Show diffs for all assignments in the case
			showDiffsForCase(pair, threshold, caseNum)
		}
	}
}

// showDiffsForCase shows diffs for all file comparisons in a student pair
func showDiffsForCase(pair StudentPair, threshold float64, caseNum int) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("Case %d: %s | %s\n", caseNum, pair.Student1, pair.Student2)
	fmt.Printf("%s\n", strings.Repeat("=", 80))

	for assignmentNum, assignment := range pair.FlaggedAssignments {
		showDiffForAssignment(pair, assignment, threshold, caseNum, assignmentNum+1)
	}
}

// showDiffForAssignment shows diffs for a specific assignment
func showDiffForAssignment(pair StudentPair, assignment AssignmentDetail, threshold float64, caseNum, assignmentNum int) {
	fmt.Printf("\n%sCase %d.%d - Assignment: %s (%.1f%%)%s\n",
		getColorCode(assignment.MaxSimilarity, threshold),
		caseNum,
		assignmentNum,
		assignment.Name,
		assignment.MaxSimilarity,
		resetColor())

	for _, fc := range assignment.FileComparisons {
		fmt.Printf("\n%s--- %s\n+++ %s\n(%.1f%% similar)%s\n",
			getColorCode(fc.Similarity, threshold),
			fc.File1,
			fc.File2,
			fc.Similarity,
			resetColor())

		// Run diff command
		cmd := exec.Command("diff", "-u", fc.File1, fc.File2)
		output, err := cmd.CombinedOutput()

		if err != nil {
			// diff returns non-zero when files differ, which is expected
			if len(output) > 0 {
				printColoredDiff(string(output))
			} else {
				fmt.Printf("Error running diff: %v\n", err)
			}
		} else {
			// Files are identical
			fmt.Println("Files are identical")
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
}

// printColoredDiff prints diff output with colors
func printColoredDiff(diffOutput string) {
	lines := strings.Split(diffOutput, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			fmt.Println()
			continue
		}

		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			// File headers in bold
			fmt.Printf("\033[1m%s\033[0m\n", line)
		case strings.HasPrefix(line, "@@"):
			// Hunk headers in cyan
			fmt.Printf("\033[0;36m%s\033[0m\n", line)
		case strings.HasPrefix(line, "-"):
			// Removed lines in red
			fmt.Printf("\033[0;31m%s\033[0m\n", line)
		case strings.HasPrefix(line, "+"):
			// Added lines in green
			fmt.Printf("\033[0;32m%s\033[0m\n", line)
		default:
			// Context lines in default color
			fmt.Println(line)
		}
	}
}
