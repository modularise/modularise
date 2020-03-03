package parser

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
)

func Parse(l *logrus.Logger, fc filecache.FileCache, sp *config.Splits) error {
	if err := parseFiles(l, fc, sp); err != nil {
		return err
	}

	sp.PkgToSplit = map[string]string{}
	sp.PathToSplit = map[string]string{}
	for _, s := range sp.Splits {
		sp.PathToSplit[s.ModulePath] = s.Name

		for f := range s.Files {
			pkg := filepath.Join(fc.ModulePath(), filepath.Dir(f))
			sp.PkgToSplit[pkg] = s.Name
		}
	}
	return nil
}

func parseFiles(l *logrus.Logger, fc filecache.FileCache, sp *config.Splits) error {
	files, err := fc.Files()
	if err != nil {
		return err
	}
	sp.NonModuleSource = !files["go.mod"]

	var mapping prefixMappings
	for n, s := range sp.Splits {
		s.Files = map[string]bool{}

		for j := range s.Includes {
			mapping = append(mapping, prefixMapping{
				prefix: filepath.Clean(s.Includes[j]),
				split:  n,
			})
		}
		for j := range s.Excludes {
			mapping = append(mapping, prefixMapping{
				prefix: filepath.Clean(s.Excludes[j]),
			})
		}
	}
	sort.Sort(mapping)
	l.Debugf("Mappings: %+v\n", mapping)

	for f := range files {
		if s := mapping.matchedSplit(filepath.Dir(f)); s != "" {
			sp.Splits[s].Files[f] = true
			l.Debugf("File %q mapped to split %q.", f, s)
		} else {
			l.Debugf("File %q not mapped to any split.", f)
		}
	}
	return nil
}

func prefixLessThan(rhs, lhs string) int {
	if strings.HasPrefix(rhs, lhs) || strings.HasPrefix(lhs, rhs) {
		if len(rhs) > len(lhs) {
			return -1
		} else if len(rhs) < len(lhs) {
			return 1
		}
		return 0
	}
	return strings.Compare(rhs, lhs)
}

type prefixMapping struct {
	prefix string
	split  string
}

type prefixMappings []prefixMapping

func (m prefixMappings) matchedSplit(p string) string {
	l, t, h := 0, 0, len(m)
	for l != h && l < len(m) {
		t = (l + h) / 2
		switch prefixLessThan(p, m[t].prefix) {
		case -1:
			h = t
		case 0:
			return m[t].split
		case 1:
			l = t + 1
		}
	}
	if h < len(m) && strings.HasPrefix(p, m[h].prefix) {
		return m[h].split
	}
	return ""
}

func (m prefixMappings) Len() int           { return len(m) }
func (m prefixMappings) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m prefixMappings) Less(i, j int) bool { return prefixLessThan(m[i].prefix, m[j].prefix) < 0 }
