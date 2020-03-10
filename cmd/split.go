package cmd

import (
	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/chopper"
	"github.com/modularise/modularise/internal/modworks"
	"github.com/modularise/modularise/internal/parser"
	"github.com/modularise/modularise/internal/repohandler"
	"github.com/modularise/modularise/internal/residuals"
	"github.com/modularise/modularise/internal/splitapi"
)

func RunSplit(c *config.CLIConfig) error {
	c.Logger.Info("Parsing split configuration.")
	if err := parser.Parse(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	c.Logger.Info("Checking self-contained character of split APIs.")
	if err := splitapi.AnalyseAPI(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	c.Logger.Info("Computing residual packages.")
	if err := residuals.ComputeResiduals(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	c.Logger.Info("Initialising local split repositories.")
	if err := repohandler.InitSplits(c.Logger, &c.Splits); err != nil {
		return err
	}

	c.Logger.Info("Splicing new content.")
	if err := chopper.CleaveSplits(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	c.Logger.Info("Setting up modules in new splices.")
	if err := modworks.CreateSplitModules(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	if c.DryRun {
		c.Logger.Info("Dry-run mode: not pushing new content to remotes.")
		c.Logger.Info("Split content can be found locally in " + c.Splits.WorkTree + ".")
		return nil
	}

	c.Logger.Info("Pushing new split content to remote repositories.")
	if err := repohandler.PushSplits(c.Logger, &c.Splits); err != nil {
		return err
	}
	c.Logger.Info("Split repositories were successfully updated.")
	return nil
}
