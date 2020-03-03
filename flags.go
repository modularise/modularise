package main

import (
	"github.com/spf13/cobra"

	"github.com/modularise/modularise/cmd/config"
)

func attachGlobalFlags(command *cobra.Command, c *config.CLIConfig) {
	command.PersistentFlags().StringVarP(
		&c.ConfigFile,
		"configuration",
		"c",
		"",
		"Location of the configuration file to use.",
	)
	command.PersistentFlags().BoolVarP(
		&c.Verbose,
		"verbose",
		"v",
		false,
		"Print (very) verbose debug logs.",
	)
}

func attachSplitFlags(command *cobra.Command, c *config.CLIConfig) {
	command.Flags().BoolVarP(
		&c.DryRun,
		"dry-run",
		"d",
		false,
		"Perform the full split flow but do not push results to the remote split repositories",
	)
	command.Flags().StringVarP(
		&c.WorkDirectory,
		"work-directory",
		"w",
		"",
		"Directory to which to write all newly created content for all configured splits. Any existing content will be removed. "+
			"If not specified a temporary folder will be used.",
	)
}
