package commands

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/greenplum-db/gpupgrade/utils"
)

func revertAction(cmd *cobra.Command, args []string) error {
	return os.RemoveAll(utils.GetStateDir())
}

func Revert() *cobra.Command {
	return &cobra.Command{
		Use:  "revert",
		RunE: revertAction,
	}
}
