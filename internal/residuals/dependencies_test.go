package residuals

import (
	"testing"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
)

func TestResolveImportsAndResiduals(t *testing.T) {
	t.Parallel()

	const (
		depSplitA = "a"
		depSplitB = "b"
	)

	tcs := map[string]struct {
		files                 map[string]testcache.FakeFileCacheEntry
		pkgToSplit            map[string]string
		expectedResiduals     map[string]bool
		expectedResidualFiles map[string]bool
		expectedSplitDeps     map[string]bool
	}{
		"NoImports": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":  {Data: []byte("module example.com/repo\n")},
				"file.go": {Data: []byte("package repo\n")},
			},
			pkgToSplit: map[string]string{
				"example.com/repo": depSplitA,
			},
			expectedResiduals:     map[string]bool{},
			expectedResidualFiles: map[string]bool{},
			expectedSplitDeps:     map[string]bool{},
		},
		"ThirdPartyImports": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":  {Data: []byte("module example.com/repo\n")},
				"file.go": {Data: []byte("package repo\n\nimport \"third-party.com/module\"\n")},
			},
			pkgToSplit: map[string]string{
				"example.com/repo": depSplitA,
			},
			expectedResiduals:     map[string]bool{},
			expectedResidualFiles: map[string]bool{},
			expectedSplitDeps:     map[string]bool{},
		},
		"NoResiduals": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":      {Data: []byte("module example.com/repo\n")},
				"file.go":     {Data: []byte("package repo\n\nimport \"example.com/repo/lib\"\n")},
				"lib/file.go": {Data: []byte("package lib\n")},
			},
			pkgToSplit: map[string]string{
				"example.com/repo":     depSplitA,
				"example.com/repo/lib": depSplitA,
			},
			expectedResiduals:     map[string]bool{},
			expectedResidualFiles: map[string]bool{},
			expectedSplitDeps:     map[string]bool{},
		},
		"Residuals": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":      {Data: []byte("module example.com/repo\n")},
				"file.go":     {Data: []byte("package repo\n\nimport \"example.com/repo/lib\"\n")},
				"lib/file.go": {Data: []byte("package lib\n")},
			},
			pkgToSplit: map[string]string{
				"example.com/repo": depSplitA,
			},
			expectedResiduals: map[string]bool{
				"example.com/repo/lib": true,
			},
			expectedResidualFiles: map[string]bool{
				"lib/file.go": true,
			},
			expectedSplitDeps: map[string]bool{},
		},
		"IndirectResiduals": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":       {Data: []byte("module example.com/repo\n")},
				"file.go":      {Data: []byte("package repo\n\nimport \"example.com/repo/lib\"\n")},
				"lib/file.go":  {Data: []byte("package lib\n\nimport \"example.com/repo/util\"\n")},
				"util/file.go": {Data: []byte("package util\n")},
			},
			pkgToSplit: map[string]string{
				"example.com/repo": depSplitA,
			},
			expectedResiduals: map[string]bool{
				"example.com/repo/lib":  true,
				"example.com/repo/util": true,
			},
			expectedResidualFiles: map[string]bool{
				"lib/file.go":  true,
				"util/file.go": true,
			},
			expectedSplitDeps: map[string]bool{},
		},
		"SplitDeps": {
			files: map[string]testcache.FakeFileCacheEntry{
				"go.mod":      {Data: []byte("module example.com/repo\n")},
				"file.go":     {Data: []byte("package repo\n\nimport \"example.com/repo/lib\"\n")},
				"lib/file.go": {Data: []byte("package lib\n")},
			},
			pkgToSplit: map[string]string{
				"example.com/repo":     depSplitA,
				"example.com/repo/lib": depSplitB,
			},
			expectedResiduals:     map[string]bool{},
			expectedResidualFiles: map[string]bool{},
			expectedSplitDeps: map[string]bool{
				depSplitB: true,
			},
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			fc, err := testcache.NewFakeFileCache("", tc.files)
			testlib.NoError(t, true, err)

			sp := &config.Splits{DataSplits: splits.DataSplits{PkgToSplit: tc.pkgToSplit}}
			s := &config.Split{DataSplit: splits.DataSplit{
				Name:  depSplitA,
				Files: map[string]bool{"file.go": true},
			}}

			err = computeDependencies(testlib.NewTestLogger(), fc, sp, s)
			testlib.NoError(t, true, err)
			testlib.Equal(t, false, tc.expectedResidualFiles, s.ResidualFiles)
			testlib.Equal(t, false, tc.expectedResiduals, s.Residuals)
			testlib.Equal(t, false, tc.expectedSplitDeps, s.SplitDeps)
		})
	}
}
