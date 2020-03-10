package cmd

import (
	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/parser"
	"github.com/modularise/modularise/internal/splitapi"
)

func RunCheck(c *config.CLIConfig) error {
	c.Logger.Info("Parsing split configuration.")
	if err := parser.Parse(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	c.Logger.Info("Checking self-contained character of split APIs and computing residual packages.")
	if err := splitapi.AnalyseAPI(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}
	c.Logger.Info("The split configuration in " + c.ConfigFile + " is valid.")
	return nil
}
