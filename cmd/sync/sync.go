package sync

import (
	"errors"
	"fmt"

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
					fmt.Println(err)
					continue
				}
				fmt.Printf("Synchronized: %s (%s)\n", repoName, acceptedAssignment.Repository.FullName)
				totalSyched++
			}
			if len(syncErrors) > 0 {
				fmt.Println("Some repositories failed to sync.")
				if !verbose {
					fmt.Println("Run with --verbose flag to see more details")
				} else {
					for _, errMsg := range syncErrors {
						fmt.Println(errMsg)
					}
				}
			}
			fmt.Printf("Synched %v repos.\n", totalSyched)
		},
	}

	cmd.Flags().IntVarP(&aId, "assignment-id", "a", 0, "ID of the assignment")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose error output")

	return cmd
}
