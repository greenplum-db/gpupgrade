package utils

import "github.com/spf13/cobra"

// This global var UpgradeVersion should have a value set at build time.
// see Makefile for -ldflags "-X etc"
var UpgradeVersion = ""

type VersionCommand struct{}

const defaultUpgradeVersionSuffix = " unknown version"

func VersionString(executableName string) string {
	if UpgradeVersion == "" {
		return executableName + defaultUpgradeVersionSuffix
	}
	return executableName + " version " + UpgradeVersion
}

func VersionAddCmdlineOption(cmd *cobra.Command, doLogVersionAndExit *bool) {
	cmd.Flags().BoolVarP(doLogVersionAndExit, "version", "v", false, "prints out version information and exits")
}
