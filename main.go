package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/modularise/modularise/cmd"
)

func main() {
	var c cmd.CLIConfig
	root := cobra.Command{
		Use: "modularise",
		PersistentPreRunE: func(_ *cobra.Command, args []string) error {
			return c.CheckConfig()
		},
		SilenceUsage: true,
	}

	attachGlobalFlags(&root, &c)

	root.AddCommand(
		checkCmd(&c),
		splitCmd(&c),
	)

	if err := root.Execute(); err != nil {
		logrus.WithError(err).Debug("Execution failed.")
		os.Exit(1)
	}
}

func checkCmd(c *cmd.CLIConfig) *cobra.Command {
	check := &cobra.Command{
		Use: "check",
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmd.RunCheck(c)
		},
	}

	return check
}

func splitCmd(c *cmd.CLIConfig) *cobra.Command {
	split := &cobra.Command{
		Use: "split",
		RunE: func(_ *cobra.Command, _ []string) error {
			return cmd.RunSplit(c)
		},
	}
	attachSplitFlags(split, c)

	return split
}
