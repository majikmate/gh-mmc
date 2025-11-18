package check

import (
"fmt"
"os"
"path/filepath"
"sort"
"strings"

"github.com/MakeNowJust/heredoc"
"github.com/cli/cli/v2/pkg/cmdutil"
"github.com/majikmate/gh-mmc/pkg/mmc"
"github.com/majikmate/gh-mmc/pkg/similarity"
"github.com/spf13/cobra"
)

func NewCmdCheck(f *cmdutil.Factory) *cobra.Command {
	var aId int
	var fileExtension string
	var threshold float64
	var starterFolder string

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
					mmc.Fatal("No classroom found. Run `gh mmc init` to initialize a classroom folder or change to a classroom/assignment folder.")
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

			// Ensure file extension starts with a dot
			if fileExtension != "" && !strings.HasPrefix(fileExtension, ".") {
				fileExtension = "." + fileExtension
			}

			fmt.Printf("Checking classroom: %s\n", c.Classroom.Name)
			fmt.Printf("Search path: %s\n", searchPath)
			fmt.Printf("File extension: %s\n", fileExtension)
			fmt.Printf("Threshold: %.0f%%\n\n", threshold)

			// Run the comparison
			result, err := similarity.CompareAssignments(searchPath, fileExtension, starterFolder)
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

			// Print overall summary only
			printOverallSummary(students, result, threshold)
		},
	}

	cmd.Flags().IntVarP(&aId, "assignment-id", "a", 0, "ID of the assignment to check (unused in new version)")
	cmd.Flags().StringVarP(&fileExtension, "extension", "e", ".html", "File extension to compare (e.g., .html, .css, .js)")
	cmd.Flags().Float64VarP(&threshold, "threshold", "t", 70.0, "Similarity threshold percentage for warnings (0-100)")
	cmd.Flags().StringVarP(&starterFolder, "starter-folder", "s", "", "Name of the starter code folder to exclude (defaults to classroom name)")

	return cmd
}

// printAssignmentResults prints similarity results for a specific assignment
func printAssignmentResults(assignment string, students []string, result *similarity.ComparisonResult, threshold float64) {
	type similarityPair struct {
		student1   string
		student2   string
		similarity float64
		file1      string
		file2      string
	}

	var highSimilarityPairs []similarityPair

	// Find all pairs above threshold for this assignment
	for i, student1 := range students {
		for j := i + 1; j < len(students); j++ {
			student2 := students[j]

			if comp, exists := result.Results[student1][student2][assignment]; exists && comp != nil {
				if comp.MaxSimilarity >= threshold {
					highSimilarityPairs = append(highSimilarityPairs, similarityPair{
student1:   student1,
student2:   student2,
similarity: comp.MaxSimilarity,
file1:      comp.File1,
file2:      comp.File2,
})
				}
			}
		}
	}

	if len(highSimilarityPairs) == 0 {
		return
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("Assignment: %s\n", assignment)
	fmt.Printf("%s\n\n", strings.Repeat("=", 80))

	// Sort by similarity (highest first)
	sort.Slice(highSimilarityPairs, func(i, j int) bool {
return highSimilarityPairs[i].similarity > highSimilarityPairs[j].similarity
	})

	fmt.Printf("⚠️  High Similarity Warnings (threshold: %.0f%%):\n", threshold)
	fmt.Println(strings.Repeat("-", 80))

	for i, p := range highSimilarityPairs {
		colorCode := getColorCode(p.similarity, threshold)
		fmt.Printf("%d. %s%-30s%s <-> %s%-30s%s: %s%.1f%%%s\n",
i+1,
colorCode, truncateString(p.student1, 30),
resetColor(),
			colorCode, truncateString(p.student2, 30),
			resetColor(),
			colorCode, p.similarity,
			resetColor())
		
		if p.file1 != "" && p.file2 != "" {
			fmt.Printf("   Files: %s\n", filepath.Base(p.file1))
			fmt.Printf("          %s\n", filepath.Base(p.file2))
		}
	}
}

// printOverallSummary prints a summary across all assignments
func printOverallSummary(students []string, result *similarity.ComparisonResult, threshold float64) {
	type studentPair struct {
		student1          string
		student2          string
		flaggedAssignments int
		maxSimilarity     float64
	}

	pairMap := make(map[string]*studentPair)

	// Count flagged assignments per student pair
	for i, student1 := range students {
		for j := i + 1; j < len(students); j++ {
			student2 := students[j]
			key := student1 + "|" + student2

			pair := &studentPair{
				student1: student1,
				student2: student2,
			}

			for _, assignment := range result.Assignments {
				if comp, exists := result.Results[student1][student2][assignment]; exists && comp != nil {
					if comp.MaxSimilarity >= threshold {
						pair.flaggedAssignments++
						if comp.MaxSimilarity > pair.maxSimilarity {
							pair.maxSimilarity = comp.MaxSimilarity
						}
					}
				}
			}

			if pair.flaggedAssignments > 0 {
				pairMap[key] = pair
			}
		}
	}

	if len(pairMap) == 0 {
		return
	}

	// Convert to slice and sort
	pairs := make([]studentPair, 0, len(pairMap))
	for _, pair := range pairMap {
		pairs = append(pairs, *pair)
	}

	sort.Slice(pairs, func(i, j int) bool {
if pairs[i].flaggedAssignments != pairs[j].flaggedAssignments {
return pairs[i].flaggedAssignments > pairs[j].flaggedAssignments
		}
		return pairs[i].maxSimilarity > pairs[j].maxSimilarity
	})

	fmt.Printf("Student pairs with high similarity:\n")
	fmt.Println(strings.Repeat("-", 80))

	for i, p := range pairs {
		colorCode := getColorCode(p.maxSimilarity, threshold)
		fmt.Printf("%d. %s%-30s%s <-> %s%-30s%s: %d assignment(s), max %s%.1f%%%s\n",
			i+1,
			colorCode, truncateString(p.student1, 30),
			resetColor(),
			colorCode, truncateString(p.student2, 30),
			resetColor(),
			p.flaggedAssignments,
			colorCode, p.maxSimilarity,
			resetColor())
	}
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
