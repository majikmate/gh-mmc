package sync

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewCmdSync(f *cmdutil.Factory) *cobra.Command {
	var aId int
	var verbose bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronizes student repos for an assignment with the starter repo",
		Long: heredoc.Doc(`
		
			Synchronizes student repos for an assignment with the starter repo they are
			forked from on GitHub.

			As a result, students can pull in updated code from the starter repo to their
			local repositories. This is most useful when the starter repo is updated with, 
			e.g., example code that shall be distributed to the students.
			
			The command can be run within the folder of an assignment, in which case the
			assignment-id is automatically detected. If the assigment-id is known, it can 
			be passed as an argument. Otherwise, the user will be prompted to 
			select a classroom.`),
		Example: `$ gh mmc sync`,
		Run: func(cmd *cobra.Command, args []string) {
			// Save the starting directory to return to it at the end
			startingDir, err := os.Getwd()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get current directory: %v", err))
			}
			defer func() {
				_ = os.Chdir(startingDir)
			}()

			client, err := api.DefaultRESTClient()
			if err != nil {
				mmc.Fatal(err)
			}

			c, err := mmc.LoadClassroom()
			if err != nil {
				mmc.Fatal(err)
			}

			a, err := mmc.LoadAssignment()
			if err != nil {
				if errors.Is(err, mmc.ErrAssignmentNotFound) {
					a, err := ghapi.PromptForAssignment(client, c.Classroom.Id)
					if err != nil {
						mmc.Fatal(err)
					}

					aId = a.Id
				} else {
					mmc.Fatal(err)
				}
			} else {
				aId = a.Id
			}

			acceptedAssignmentList, err := ghapi.ListAllAcceptedAssignments(client, aId, 15)
			if err != nil {
				mmc.Fatal(err)
			}

			totalSyched := 0
			syncErrors := []string{}
			for _, acceptedAssignment := range acceptedAssignmentList.AcceptedAssignments {
				repoName := acceptedAssignment.Repository.Name
				if len(acceptedAssignment.Students) == 1 {
					if name, err := c.GetRepoName(acceptedAssignment.Students[0].Login); err == nil {
						repoName = name
					}
				}
				_, _, err := gh.Exec("repo", "sync", acceptedAssignment.Repository.FullName)
				if err != nil {
					//Don't bail on an error the repo could have changes preventing
					//a pull, continue with rest of repos
					errMsg := fmt.Sprintf("Failed to sync %s (%s): %v", repoName, acceptedAssignment.Repository.HtmlUrl, err)
					syncErrors = append(syncErrors, errMsg)
					if verbose {
						fmt.Println(errMsg)
					} else {
						fmt.Printf("Failed to sync: %s (%s)\n", repoName, acceptedAssignment.Repository.HtmlUrl)
					}
					continue
				}
				fmt.Printf("Synchronized: %s (%s)\n", repoName, acceptedAssignment.Repository.HtmlUrl)
				totalSyched++
			}
			if len(syncErrors) > 0 {
				fmt.Printf("\n%d repositories failed to sync:\n", len(syncErrors))
				if !verbose {
					fmt.Println("Run with --verbose flag to see detailed error messages")
					for _, errMsg := range syncErrors {
						// Extract just the repo name from the error message for summary
						prefix := "Failed to sync "
						if len(errMsg) > len(prefix) && errMsg[:len(prefix)] == prefix {
							remaining := errMsg[len(prefix):]
							if parenIdx := strings.Index(remaining, " ("); parenIdx > 0 {
								repoName := remaining[:parenIdx]
								fmt.Printf("  - %s\n", repoName)
							} else {
								fmt.Printf("  - %s\n", remaining)
							}
						}
					}
				} else {
					for _, errMsg := range syncErrors {
						fmt.Printf("  %s\n", errMsg)
					}
				}
				fmt.Printf("\nSuccessfully synced %d out of %d repositories.\n", totalSyched, totalSyched+len(syncErrors))
			} else {
				fmt.Printf("\nSuccessfully synced all %d repositories.\n", totalSyched)
			}
		},
	}

	cmd.Flags().IntVarP(&aId, "assignment-id", "a", 0, "ID of the assignment")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose error output")

	return cmd
}
