package splits

import "gopkg.in/src-d/go-git.v4"

type Splits struct {
	// Map of all configured splits.
	Splits map[string]*Split `yaml:"splits"`

	// Internal state.
	DataSplits
}

type Split struct {
	// Module path for the split
	ModulePath string
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
	URL string
	// Branch on the remote VCS that should be cloned from / pushed to for split content.
	Branch string

	// Internal state.
	DataSplit
}

// DataSplits contains information that is not part of the configuration of the splits but which is
// filled in and used throughout the processing of the source code.
type DataSplits struct {
	// Filepath to split mapping.
	PathToSplit map[string]*Split
	// Go package name to split mapping.
	PkgToSplit map[string]*Split
	// Directory under which all split work will be done and stored.
	WorkTree string
}

// splitData contains information that is not part of the configuration of a split but which is
// filled in and used throughout the processing of the source code.
type DataSplit struct {
	// Name of the split
	Name string
	// Set of the paths of all files that are directly included in the split relative to the source
	// module's root. This does not include files that are part of any residuals of the split.
	Files map[string]bool
	// Residual Go packages for this split. These are packages imported by the Go packages
	// explicitly included in the split via its Includes and Excludes but are not explicitly
	// included themselves in any split. Residual packages are not allowed to be referenced as part
	// of any exported symbol of the split's packages.
	Residuals map[string]bool
	// Set of the paths of all files that are part of any residuals of the split.
	ResidualFiles map[string]bool
	// New virtual root relative to the root of the source module for packages part of the split's module.
	Root string
	// New virtual root relative to the root of the source module for residual packages of the split's module.
	ResidualsRoot string
	// Name of splits of which this split imports Go packages.
	SplitDeps map[string]bool
	// New pseudo-version for the content of this split.
	Version string
	// Folder to which the content of this split is written.
	WorkDir string
	// Git repository stored inside WorkDir.
	Repo *git.Repository
}
