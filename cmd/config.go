package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/modularise/modularise/internal/filecache"
	"github.com/modularise/modularise/internal/filecache/uncache"
	"github.com/modularise/modularise/internal/splits"
)

type CLIConfig struct {
	// Path to the configuration file to use. If empty this will default to a 'modularise.yaml'
	// file located at the root of the Go module from which the command is invoked.
	ConfigFile string
	// File to which to write execution logs. If empty logs will be written to the standard output
	// of the command invocation.
	LogFile string
	// Directory to which to write all split content.
	WorkDirectory string
	// If set do not push new split content to the associated remotes.
	DryRun bool
	// If set emit verbose debug logs.
	Verbose bool

	// Internal state.
	cliConfigData
}

type cliConfigData struct {
	Logger    *logrus.Logger
	Filecache filecache.FileCache
	Splits    splits.Splits
}

func (c *CLIConfig) CheckConfig() error {
	// Logger needs to be checked first as anything after this point might write logs.
	if err := c.checkLogger(); err != nil {
		return err
	}

	if err := c.checkConfigFile(); err != nil {
		return err
	}

	if c.WorkDirectory != "" {
		c.Splits.WorkTree = c.WorkDirectory
	}
	for n, s := range c.Splits.Splits {
		s.Name = n
	}

	fc, err := uncache.NewUncache(c.Logger, filepath.Dir(c.ConfigFile))
	if err != nil {
		return err
	}
	c.Filecache = fc

	return nil
}

func (c *CLIConfig) checkLogger() error {
	if c.Logger != nil {
		return nil
	}

	c.Logger = logrus.New()
	if c.Verbose {
		c.Logger.SetLevel(logrus.DebugLevel)
		c.Logger.Debug("Enabling debug logging because the '-v | --verbose' flag was set.")
	} else if _, ok := os.LookupEnv("DEBUG"); ok {
		c.Logger.SetLevel(logrus.DebugLevel)
		c.Logger.Debug("Enabling debug logging because the DEBUG environment variable is set.")
	} else if val, ok := os.LookupEnv("LOG_LEVEL"); ok {
		lvl, err := logrus.ParseLevel(val)
		if err != nil {
			c.Logger.WithError(err).Errorf("Could not parse value of LOG_LEVEL environment variable (%s) as a valid log level.", val)
			return err
		}
		c.Logger.SetLevel(lvl)
	}

	c.Logger.Debugf("Further log output will be written to %q.", c.LogFile)
	if c.LogFile != "" {
		f, err := os.OpenFile(c.LogFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0644)
		if err != nil {
			c.Logger.WithError(err).Errorf("Failed to open %q to write log output.", c.LogFile)
			return err
		}
		c.Logger.SetOutput(f)
	}
	return nil
}

func (c *CLIConfig) checkConfigFile() error {
	if c.ConfigFile == "" {
		if err := c.findConfigFile(); err != nil {
			return err
		}
	}

	c.Logger.Debugf("Reading configuration file %q.", c.ConfigFile)
	cb, err := ioutil.ReadFile(c.ConfigFile)
	if err != nil {
		c.Logger.WithError(err).Errorf("Unable to read content of configuration file %q.", c.ConfigFile)
		return err
	}

	c.Logger.Debugf("Parsing configuration file content:\n%s", cb)
	if err = yaml.Unmarshal(cb, &c.Splits); err != nil {
		c.Logger.WithError(err).Errorf("Unable to parse configuration file %q.", c.ConfigFile)
		return err
	}

	if len(c.Splits.Splits) == 0 {
		c.Logger.Errorf("No splits were found in configuration file %q.", c.ConfigFile)
		return fmt.Errorf("%q does not contain any configured splits", c.ConfigFile)
	}
	return nil
}

func (c *CLIConfig) findConfigFile() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.Logger.Debug("Running 'go list -m -json' to determine the current module root for the default configuration file location.")
	out, err := exec.CommandContext(ctx, "go", "list", "-m", "-json").CombinedOutput()
	if err != nil {
		c.Logger.WithError(err).Error("Could not determine Go module for the current directory. Are you sure you are inside the target module?")
		return fmt.Errorf("failed to run 'go list -m -json': %v\noutput was:\n%s", err, out)
	}

	c.Logger.Debugf("'go list -m -json' returned:\n%s", out)
	mi := uncache.ModuleInfo{}
	if err = json.Unmarshal(out, &mi); err != nil {
		c.Logger.WithError(err).Errorf("Could not parse result of 'go list -m -json': %s", out)
		return err
	}
	p := filepath.Join(mi.Dir, "modularise.yaml")

	info, err := os.Stat(p)
	if err != nil && !os.IsNotExist(err) {
		c.Logger.WithError(err).Errorf("Encountered an unexpected error while testing existence of %q.", p)
		return err
	} else if os.IsNotExist(err) || info.IsDir() {
		c.Logger.Errorf("No configuration file was found at %q.", p)
	}

	c.ConfigFile = p
	c.Logger.Infof("Using Modularise configuration file at default location %q.", p)
	return nil
}
