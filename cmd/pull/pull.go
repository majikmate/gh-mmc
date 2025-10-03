package pull

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewCmdPull(f *cmdutil.Factory) *cobra.Command {
	var aId int
	var starterFolder string
	var isAssignmentFolder bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Clone and pull all repositories for an assignment",
		Long: heredoc.Doc(`
		
			Clones and pulls all repositories for an assignment, including the starter
			code repository and all student repositories.

			This command will:
			- Clone repositories that don't exist locally
			- Pull updates for repositories that are already cloned
			- Handle both starter code repository (.main folder) and student repositories
			- Create assignment folder if running from classroom folder

			The command looks for repositories in the current directory. If a repository 
			doesn't exist locally, it will be cloned first. If it exists, the latest 
			changes will be pulled from the default branch.
			
			The command can be run within the folder of an assignment, in which case the
			assignment-id is automatically detected. If the assignment-id is known, it can 
			be passed as an argument. Otherwise, the user will be prompted to 
			select a classroom.`),
		Example: `$ gh mmc pull`,
		Run: func(cmd *cobra.Command, args []string) {
			client, err := api.DefaultRESTClient()
			if err != nil {
				mmc.Fatal(err)
			}

			c, err := mmc.LoadClassroom()
			if err != nil {
				mmc.Fatal(err)
			}

			isClassroomFolder, err := mmc.IsClassroomFolder()
			if err != nil {
				mmc.Fatal(err)
			}

			if isAssignmentFolder, err = mmc.IsAssignmentFolder(); err == nil && isAssignmentFolder {
				a, err := mmc.LoadAssignment()
				if err != nil {
					mmc.Fatal(err)
				}
				aId = a.Id
			}
			if err != nil {
				mmc.Fatal(err)
			}

			if !isClassroomFolder && !isAssignmentFolder {
				mmc.Fatal("No classroom or assignment found. `gh mmc pull` should either be run from within a classroom folder or from within an assignment folder. Run `gh mmc init` to initialize a classroom folder or change to an initialized classroom folder.")
			}

			if aId == 0 {
				a, err := ghapi.PromptForAssignment(client, c.Classroom.Id)
				if err != nil {
					mmc.Fatal(err)
				}

				aId = a.Id
			}

			assignment, err := ghapi.GetAssignment(client, aId)
			if err != nil {
				mmc.Fatal(err)
			}

			var assignmentPath string
			if isAssignmentFolder {
				assignmentPath, err = os.Getwd()
			} else {
				assignmentPath, err = filepath.Abs(assignment.Slug)
			}
			if err != nil {
				fmt.Println("Error getting absolute path for directory: ", err)
				return
			}

			if !isAssignmentFolder {
				if _, err := os.Stat(assignmentPath); os.IsNotExist(err) {
					fmt.Println("Creating directory: ", assignmentPath)
					err = os.MkdirAll(assignmentPath, 0755)
					if err != nil {
						mmc.Fatal(err)
					}
				}

				a := mmc.NewAssignment()
				a.Set(assignment.Id, assignment.Slug)
				err = a.Save(assignmentPath)
				if err != nil {
					mmc.Fatal(err)
				}

				// Change to assignment directory
				err = os.Chdir(assignmentPath)
				if err != nil {
					mmc.Fatal(fmt.Errorf("failed to change to assignment directory: %v", err))
				}
			}

			acceptedAssignmentList, err := ghapi.ListAllAcceptedAssignments(client, aId, 15)
			if err != nil {
				mmc.Fatal(err)
			}

			totalPulled := 0
			totalCloned := 0
			pullErrors := []string{}

			// Get current directory after potential assignment folder creation
			currentDir, err := os.Getwd()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get current directory: %v", err))
			}

			// Clone starter code repository if it exists and isn't already cloned
			if assignment.StarterCodeRepository.Id != 0 {
				if starterFolder == "" {
					starterFolder = ".main"
				}
				starterPath := filepath.Join(currentDir, starterFolder)

				if _, err := os.Stat(starterPath); os.IsNotExist(err) {
					// Starter repo doesn't exist, clone it
					_, _, err := gh.Exec("repo", "clone", assignment.StarterCodeRepository.FullName, starterFolder)
					if err != nil {
						errMsg := fmt.Sprintf("Failed to clone starter repository %s (%s): %v", starterFolder, assignment.StarterCodeRepository.HtmlUrl, err)
						pullErrors = append(pullErrors, errMsg)
						fmt.Printf("Failed to clone starter repository: %s\n", starterFolder)
					} else {
						fmt.Printf("Cloned starter repository: %s (%s)\n", starterFolder, assignment.StarterCodeRepository.HtmlUrl)
						totalCloned++
					}
				} else {
					// Starter repo exists, pull changes
					cmd := exec.Command("git", "pull")
					cmd.Dir = starterPath
					output, err := cmd.CombinedOutput()
					if err != nil {
						errMsg := fmt.Sprintf("Failed to pull starter repository %s (%s): %v\nOutput: %s", starterFolder, assignment.StarterCodeRepository.HtmlUrl, err, string(output))
						pullErrors = append(pullErrors, errMsg)
						fmt.Printf("Failed to pull starter repository: %s\n", starterFolder)
					} else {
						fmt.Printf("Pulled starter repository: %s (%s)\n", starterFolder, assignment.StarterCodeRepository.HtmlUrl)
						totalPulled++
					}
				}
			}

			fmt.Printf("Processing %d student repositories...\n\n", len(acceptedAssignmentList.AcceptedAssignments))

			for i, acceptedAssignment := range acceptedAssignmentList.AcceptedAssignments {
				repoName := acceptedAssignment.Repository.Name
				if len(acceptedAssignment.Students) == 1 {
					if name, err := c.GetRepoName(acceptedAssignment.Students[0].Login); err == nil {
						repoName = name
					}
				}

				fmt.Printf("[%d/%d] Processing %s...", i+1, len(acceptedAssignmentList.AcceptedAssignments), repoName)

				repoPath := filepath.Join(currentDir, repoName)

				// Check if repository directory exists
				if _, err := os.Stat(repoPath); os.IsNotExist(err) {
					// Repository doesn't exist, clone it
					_, _, err := gh.Exec("repo", "clone", acceptedAssignment.Repository.FullName, repoName)
					if err != nil {
						errMsg := fmt.Sprintf("Failed to clone %s (%s): %v", repoName, acceptedAssignment.Repository.HtmlUrl, err)
						pullErrors = append(pullErrors, errMsg)
						if verbose {
							fmt.Printf(" FAILED\n%s\n", errMsg)
						} else {
							fmt.Printf(" FAILED\n")
						}
						continue
					}
					fmt.Printf(" CLONED\n")
					totalCloned++
				} else {
					// Repository exists, pull changes
					// Try git pull using standard git command
					cmd := exec.Command("git", "pull")
					cmd.Dir = repoPath
					output, err := cmd.CombinedOutput()
					if err != nil {
						errMsg := fmt.Sprintf("Failed to pull %s (%s): %v\nOutput: %s", repoName, acceptedAssignment.Repository.HtmlUrl, err, string(output))
						pullErrors = append(pullErrors, errMsg)
						if verbose {
							fmt.Printf(" FAILED\n%s\n", errMsg)
						} else {
							fmt.Printf(" FAILED\n")
						}
						continue
					}

					fmt.Printf(" PULLED\n")
					totalPulled++
				}
			}

			if len(pullErrors) > 0 {
				fmt.Printf("\n%d repositories failed to pull/clone:\n", len(pullErrors))
				if !verbose {
					fmt.Println("Run with --verbose flag to see detailed error messages")
					for _, errMsg := range pullErrors {
						// Extract just the repo name from the error message for summary
						if strings.Contains(errMsg, "Failed to clone ") {
							prefix := "Failed to clone "
							if len(errMsg) > len(prefix) && errMsg[:len(prefix)] == prefix {
								remaining := errMsg[len(prefix):]
								if parenIdx := strings.Index(remaining, " ("); parenIdx > 0 {
									repoName := remaining[:parenIdx]
									fmt.Printf("  - %s (clone failed)\n", repoName)
								}
							}
						} else if strings.Contains(errMsg, "Failed to pull ") {
							prefix := "Failed to pull "
							if len(errMsg) > len(prefix) && errMsg[:len(prefix)] == prefix {
								remaining := errMsg[len(prefix):]
								if parenIdx := strings.Index(remaining, " ("); parenIdx > 0 {
									repoName := remaining[:parenIdx]
									fmt.Printf("  - %s (pull failed)\n", repoName)
								}
							}
						}
					}
				} else {
					for _, errMsg := range pullErrors {
						fmt.Printf("  %s\n", errMsg)
					}
				}
				fmt.Printf("\nResults: %d cloned, %d pulled, %d failed out of %d total repositories.\n",
					totalCloned, totalPulled, len(pullErrors), totalCloned+totalPulled+len(pullErrors))
			} else {
				fmt.Printf("\nSuccessfully processed all %d repositories (%d cloned, %d pulled).\n",
					totalCloned+totalPulled, totalCloned, totalPulled)
			}
		},
	}

	cmd.Flags().IntVarP(&aId, "assignment-id", "a", 0, "ID of the assignment")
	cmd.Flags().StringVarP(&starterFolder, "starter-folder", "s", "", "name of the folder the starter code shall be cloned to")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose error output")

	return cmd
}
