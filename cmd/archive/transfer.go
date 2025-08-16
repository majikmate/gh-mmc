package archive

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewCmdArchive(f *cmdutil.Factory) *cobra.Command {
	var aId int
	var verbose bool

	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Transfers classroom organization ownership to each single student and archives the classroom",
		Long: heredoc.Doc(`

			Transfers classroom organization ownership to each single student and archives 
			the classroom.

			This command invites all students as members of the classroom organization
			and grants each of them ownership of the organization with the intention
			to transfer the organization ownership to all of them. This allows to leave
			the organization as owner and avoids organization clutter in the own account.

			Because organisation management needs additional authorization scopes,
			gh cli needs to be authorized to include these scopes. Please run:

			gh auth refresh -h github.com -s admin:org

			and follow the instructions before running this command.
		`),
		Example: `$ gh mmc archive`,
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
