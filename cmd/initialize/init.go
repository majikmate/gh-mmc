package initialize

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewCmdInit(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Creates or updates the roster of the classroom and invites the students",
		Long: heredoc.Doc(`

			Creates or updates the roster of the classroom in the current folder from a list of
			accounts, and makes sure that every student on the roster is a member of the
			GitHub organization of the classroom or is invited to it.

			The command can be run in the classroom folder or in any folder below it, and
			always operates in the classroom folder. The classroom folder is the nearest
			folder, searching upwards from the current folder, that is either the folder of
			an existing classroom, i.e. the folder containing the .mmc folder with the
			classroom.json file, or, for a new classroom, the folder containing the accounts
			file. If neither is found, the command aborts with an error.

			The accounts are read from an Excel file in the classroom folder that matches
			the filename pattern [Aa]ccounts*.xlsx. It must contain a header in the first
			row with following fields:

			- Name         ... Full name of the student
			- Email        ... Email address of the student
			- GitHub User  ... GitHub username of the student

			The user will be prompted to select the GitHub organization from the list of
			organizations the user is a member of, and then the organizations containing the
			template repositories that courses are created from. At least one organization
			must be selected. The organizations selected before are preselected, or for a new
			classroom, the organization of the classroom. The selected organizations are
			saved in .mmc/classroom.json.

			Students that are not members of the organization are invited to it, unless an
			invitation is pending already. Inviting requires the user to be an owner of the
			organization and the gh token to have the admin:org scope. If it does not have
			it, the scope is added for inviting, which requires to authenticate in the
			browser, and removed again afterwards without any interaction, even if the
			command fails or is interrupted.

			A table summarizes the membership status of the students on the roster.`),
		Example: `$ gh mmc init`,
		Run: func(cmd *cobra.Command, args []string) {
			// The classroom root is the nearest folder of an existing classroom or, for a new classroom, the
			// nearest folder containing the accounts file
			classroomFolder, err := mmc.FindInitFolder()
			if err != nil {
				mmc.Fatal(err)
			}

			// Execute in the classroom root and return to the current folder afterwards
			restore, err := mmc.ChangeToFolder(classroomFolder)
			if err != nil {
				mmc.Fatal(err)
			}
			defer restore()

			client, err := api.DefaultRESTClient()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to create gh client: %v", err))
			}

			as, err := mmc.ReadAccounts()
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to read accounts: %v", err))
			}

			organizations, err := ghapi.ListAllOrganizations(client)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to list organizations: %v", err))
			}

			org, err := ghapi.SelectOrganization(organizations)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get organization: %v", err))
			}

			// The template organizations of an existing classroom are preselected, else the organization of the
			// classroom
			defaults := []string{org.Login}
			if existing, err := mmc.LoadClassroomFrom(classroomFolder); err == nil && len(existing.TemplateOrganizations) > 0 {
				defaults = existing.TemplateOrganizationLogins()
			}
			templateOrgs, err := ghapi.SelectOrganizations("Select the organizations containing template repositories (ESC or Ctrl+C to cancel):", organizations, defaults)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to get template organizations: %v", err))
			}

			c := mmc.NewClassroom()
			c.SetOrganization(org.Id, org.Login)
			for _, o := range templateOrgs {
				c.AddTemplateOrganization(o.Id, o.Login)
			}
			for _, a := range as {
				c.AddStudent(a.Name, a.Email, a.GithubUser)
			}
			err = c.Save(classroomFolder)
			if err != nil {
				mmc.Fatal(fmt.Errorf("failed to save classroom: %v", err))
			}

			// Students must be members of the organization
			membership, err := ghapi.LoadOrganizationMembership(client, org.Login)
			if err != nil {
				mmc.Fatal(err)
			}

			invitees := make([]ghapi.Invitee, 0, len(as))
			for _, a := range as {
				invitees = append(invitees, ghapi.Invitee{Login: a.GithubUser, Email: a.Email})
			}
			_, errs := membership.InviteMissing(client, invitees)

			totalMembers, totalInvited, totalPending := 0, 0, 0
			inviteErrors := []string{}
			for _, a := range as {
				if err := errs[strings.ToLower(a.GithubUser)]; err != nil {
					// Don't bail on an error, continue with the rest of the students
					errMsg := fmt.Sprintf("Failed: %s: %v", a.Name, err)
					inviteErrors = append(inviteErrors, errMsg)
					fmt.Println(errMsg)
					continue
				}
				switch membership.Status(a.GithubUser, a.Email) {
				case ghapi.MembershipActive:
					totalMembers++
				case ghapi.MembershipPending:
					totalPending++
					fmt.Printf("Invitation to organization %s pending: %s (%s)\n", org.Login, a.Name, a.GithubUser)
				case ghapi.MembershipInvited:
					totalInvited++
					fmt.Printf("Invited to organization %s: %s (%s)\n", org.Login, a.Name, a.GithubUser)
				}
			}

			if len(inviteErrors) > 0 {
				fmt.Printf("\n%d students could not be invited:\n", len(inviteErrors))
				for _, errMsg := range inviteErrors {
					fmt.Printf("  %s\n", errMsg)
				}
			}

			printRosterTable(filepath.Base(classroomFolder), org.Login, totalMembers, totalInvited, totalPending, len(inviteErrors), len(as))
			if len(inviteErrors) > 0 {
				os.Exit(1)
			}
		},
	}

	return cmd
}

// printRosterTable prints an overview of the membership of the students on the roster in the organization
func printRosterTable(classroom, org string, existing, invited, pending, failed, total int) {
	rows := [][]string{
		{"Existing", strconv.Itoa(existing), "members of the organization"},
		{"Invited", strconv.Itoa(invited), "invited now"},
		{"Pending", strconv.Itoa(pending), "invited before, not accepted yet"},
	}
	if failed > 0 {
		rows = append(rows, []string{"Failed", strconv.Itoa(failed), "could not be invited, see the errors above"})
	}

	mmc.Table{
		Title:        fmt.Sprintf("Roster of classroom %s in organization %s:", classroom, org),
		Header:       []string{"Status", "Students", "Meaning"},
		Rows:         rows,
		Footer:       []string{"Total", strconv.Itoa(total), ""},
		RightAligned: map[int]bool{1: true},
	}.Print()
}
