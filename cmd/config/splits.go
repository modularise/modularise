package config

import "github.com/modularise/modularise/internal/splits"

type Splits struct {
	// Authentication setup to clone / push Git repositories.
	Credentials AuthConfig `yaml:"credentials,omitempty"`
	// Map of all configured splits.
	Splits map[string]*Split `yaml:"splits,omitempty"`

	// Internal state.
	splits.DataSplits `yaml:"-"`
}

type Split struct {
	// Module path for the split
	ModulePath string `yaml:"module_path,omitempty"`
	// List of paths relative to the source module's root. Any Go packages below these paths will be
	// made part of this split, unless:
	// - they are explicitly exluded by a longer prefix path in the Excludes list.
	// - they are explicitly included in another split.
	Includes []string `yaml:"includes,omitempty"`
	// List of paths relative to the source module's root. Any Go packages below these paths will
	// not be made part of this split, unless they are explicitly included by a longer prefix path
	// in the Includes list.
	Excludes []string `yaml:"excludes,omitempty"`
	// URL of the Git VCS where this split resides.
	URL string `yaml:"url,omitempty"`
	// Branch on the remote VCS that should be cloned from / pushed to for split content, defaults to 'master'.
	Branch string `yaml:"branch,omitempty"`

	// Internal state.
	splits.DataSplit `yaml:"-"`
}
