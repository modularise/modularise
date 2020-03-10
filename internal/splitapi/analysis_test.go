package splitapi

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
)

func TestFile(t *testing.T) {
	t.Parallel()

	const (
		testPkg   = "example.com/pkg"
		testSplit = "test-split"
	)
	depSplit := &config.Split{DataSplit: splits.DataSplit{Name: "split"}}

	tcs := map[string]struct {
		in         string
		pkgTosplit map[string]string
		errs       []residualError
	}{
		"InterfaceType": {
			in: `package test

type MyInterface interface {
	LocalMethod(LocalType) (LocalType, error)
	ExternalMethod(pkg.ExternalType) (pkg.ExternalType, error)
}`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"InterfaceTypeWithEmbedding": {
			in: `package test

type MyInterface interface {
	pkg.ExternalType

	LocalMethod(LocalType) (LocalType, error)
}`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"StructType": {
			in: `package test

type MyStruct struct {
	LocalField LocalType
	ExternalField pkg.ExternalType
}`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"StructTypeWithEmbedding": {
			in: `package test

type MyStruct struct {
	pkg.ExternalType

	LocalField LocalType
}`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"UnexportedFunc": {
			in: `package test

func unexportedFunc(_ pkg.ExternalType) {}
`,
		},
		"ExportedFunc": {
			in: `package test

func ExportedFunc(_ pkg.ExternalType) {}
`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"ExportedFuncNoSplit": {
			in: `package test

func ExportedFunc(_ pkg.ExternalType) {}
`,
			errs: []residualError{&nonSplitImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.ExternalType", Loc: "3:21"}},
		},
		"TypeRedeclaration": {
			in: `package test

type LocalType pkg.ExportedType
`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"TypeRedeclarationNonSplit": {
			in: `package test

type LocalType pkg.ExportedType
`,
			errs: []residualError{&nonSplitImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.ExportedType", Loc: "3:16"}},
		},
		"TypeAlias": {
			in: `package test

type LocalType = pkg.ExportedType
`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"TypeAliasNonSplit": {
			in: `package test

type LocalType = pkg.ExportedType
`,
			errs: []residualError{&nonSplitImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.ExportedType", Loc: "3:18"}},
		},
		"GlobalExportedConstant": {
			in: `package test

const MyConst pkg.ExportedType = nil
`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"GlobalExportedConstantNonSplit": {
			in: `package test

const MyConst pkg.ExportedType = nil
`,
			errs: []residualError{&nonSplitImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.ExportedType", Loc: "3:15"}},
		},
		"GlobalExportedVariable": {
			in: `package test

var MyVar pkg.ExportedType
`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"GlobalExportedVariableNonSplit": {
			in: `package test

var MyVar pkg.ExportedType
`,
			errs: []residualError{&nonSplitImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.ExportedType", Loc: "3:11"}},
		},
	}

	for n := range tcs { // nolint: dupl
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			fc, err := testcache.NewFakeFileCache("", map[string]testcache.FakeFileCacheEntry{
				"go.mod": testcache.FakeFileCacheEntry{Data: []byte("module example.com/pkg")},
				"lib.go": testcache.FakeFileCacheEntry{Data: []byte("package pkg\n")},
			})
			testlib.NoError(t, true, err)

			pkgToSplit := tc.pkgTosplit
			if pkgToSplit == nil {
				pkgToSplit = map[string]string{}
			}
			az := &analyser{
				log: testlib.NewTestLogger(),
				fc:  fc,
				sp:  &config.Splits{DataSplits: splits.DataSplits{PkgToSplit: pkgToSplit}},
			}
			a := &analysis{
				split:   &config.Split{DataSplit: splits.DataSplit{Name: testSplit}},
				fs:      token.NewFileSet(),
				imports: map[string]string{"pkg": testPkg},
			}
			f, err := parser.ParseFile(a.fs, "", tc.in, parser.AllErrors|parser.ParseComments)
			testlib.NoError(t, true, err)

			testlib.Equal(t, false, tc.errs, az.analyseFile(a, f))
		})
	}
}

func TestType(t *testing.T) {
	t.Parallel()

	const (
		testPkg   = "example.com/pkg"
		testSplit = "test-split"
	)
	depSplit := &config.Split{DataSplit: splits.DataSplit{Name: "split"}}

	tcs := map[string]struct {
		in         string
		pkgTosplit map[string]string
		errs       []residualError
	}{
		"LocalExportedType": {
			in: "LocalType",
		},
		"LocalUnexportedType": {
			in: "localType",
		},
		"ExternalSplitExportedType": {
			in:         "pkg.ExternalType",
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"ExternalSplitUnexportedType": {
			in:         "pkg.externalType",
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
			errs:       []residualError{&unexportedImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.externalType", Loc: "1:1"}},
		},
		"ExternalNonSplitExportedType": {
			in:   "pkg.ExternalType",
			errs: []residualError{&nonSplitImportErr{Split: testSplit, Pkg: testPkg, Symbol: "pkg.ExternalType", Loc: "1:1"}},
		},
		"ImpossibleNestedType": {
			in:   "pkg.ExternalType.Field",
			errs: []residualError{&unexpectedTypeErr{Split: testSplit, Symbol: "pkg.ExternalType.Field", Loc: "1:1"}},
		},
		"MapType": {
			in:         `map[LocalType]pkg.ExternalType`,
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
		"StarType": {
			in: "*LocalType",
		},
		"ParenType": {
			in: "(LocalType)",
		},
		"Arraytype": {
			in: "[]LocalType",
		},
		"ChanType": {
			in: "chan LocalType",
		},
		"ComplexType": {
			in:         "chan *([]*pkg.ExternalType)",
			pkgTosplit: map[string]string{testPkg: depSplit.Name},
		},
	}

	for n := range tcs { // nolint: dupl
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			fc, err := testcache.NewFakeFileCache("", map[string]testcache.FakeFileCacheEntry{
				"go.mod": testcache.FakeFileCacheEntry{Data: []byte("module example.com/pkg")},
				"lib.go": testcache.FakeFileCacheEntry{Data: []byte("package pkg\n")},
			})
			testlib.NoError(t, true, err)

			pkgToSplit := tc.pkgTosplit
			if pkgToSplit == nil {
				pkgToSplit = map[string]string{}
			}
			az := &analyser{
				log: testlib.NewTestLogger(),
				fc:  fc,
				sp:  &config.Splits{DataSplits: splits.DataSplits{PkgToSplit: pkgToSplit}},
			}
			a := &analysis{
				split:   &config.Split{DataSplit: splits.DataSplit{Name: "test-split"}},
				fs:      token.NewFileSet(),
				imports: map[string]string{"pkg": testPkg},
			}
			e, err := parser.ParseExprFrom(a.fs, "", tc.in, parser.AllErrors|parser.ParseComments)
			testlib.NoError(t, true, err)

			testlib.Equal(t, false, tc.errs, az.analyseCompositeType(a, e))
		})
	}
}
