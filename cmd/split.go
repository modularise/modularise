package cmd

import (
	"github.com/modularise/modularise/internal/chopper"
	"github.com/modularise/modularise/internal/modworks"
	"github.com/modularise/modularise/internal/parser"
	"github.com/modularise/modularise/internal/repohandler"
	"github.com/modularise/modularise/internal/residuals"
)

func RunSplit(c *CLIConfig) error {
	if err := parser.Parse(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	if err := residuals.ComputeResiduals(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	if err := repohandler.InitSplits(c.Logger, &c.Splits); err != nil {
		return err
	}

	if err := chopper.CleaveSplits(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	if err := modworks.CreateSplitModules(c.Logger, c.Filecache, &c.Splits); err != nil {
		return err
	}

	if err := repohandler.PushSplits(c.Logger, &c.Splits); err != nil {
		return err
	}
	return nil
}
