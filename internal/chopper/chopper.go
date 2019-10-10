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

	"github.com/sirupsen/logrus"

	"github.com/Helcaraxan/modularise/internal/filecache"
	"github.com/Helcaraxan/modularise/internal/splits"
)

func CleaveSplits(log *logrus.Logger, fc filecache.FileCache, sp *splits.Splits) error {
	for _, s := range sp.Splits {
		s.Root = computeSplitRoot(s.Files)
		log.Debugf("Computed root %q for split %q containing files %v.", s.Root, s.Name, s.Files)

		if len(s.Residuals) > 0 {
			f := filepath.Join(s.Root, "modularise_dummy.go")
			s.ResidualFiles[f] = true
			s.ResidualsRoot = computeSplitRoot(s.ResidualFiles)
			log.Debugf("Computed residuals root %q for split %q using residual files %v.", s.ResidualsRoot, s.Name, s.ResidualFiles)
			delete(s.ResidualFiles, f)
		}
	}
	for _, s := range sp.Splits {
		c := cleaver{log: log, fc: fc, s: s, sp: sp}
		if err := c.cleaveSplit(); err != nil {
			return err
		}
	}
	return nil
}

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
	log *logrus.Logger
	fc  filecache.FileCache
	s   *splits.Split
	sp  *splits.Splits
}

func (c cleaver) cleaveSplit() error {
	c.log.Debugf("Cleaving split %q with files %v and residuals %v.", c.s.Name, c.s.Files, c.s.Residuals)
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

	c.log.Debugf("Copying over %q for split %q to %q.", source, c.s.Name, target)

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		c.log.WithError(err).Errorf("Failed to create a new directory %q in split %q.", target, c.s.Name)
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
			c.log.WithError(err).Errorf("Failed to format the content for target file %q in split %q.", target, c.s.Name)
			return err
		}
		content = buf.Bytes()
	} else {
		var err error
		content, err = c.fc.ReadFile(source)
		if err != nil {
			return err
		}
	}

	fd, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		c.log.WithError(err).Errorf("Failed to open file %q in split %q.", target, c.s.Name)
		return err
	}
	if _, err = fd.Write(content); err != nil {
		c.log.WithError(err).Errorf("Failed to write content to file %q in split %q.", target, c.s.Name)
		return err
	}
	if err = fd.Close(); err != nil {
		c.log.WithError(err).Errorf("Failed to close file %q in split %q after writing its content.", target, c.s.Name)
		return err
	}
	return nil
}

func (c cleaver) rewriteImports(a *ast.File) {
	for _, imp := range a.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if c.sp.PkgToSplit[p] != nil {
			p = strings.Replace(p, filepath.Join(c.fc.ModulePath(), c.sp.PkgToSplit[p].Root), c.sp.PkgToSplit[p].ModulePath, 1)
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
		c.log.Debugf("Rewrote import %s to %q.", imp.Path.Value, p)
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
		if err = ioutil.WriteFile(filepath.Join(c.s.WorkDir, fn), b, 0644); err != nil {
			c.log.WithError(err).Errorf("Failed to write metadata file %q to split %q in %q.", fn, c.s.Name, c.s.WorkDir)
			return err
		}
	}

	rmc := fmt.Sprintf(splitReadmeTemplate, c.fc.ModulePath())
	if err = ioutil.WriteFile(filepath.Join(c.s.WorkDir, "README.md"), []byte(rmc), 0644); err != nil {
		c.log.WithError(err).Errorf("Failed to write the default README.md file to split %q in %q.", c.s.Name, c.s.WorkDir)
		return err
	}
	return nil
}
