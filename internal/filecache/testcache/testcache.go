package testcache

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type FakeFileCacheEntry struct {
	Data []byte
}

type FakeFileCache struct {
	dir         string
	path        string
	fileEntries map[string]FakeFileCacheEntry
}

func NewFakeFileCache(root string, files map[string]FakeFileCacheEntry) (*FakeFileCache, error) {
	goMod, ok := files["go.mod"]
	if !ok {
		return nil, errors.New("no go.mod in specified cache entries")
	}

	m := regexp.MustCompile(`(?m:^module ([^\s]+)$)`).FindSubmatch(goMod.Data)
	if len(m) == 0 {
		return nil, errors.New("no module path found in go.mod")
	}

	ifs := map[string]FakeFileCacheEntry{}
	for f, e := range files {
		if strings.HasPrefix(f, ".git"+string(os.PathSeparator)) {
			continue
		}
		ifs[f] = e
	}

	return &FakeFileCache{
		dir:         root,
		path:        string(m[1]),
		fileEntries: ifs,
	}, nil
}

func (c FakeFileCache) Root() string {
	return c.dir
}

func (c FakeFileCache) ModulePath() string {
	return c.path
}

func (c FakeFileCache) Pkgs() (map[string]bool, error) {
	pkgs := map[string]bool{}
	for relPkg := range relativePkgs(c.fileEntries) {
		pkgs[filepath.Join(c.path, relPkg)] = true
	}
	return pkgs, nil
}

func (c FakeFileCache) Files() (map[string]bool, error) {
	fs := map[string]bool{}
	for p := range c.fileEntries {
		fs[p] = true
	}
	return fs, nil
}

func (c FakeFileCache) FilesInPkg(pkg string) (map[string]bool, error) {
	pkgs, _ := c.Pkgs()
	if !pkgs[pkg] {
		return nil, fmt.Errorf("package %q is not part of module %q", pkg, c.path)
	}
	fs := map[string]bool{}
	for f := range c.fileEntries {
		if filepath.Join(c.path, filepath.Dir(f)) == pkg {
			fs[f] = true
		}
	}
	return fs, nil
}

func (c FakeFileCache) ReadFile(path string) ([]byte, error) {
	path = filepath.Clean(path)

	f, ok := c.fileEntries[path]
	if !ok {
		return nil, fmt.Errorf("file %q is not part of the module", path)
	}
	cd := make([]byte, len(f.Data))
	copy(cd, f.Data)
	return cd, nil
}

func (c *FakeFileCache) ReadGoFile(path string) (*ast.File, *token.FileSet, error) {
	path = filepath.Clean(path)

	f, ok := c.fileEntries[path]
	if !ok {
		return nil, nil, fmt.Errorf("file %q is not part of the module", path)
	} else if filepath.Ext(path) != ".go" {
		return nil, nil, fmt.Errorf("%s is not a go file", path)
	}
	fs := token.NewFileSet()
	a, err := parser.ParseFile(fs, "", f.Data, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	return a, fs, nil
}

func relativePkgs(files map[string]FakeFileCacheEntry) map[string]bool {
	pkgs := map[string]bool{}
	for f := range files {
		if filepath.Ext(f) != ".go" {
			continue
		}
		pkgs[filepath.Dir(f)] = true
	}
	return pkgs
}
