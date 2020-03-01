package config

import "github.com/Helcaraxan/modularise/internal/splits"

type Splits struct {
	// Authentication setup to clone / push Git repositories.
	Credentials AuthConfig `yaml:"credentials"`
	// Map of all configured splits.
	Splits map[string]*Split `yaml:"splits"`

	// Internal state.
	splits.DataSplits `yaml:"-"`
}

type Split struct {
	// Module path for the split
	ModulePath string `yaml:"module_path"`
	// List of paths relative to the source module's root. Any Go packages below these paths will be
	// made part of this split, unless:
	// - they are explicitly exluded by a longer prefix path in the Excludes list.
	// - they are explicitly included in another split.
	Includes []string `yaml:"includes"`
	// List of paths relative to the source module's root. Any Go packages below these paths will
	// not be made part of this split, unless they are explicitly included by a longer prefix path
	// in the Includes list.
	Excludes []string `yaml:"excludes"`
	// URL of the Git VCS where this split resides.
	URL string `yaml:"url"`
	// Branch on the remote VCS that should be cloned from / pushed to for split content, defaults to 'master'.
	Branch string `yaml:"branch"`

	// Internal state.
	splits.DataSplit `yaml:"-"`
}
