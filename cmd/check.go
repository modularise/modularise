package cmd

import "github.com/Helcaraxan/modularise/cmd/config"

func RunCheck(c *config.CLIConfig) error {
	c.Logger.Fatal("The 'check' command is not yet functional.")
	return nil
}
