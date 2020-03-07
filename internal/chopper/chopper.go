package chopper

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
	"go.uber.org/zap"
)

// CleaveSplits will create the content of the configured splits in their respective working
// directories. This includes the rewriting of import paths where needed.
//
// The prequisites on the fields of a config.Splits object for CleaveSplits to be able to operate
// are:
//  - PathToSplit and PkgToSplit have been populated.
//  - WorkTree has been populated and the path in question is an existing directory.
//  - For each config.Split in Splits the Name, Files, Residuals and ResidualFiles fields have been populated.
//  - For each config.Split in Splits the WorkDir field is populated and corrresponds to an existing directory.
func CleaveSplits(log *zap.Logger, fc filecache.FileCache, sp *config.Splits) error {
	for _, s := range sp.Splits {
		s.Root = computeSplitRoot(s.Files)
		log.Debug("Computed root.", zap.String("split", s.Name), zap.String("root", s.Root), zap.Any("files", s.Files))

		if len(s.Residuals) > 0 {
			f := filepath.Join(s.Root, "modularise_dummy.go")
			s.ResidualFiles[f] = true
			s.ResidualsRoot = computeSplitRoot(s.ResidualFiles)
			log.Debug("Computed residuals root.", zap.String("split", s.Name), zap.String("root", s.ResidualsRoot), zap.Any("files", s.ResidualFiles))
			delete(s.ResidualFiles, f)
		}
	}
	for _, s := range sp.Splits {
		c := cleaver{log: log.With(zap.String("split", s.Name)), fc: fc, s: s, sp: sp}
		if err := c.cleaveSplit(); err != nil {
			return err
		}
	}
	return nil
}

// computeSplitRoot will find the longest common prefix path of the supplied set of paths. Paths
// must be filepaths and not directory paths as the last element of each path is stripped of before
// compute the prefix.
func computeSplitRoot(fs map[string]bool) string {
	if len(fs) == 0 {
		return ""
	}

	pkgPath := func(fp string) []string {
		return strings.Split(filepath.Dir(fp), "/")
	}

	var root []string
	for f := range fs {
		if root == nil {
			root = pkgPath(f)
			continue
		}

		next := pkgPath(f)
		if len(root) > len(next) {
			root = root[:len(next)]
		} else {
			next = next[:len(root)]
		}

		var j int
		for j < len(root) {
			if root[j] != next[j] {
				break
			}
			j++
		}
		root = root[:j]
	}
	return filepath.Join(root...)
}

type cleaver struct {
	log *zap.Logger
	fc  filecache.FileCache
	s   *config.Split
	sp  *config.Splits
}

func (c cleaver) cleaveSplit() error {
	c.log.Debug("Cleaving split.")
	for f := range c.s.Files {
		if err := c.copyFileToWorkDir(f, false); err != nil {
			return err
		}
	}
	for f := range c.s.ResidualFiles {
		if err := c.copyFileToWorkDir(f, true); err != nil {
			return err
		}
	}

	if err := c.copyMetafiles(); err != nil {
		return err
	}
	return nil
}

var internalPathRE = regexp.MustCompile(`(^|/)internal($|/)`)

func (c cleaver) copyFileToWorkDir(source string, residual bool) error {
	var target string

	if !residual || (strings.HasPrefix(source, c.s.Root+string(os.PathSeparator)) && !internalPathRE.MatchString(strings.TrimPrefix(source, c.s.Root))) {
		target = strings.TrimPrefix(source, c.s.Root+string(os.PathSeparator))
	} else {
		target = filepath.Join(
			"internal",
			"residuals",
			strings.ReplaceAll(strings.TrimPrefix(source, c.s.ResidualsRoot+string(os.PathSeparator)), "internal"+string(os.PathSeparator), ""),
		)
	}
	target = filepath.Join(c.s.WorkDir, target)

	c.log.Debug("Copying over file.", zap.String("source", source), zap.String("targer", target))

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		c.log.Error("Failed to create a new directory.", zap.String("path", target), zap.Error(err))
		return err
	}

	var content []byte
	if filepath.Ext(source) == ".go" {
		a, fs, err := c.fc.ReadGoFile(source)
		if err != nil {
			return err
		}
		c.rewriteImports(a)
		buf := bytes.Buffer{}
		if err = printer.Fprint(&buf, fs, a); err != nil {
			c.log.Error("Failed to format Go source content.", zap.String("file", target), zap.Error(err))
			return err
		}
		content = buf.Bytes()
	} else if filepath.Base(source) != "go.mod" && filepath.Base(source) != "go.sum" {
		var err error
		content, err = c.fc.ReadFile(source)
		if err != nil {
			return err
		}
	}

	fd, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		c.log.Error("Failed to open file.", zap.String("file", target), zap.Error(err))
		return err
	}
	if _, err = fd.Write(content); err != nil {
		c.log.Error("Failed to write file content.", zap.String("file", target), zap.Error(err))
		return err
	}
	if err = fd.Close(); err != nil {
		c.log.Error("Failed to close file.", zap.String("file", target), zap.Error(err))
		return err
	}
	return nil
}

func (c cleaver) rewriteImports(a *ast.File) {
	for _, imp := range a.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if c.sp.PkgToSplit[p] != "" {
			ts := c.sp.Splits[c.sp.PkgToSplit[p]]
			p = strings.Replace(p, filepath.Join(c.fc.ModulePath(), ts.Root), ts.ModulePath, 1)
		} else if c.s.Residuals[p] {
			rp := strings.Join([]string{c.fc.ModulePath(), c.s.Root, ""}, "/")
			if strings.HasPrefix(p, rp) && internalPathRE.MatchString(strings.TrimPrefix(p, rp)) {
				p = strings.Replace(p, filepath.Join(c.fc.ModulePath(), c.s.Root), c.s.ModulePath, 1)
			} else {
				p = filepath.Join(
					c.s.ModulePath,
					"internal",
					"residuals",
					strings.ReplaceAll(strings.TrimPrefix(p, filepath.Join(c.fc.ModulePath(), c.s.ResidualsRoot)), "/internal", ""),
				)
			}
		}
		c.log.Debug("Rewrote import.", zap.String("old", imp.Path.Value), zap.String("new", p))
		imp.Path.Value = fmt.Sprintf(`"%s"`, p)
	}
}

func (c cleaver) copyMetafiles() error {
	metaFiles := []string{
		"license",
		"license.md",
		"licence",
		"licence.md",
		"LICENSE",
		"LICENSE.md",
		"LICENCE",
		"LICENCE.md",
	}

	fs, err := c.fc.Files()
	if err != nil {
		return err
	}

	for _, fn := range metaFiles {
		if !fs[fn] {
			continue
		}

		var b []byte
		b, err = c.fc.ReadFile(fn)
		if err != nil {
			return err
		}
		p := filepath.Join(c.s.WorkDir, fn)
		if err = ioutil.WriteFile(p, b, 0644); err != nil {
			c.log.Error("Failed to write metadata file.", zap.String("file", p), zap.Error(err))
			return err
		}
	}

	rmc := fmt.Sprintf(splitReadmeTemplate, c.fc.ModulePath())
	p := filepath.Join(c.s.WorkDir, "README.md")
	if err = ioutil.WriteFile(p, []byte(rmc), 0644); err != nil {
		c.log.Error("Failed to write the default README.md file.", zap.String("path", p), zap.Error(err))
		return err
	}
	return nil
}
