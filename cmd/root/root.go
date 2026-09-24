package root

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/majikmate/gh-mmc/cmd/check"
	"github.com/majikmate/gh-mmc/cmd/codespaces"
	"github.com/majikmate/gh-mmc/cmd/deletion"
	"github.com/majikmate/gh-mmc/cmd/initialize"
	"github.com/majikmate/gh-mmc/cmd/pull"
	"github.com/majikmate/gh-mmc/cmd/sync"
	"github.com/majikmate/gh-mmc/pkg/ghapi"
	"github.com/majikmate/gh-mmc/pkg/mmc"
	"github.com/spf13/cobra"
)

func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mmc <command>",
		Short: "\nAn opinionated GitHub Classroom CLI",
		// No command runs with an elevated gh token that a killed run left behind, or without gh being logged in
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := ghapi.EnsureAuth(); err != nil {
				mmc.Fatal(err)
			}
		},
	}

	cmd.AddCommand(initialize.NewCmdInit(f))
	cmd.AddCommand(pull.NewCmdPull(f))
	cmd.AddCommand(sync.NewCmdSync(f))
	cmd.AddCommand(check.NewCmdCheck(f))
	cmd.AddCommand(codespaces.NewCmdCodespaces(f))
	cmd.AddCommand(deletion.NewCmdClean(f))
	cmd.AddCommand(deletion.NewCmdDelete(f))

	return cmd
}
