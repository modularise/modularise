package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"

	"github.com/modularise/modularise/internal/filecache"
	"github.com/modularise/modularise/internal/filecache/cache"
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
	Logger    *zap.Logger
	Filecache filecache.FileCache
	Splits    Splits
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

	fc, err := cache.NewCache(c.Logger, filepath.Dir(c.ConfigFile))
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

	var enc zapcore.Encoder
	var level zapcore.LevelEnabler
	if c.Verbose {
		enc = zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
		level = zapcore.DebugLevel
	} else {
		enc = zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig())
		level = zapcore.InfoLevel
	}

	var out zapcore.WriteSyncer
	if c.LogFile != "" {
		f, err := os.OpenFile(c.LogFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		out = f
	} else {
		out = os.Stdout
	}

	c.Logger = zap.New(zapcore.NewCore(enc, out, level))
	return nil
}

func (c *CLIConfig) checkConfigFile() error {
	if c.ConfigFile == "" {
		if err := c.findConfigFile(); err != nil {
			return err
		}
	}

	c.Logger.Debug("Reading configuration file.", zap.String("file", c.ConfigFile))
	cb, err := ioutil.ReadFile(c.ConfigFile)
	if err != nil {
		c.Logger.Error("Unable to read content of configuration file %q.", zap.String("file", c.ConfigFile), zap.Error(err))
		return err
	}

	c.Logger.Debug("Parsing configuration file content.", zap.String("file", c.ConfigFile), zap.ByteString("configuration", cb))
	if err = yaml.Unmarshal(cb, &c.Splits); err != nil {
		c.Logger.Error("Unable to parse configuration file.", zap.String("file", c.ConfigFile), zap.Error(err))
		return err
	}

	if len(c.Splits.Splits) == 0 {
		c.Logger.Error("No splits were found in configuration file.", zap.String("file", c.ConfigFile), zap.Error(err))
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
		c.Logger.Error("Could not determine Go module for the current directory. Are you sure you are inside the target module?", zap.Error(err))
		return fmt.Errorf("failed to run 'go list -m -json': %v\noutput was:\n%s", err, out)
	}

	c.Logger.Debug("'go list -m -json' returned.", zap.ByteString("output", out))
	mi := cache.ModuleInfo{}
	if err = json.Unmarshal(out, &mi); err != nil {
		c.Logger.Error("Could not parse result of 'go list -m -json': %s", zap.ByteString("output", out), zap.Error(err))
		return err
	}
	p := filepath.Join(mi.Dir, "modularise.yaml")

	info, err := os.Stat(p)
	if err != nil && !os.IsNotExist(err) {
		c.Logger.Error("Encountered an unexpected error while testing file existence.", zap.String("path", p), zap.Error(err))
		return err
	} else if os.IsNotExist(err) || info.IsDir() {
		c.Logger.Error("No configuration file was found.", zap.String("path", p))
	}

	c.ConfigFile = p
	c.Logger.Info("Using Modularise configuration file at default location.", zap.String("path", p))
	return nil
}
