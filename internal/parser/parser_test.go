package parser

import (
	"testing"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/testlib"
)

func TestUnit_ParseUnit(t *testing.T) {
	t.Parallel()

	goMod := testcache.FakeFileCacheEntry{Data: []byte("module example.com/mod")}

	tcs := map[string]struct {
		files    map[string]testcache.FakeFileCacheEntry
		splits   config.Splits
		expected map[string]map[string]bool
	}{
		"NoSplit": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod": goMod,
				"one.go": {},
			},
			splits:   config.Splits{Splits: map[string]*config.Split{}},
			expected: map[string]map[string]bool{},
		},
		"OneSplitOneFile": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":     goMod,
				"one/one.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {Includes: []string{"one"}},
			}},
			expected: map[string]map[string]bool{
				"one": {"one/one.go": true},
			},
		},
		"OneSplitOneFileNested": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":            goMod,
				"one/nested/one.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {Includes: []string{"one"}},
			}},
			expected: map[string]map[string]bool{
				"one": {"one/nested/one.go": true},
			},
		},
		"OneSplitOneIdenticallyNamedFile": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod": goMod,
				"one.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {Includes: []string{"one"}},
			}},
			expected: map[string]map[string]bool{
				"one": {},
			},
		},
		"OneSplitOneFileOneOutsideOneIgnored": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":                 goMod,
				"ignored.go":             {},
				"one/one.go":             {},
				"one/ignored/ignored.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {
					Includes: []string{"one"},
					Excludes: []string{"one/ignored"},
				},
			}},
			expected: map[string]map[string]bool{
				"one": {"one/one.go": true},
			},
		},
		"TwoNestedSplitsSimple": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":         goMod,
				"one/one.go":     {},
				"one/two/two.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {Includes: []string{"one"}},
				"two": {Includes: []string{"one/two"}},
			}},
			expected: map[string]map[string]bool{
				"one": {"one/one.go": true},
				"two": {"one/two/two.go": true},
			},
		},
		"TwoNestedSplitsComplex": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":             goMod,
				"one/one.go":         {},
				"one/two/two.go":     {},
				"one/two/one/one.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {Includes: []string{
					"one",
					"one/two/one",
				}},
				"two": {Includes: []string{"one/two"}},
			}},
			expected: map[string]map[string]bool{
				"one": {
					"one/one.go":         true,
					"one/two/one/one.go": true,
				},
				"two": {"one/two/two.go": true},
			},
		},
		"TwoSplitsWithOverlappingPaths": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod": goMod,
				// The files need to be nested as the matcher removes the last part of the to-be-matched path.
				"one/lib/one.go":    {},
				"onetwo/lib/two.go": {},
			},
			splits: config.Splits{Splits: map[string]*config.Split{
				"one": {Includes: []string{"one"}},
				"two": {Includes: []string{"onetwo"}},
			}},
			expected: map[string]map[string]bool{
				"one": {"one/lib/one.go": true},
				"two": {"onetwo/lib/two.go": true},
			},
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			fc, err := testcache.NewFakeFileCache("fake-cache-dir", tc.files)
			testlib.NoError(t, true, err)

			err = Parse(testlib.NewTestLogger(), fc, &tc.splits)
			testlib.NoError(t, true, err)

			testlib.True(t, true, len(tc.splits.Splits) == len(tc.expected))
			for s, e := range tc.expected {
				testlib.Equal(t, false, e, tc.splits.Splits[s].Files)
			}
		})
	}
}
