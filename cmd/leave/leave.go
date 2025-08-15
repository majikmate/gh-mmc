package leave

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewCmdLeave(f *cmdutil.Factory) *cobra.Command {
	var aId int
	var verbose bool

	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Leaves a classroom and hands over ownership to each single student",
		Long: heredoc.Doc(`

			Leaves a classroom and hands over ownership to each single student.

			This command adds all students as members of the organization and grants
			each of them ownership of the organization.
		`),
		Example: `$ gh mmc leave`,
		Run: func(cmd *cobra.Command, args []string) {
			client, err := api.DefaultRESTClient()
			if err != nil {
				mmc.Fatal(err)
			}

			c, err := mmc.LoadClassroom()
			if err != nil {
				mmc.Fatal(err)
			}

			crm, err := ghapi.GetClassroom(client, c.Classroom.Id)
			if err != nil {
				mmc.Fatal(err)
			}

			err = ghapi.AddOrganizationOwner(client, crm.Organization.Login, "staussh")
			if err != nil {
				mmc.Fatal(err)
			}

			// role := "admin" // "admin" == organization owner

			// m, resp, err := client.Organizations.EditOrgMembership(ctx, username, org, &github.Membership{
			// 	Role: &role,
			// })
			// if err != nil {
			// 	log.Fatalf("EditOrgMembership failed: %v (HTTP %d)", err, resp.StatusCode)
			// }

			// fmt.Printf("state=%s role=%s\n", m.GetState(), m.GetRole())

			fmt.Printf("Organization %s left.\n", crm.Organization.Login)
		},
	}

	cmd.Flags().IntVarP(&aId, "assignment-id", "a", 0, "ID of the assignment")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose error output")

	return cmd
}
