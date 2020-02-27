package uncache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

type ModuleInfo struct {
	Dir  string `json:"dir"`
	Path string `json:"path"`
}

func NewUncache(log *logrus.Logger, root string) (*Uncache, error) {
	const nonModuleListErr = "go list -m: not using modules"

	var err error
	if root, err = filepath.Abs(root); err != nil {
		log.WithError(err).Errorf("Unable to determine the absolute path to the root of the filecache %q.", root)
		return nil, err
	}

	eb, ob := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := exec.Command("go", "list", "-m", "-json")
	cmd.Env = append(
		os.Environ(),
		"GO111MODULE=auto", // This is to ensure we fail when running in non-module directories.
	)
	cmd.Dir = root
	cmd.Stderr = eb
	cmd.Stdout = ob
	if err = cmd.Run(); err != nil {
		if !strings.Contains(eb.String(), nonModuleListErr) {
			log.WithError(err).Errorf("Unable to run 'go list -m -json' in directory %q:\n%s", root, eb.String())
			return nil, errors.New("go list error")
		}

		// We're in a non-module Go project.
		gp, ok := os.LookupEnv("GOPATH")
		gp = filepath.Join(gp, "src") + string(os.PathSeparator)
		if !ok || !strings.HasPrefix(root, gp) {
			log.Errorf("The provided root for the filecache %q is not inside the configured GOPATH %q.", root, gp)
			return nil, errors.New("provided root not part of GOPATH")
		}

		return &Uncache{
			log:  log,
			root: root,
			path: filepath.ToSlash(strings.TrimPrefix(root, gp)),
		}, nil
	}

	var mi ModuleInfo
	if err = json.Unmarshal(ob.Bytes(), &mi); err != nil {
		log.WithError(err).Errorf("Unexpected output from 'go list -m -json' in directory %q:\n%s", root, ob.String())
		return nil, errors.New("go list error")
	}

	return &Uncache{
		log:  log,
		root: mi.Dir,
		path: mi.Path,
	}, nil
}

type Uncache struct {
	log   *logrus.Logger
	root  string
	path  string
	files map[string]bool
	pkgs  map[string]bool
}

func (c Uncache) Root() string {
	return c.root
}

func (c Uncache) ModulePath() string {
	return c.path
}

func (c *Uncache) Pkgs() (map[string]bool, error) {
	if err := c.populateFilesAndPkgs(); err != nil {
		c.log.WithError(err).Errorf("Failed to initialise file and package data for uncache rooted at %q.", c.root)
		return nil, err
	}
	return c.pkgs, nil
}

func (c *Uncache) Files() (map[string]bool, error) {
	if err := c.populateFilesAndPkgs(); err != nil {
		c.log.WithError(err).Errorf("Failed to initialise file and package data for uncache rooted at %q.", c.root)
		return nil, err
	}
	return c.files, nil
}

func (c *Uncache) FilesInPkg(pkg string) (map[string]bool, error) {
	if err := c.populateFilesAndPkgs(); err != nil {
		c.log.WithError(err).Errorf("Failed to initialise file and package data for uncache rooted at %q.", c.root)
		return nil, err
	}
	if !c.pkgs[pkg] {
		c.log.Errorf("Supplied package %q is not part of module %q abstracted by this filecache.", pkg, c.path)
		return nil, fmt.Errorf("package %q is not part of module %q", pkg, c.path)
	}
	fs := map[string]bool{}
	for f := range c.files {
		if filepath.Join(c.path, filepath.Dir(f)) == pkg {
			fs[f] = true
		}
	}
	return fs, nil
}

func (c *Uncache) ReadFile(path string) ([]byte, error) {
	if err := c.populateFilesAndPkgs(); err != nil {
		c.log.WithError(err).Errorf("Failed to initialise file and package data for uncache rooted at %q.", c.root)
		return nil, err
	}

	path = filepath.Clean(path)
	if !c.files[path] {
		c.log.Errorf("File %q does not exist or is not part of module %q.", path, c.path)
		return nil, fmt.Errorf("could not access %s", path)
	}
	return ioutil.ReadFile(filepath.Join(c.root, path))
}

func (c *Uncache) ReadGoFile(path string) (*ast.File, *token.FileSet, error) {
	if err := c.populateFilesAndPkgs(); err != nil {
		c.log.WithError(err).Errorf("Failed to initialise file and package data for uncache rooted at %q.", c.root)
		return nil, nil, err
	}

	path = filepath.Clean(path)
	if !c.files[path] {
		c.log.Errorf("File %q does not exist or is not part of module %q.", path, c.path)
		return nil, nil, fmt.Errorf("could not access %s", path)
	}

	if filepath.Ext(path) != ".go" {
		c.log.Errorf("File %q is not a Go file", path)
		return nil, nil, fmt.Errorf("%s is not a go file", path)
	}

	fset := token.NewFileSet()
	a, err := parser.ParseFile(fset, filepath.Join(c.root, path), nil, parser.AllErrors|parser.ParseComments)
	return a, fset, err
}

func (c *Uncache) populateFilesAndPkgs() (err error) {
	if c.files != nil && c.pkgs != nil {
		return nil
	}

	files := map[string]bool{}
	pkgs := map[string]bool{}
	err = filepath.Walk(c.root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			c.log.WithError(err).Errorf("Failed to walk sub-directories of %q.", c.root)
			return err
		}

		if fi.IsDir() {
			if path == c.root {
				return nil
			} else if filepath.Base(path) == ".git" {
				return filepath.SkipDir
			}
			if _, err = os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			} else if !os.IsNotExist(err) {
				c.log.WithError(err).Errorf("Could not gather information about a potential go.mod file at %q.", filepath.Join(path, "go.mod"))
				return err
			}
			return nil
		}

		files[strings.TrimPrefix(path, c.root+"/")] = true
		if filepath.Base(path) != "go.mod" && filepath.Ext(path) == ".go" {
			pkgs[strings.Replace(filepath.Dir(path), c.root, c.path, 1)] = true
		}
		return nil
	})
	if err != nil {
		return err
	}

	c.files = files
	c.pkgs = pkgs
	return nil
}
