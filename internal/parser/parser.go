package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
)

// Parse iterates over the configured splits and populates information about their contents. This
// mostly covers files and Go packages
func Parse(l *zap.Logger, fc filecache.FileCache, sp *config.Splits) error {
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

func parseFiles(l *zap.Logger, fc filecache.FileCache, sp *config.Splits) error {
	sp.NonModuleSource = !fc.Files()["go.mod"]

	var mapping prefixMappings
	for n, s := range sp.Splits {
		s.Files = map[string]bool{}

		for j := range s.Includes {
			mapping = append(mapping, prefixMapping{
				prefix: filepath.Clean(s.Includes[j]) + string(os.PathSeparator),
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

	var nonMatched []string
	for f := range fc.Files() {
		if s := mapping.matchedSplit(filepath.Dir(f) + string(os.PathSeparator)); s != "" {
			sp.Splits[s].Files[f] = true
		} else {
			nonMatched = append(nonMatched, f)
		}
	}

	// This computation and logging sequence might be expensive for large projects hence we guard
	// this with an explicit logger-level check.
	if l.Core().Enabled(zapcore.DebugLevel) {
		var matchDebug []string
		for _, m := range mapping {
			matchDebug = append(matchDebug, fmt.Sprintf("%s => %s", m.prefix, m.split))
		}
		l.Debug("Split path matches.", zap.Strings("mapping", matchDebug))
		sort.Strings(nonMatched)
		l.Debug("Non-matched files.", zap.Strings("files", nonMatched))
		for _, s := range sp.Splits {
			l.Debug("Computed files of split.", zap.String("split", s.Name), zap.Any("files", s.Files))
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

// We use this custom prefixMapping datastructure to infer the appropriate mapping from a given
// filepath to the corresponding split, if such a split exists. The algorithm that is used is:
//  - For each 'include' create a prefixMapping to the including split's name.
//  - For each 'exclude' create a prefixMapping to an empty string.
//  - Sort the obtained slice of prefixMapping structs in alphabetical order, in the case of one
//    string being a prefix of another, the longer string is sorted first.
//  - In order to match a filepath to a split compute the theoretical index in the list where the
//    filepath would be inserted. The current prefixMapping at that index indicates the split to
//    which the filepath should be mapped. If the prefixMapping indicates an empty string the
//    filepath does not belong to any string.
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
