package sync

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/majikmate/gh-mmc/cmd/pull"
	"github.com/spf13/cobra"
)

func NewCmdSync(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Pulls a course and synchronizes its student repositories with the starter repository",
		Long: heredoc.Doc(`

			Pulls a course and synchronizes its student repositories with the starter repository
			on GitHub, so that the students can pull the changes of the starter repository, e.g.
			example code that shall be distributed to the students.

			The command must be run within a course folder, i.e. the folder containing the .mmc
			folder with the course.json file or any folder below it. It always operates in the
			course folder and returns to the current folder afterwards. If there is no course
			folder, it aborts with an error.

			The command does everything gh mmc pull does, see gh mmc pull --help. Additionally, it
			synchronizes the default branch of every valid student repository on GitHub with the
			starter repository before pulling it, so that the local clones have the latest state.
			Student repositories created in the same run are up to date already. Student
			repositories whose changes conflict with the changes of the starter repository cannot
			be synchronized and are reported as error. Invalid student repositories are only
			reported and not synchronized.

			Student repositories that received changes of the starter repository are labeled
			Synced in the list of repositories, see gh mmc pull --help.`),
		Example: `$ gh mmc sync`,
		Run: func(cmd *cobra.Command, args []string) {
			pull.Run(pull.Options{Sync: true})
		},
	}

	return cmd
}
