package root

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/majikmate/gh-mmc/cmd/codespaces"
	"github.com/majikmate/gh-mmc/cmd/initialize"
	"github.com/majikmate/gh-mmc/cmd/pull"
	"github.com/majikmate/gh-mmc/cmd/sync"
	"github.com/spf13/cobra"
)

func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mmc <command>",
		Short: "\nAn opinionated GitHub Classroom CLI",
	}

	cmd.AddCommand(initialize.NewCmdInit(f))
	cmd.AddCommand(pull.NewCmdPull(f))
	cmd.AddCommand(sync.NewCmdSync(f))
	cmd.AddCommand(codespaces.NewCmdCodespaces(f))

	return cmd
}
