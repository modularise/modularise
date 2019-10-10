package filecache

import (
	"go/ast"
	"go/token"

	"github.com/Helcaraxan/modularise/internal/filecache/testcache"
	"github.com/Helcaraxan/modularise/internal/filecache/uncache"
)

// Ensure that we implement the required interface.
var (
	_ FileCache = &testcache.FakeFileCache{}
	_ FileCache = &uncache.Uncache{}
)

type Type uint8

const (
	Unknown Type = iota
	Uncache
	TestCache
)

// FileCache represents an abstraction for read-only access to the files and information of a Go
// module.
type FileCache interface {
	// Absolute path to the root of the Go module abstracted by this filecache.
	Root() string
	// Module path for the Go module abstracted by this filecache.
	ModulePath() string
	// Set of all the Go packages contained within the module abstracted by this filecache.
	Pkgs() (map[string]bool, error)
	// Set of all Go and non-Go files contained within the module abstracted by this filecache. The
	// retured paths are all relative to the module's root.
	Files() (map[string]bool, error)
	// Set of all Go and non-Go files contained within the specified Go package. The package must be
	// part of the module abstracted by this filecache. The returned paths are all relative to the
	// module's root.
	FilesInPkg(pkg string) (map[string]bool, error)

	// Retrieve the content of an arbitrary file if it exists within the module abstracted by this
	// filecache. The path argument is interpreted as relative to the root of the module.
	ReadFile(path string) ([]byte, error)
	// Retrieve the parsed data of a Go file if it exists within the module abstracted by this
	// filecache. The path argument is interpreted as relative to the root of the module. The
	// returned ast.File object may be modified and tweaked without it affecting the result of any
	// subsequent calls to ReadGoFile for the same path.
	ReadGoFile(path string) (*ast.File, *token.FileSet, error)
}
