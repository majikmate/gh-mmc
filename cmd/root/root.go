package root

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/majikmate/gh-mmc/cmd/archive"
	"github.com/majikmate/gh-mmc/cmd/clone"
	"github.com/majikmate/gh-mmc/cmd/initialize"
	"github.com/majikmate/gh-mmc/cmd/sync"
	"github.com/spf13/cobra"
)

func NewRootCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mmc <command>",
		Short: "\nAn opinionated GitHub Classroom CLI",
	}

	cmd.AddCommand(initialize.NewCmdInit(f))
	cmd.AddCommand(clone.NewCmdClone(f))
	cmd.AddCommand(sync.NewCmdSync(f))
	cmd.AddCommand(archive.NewCmdArchive(f))

	return cmd
}
