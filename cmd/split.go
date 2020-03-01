package cmd

import (
	"github.com/Helcaraxan/modularise/cmd/config"
	"github.com/Helcaraxan/modularise/internal/chopper"
	"github.com/Helcaraxan/modularise/internal/modworks"
	"github.com/Helcaraxan/modularise/internal/parser"
	"github.com/Helcaraxan/modularise/internal/repohandler"
	"github.com/Helcaraxan/modularise/internal/residuals"
)

func RunSplit(c *config.CLIConfig) error {
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

	if c.DryRun {
		return nil
	}

	if err := repohandler.PushSplits(c.Logger, &c.Splits); err != nil {
		return err
	}
	return nil
}
