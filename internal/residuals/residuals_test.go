package residuals

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/Helcaraxan/modularise/internal/filecache/testcache"
	"github.com/Helcaraxan/modularise/internal/splits"
	"github.com/Helcaraxan/modularise/internal/testlib"
)

func TestFile(t *testing.T) {
	t.Parallel()

	splitPkg := "example.com/pkg"
	depSplit := &splits.Split{DataSplit: splits.DataSplit{Name: "split"}}

	tcs := map[string]struct {
		in         string
		pkgTosplit map[string]*splits.Split
		errTypes   []reflect.Type
	}{
		"InterfaceType": {
			in: `package test

type MyInterface interface {
	LocalMethod(LocalType) (LocalType, error)
	ExternalMethod(pkg.ExternalType) (pkg.ExternalType, error)
}`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"InterfaceTypeWithEmbedding": {
			in: `package test

type MyInterface interface {
	pkg.ExternalType

	LocalMethod(LocalType) (LocalType, error)
}`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"StructType": {
			in: `package test

type MyStruct struct {
	LocalField LocalType
	ExternalField pkg.ExternalType
}`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"StructTypeWithEmbedding": {
			in: `package test

type MyStruct struct {
	pkg.ExternalType

	LocalField LocalType
}`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
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
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"ExportedFuncNoSplit": {
			in: `package test

func ExportedFunc(_ pkg.ExternalType) {}
`,
			errTypes: []reflect.Type{reflect.TypeOf(nonSplitImportErr(""))},
		},
		"TypeRedeclaration": {
			in: `package test

type LocalType pkg.ExportedType
`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"TypeRedeclarationNonSplit": {
			in: `package test

type LocalType pkg.ExportedType
`,
			errTypes: []reflect.Type{reflect.TypeOf(nonSplitImportErr(""))},
		},
		"TypeAlias": {
			in: `package test

type LocalType = pkg.ExportedType
`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"TypeAliasNonSplit": {
			in: `package test

type LocalType = pkg.ExportedType
`,
			errTypes: []reflect.Type{reflect.TypeOf(nonSplitImportErr(""))},
		},
		"GlobalExportedConstant": {
			in: `package test

const MyConst pkg.ExportedType = nil
`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"GlobalExportedConstantNonSplit": {
			in: `package test

const MyConst pkg.ExportedType = nil
`,
			errTypes: []reflect.Type{reflect.TypeOf(nonSplitImportErr(""))},
		},
		"GlobalExportedVariable": {
			in: `package test

var MyVar pkg.ExportedType
`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"GlobalExportedVariableNonSplit": {
			in: `package test

var MyVar pkg.ExportedType
`,
			errTypes: []reflect.Type{reflect.TypeOf(nonSplitImportErr(""))},
		},
	}

	for n := range tcs { // nolint: dupl
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			pkgToSplit := tc.pkgTosplit
			if pkgToSplit == nil {
				pkgToSplit = map[string]*splits.Split{}
			}
			a := &analyser{
				log:     l,
				fs:      token.NewFileSet(),
				imports: map[string]string{"pkg": splitPkg},
				pkgs:    map[string]bool{splitPkg: true},
				s:       &splits.Split{DataSplit: splits.DataSplit{Name: "test-split"}},
				sp:      &splits.Splits{DataSplits: splits.DataSplits{PkgToSplit: pkgToSplit}},
			}
			f, err := parser.ParseFile(a.fs, "", tc.in, parser.AllErrors|parser.ParseComments)
			testlib.NoError(t, true, err)

			a.analyseFile(f)

			l.Debugf("DEBUG - %s: %v", t.Name(), a.errs)
			testlib.True(t, true, len(tc.errTypes) == len(a.errs))
			for i := range tc.errTypes {
				testlib.True(t, false, reflect.TypeOf(a.errs[i]).ConvertibleTo(tc.errTypes[i]))
			}
		})
	}
}

func TestType(t *testing.T) {
	t.Parallel()

	splitPkg := "example.com/pkg"
	depSplit := &splits.Split{DataSplit: splits.DataSplit{Name: "split"}}

	tcs := map[string]struct {
		in         string
		pkgTosplit map[string]*splits.Split
		errTypes   []reflect.Type
	}{
		"LocalExportedType": {
			in: "LocalType",
		},
		"LocalUnexportedType": {
			in: "localType",
		},
		"ExternalSplitExportedType": {
			in:         "pkg.ExternalType",
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
		"ExternalSplitUnexportedType": {
			in:         "pkg.externalType",
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
			errTypes:   []reflect.Type{reflect.TypeOf(unexpectedTypeErr(""))},
		},
		"ExternalNonSplitExportedType": {
			in:       "pkg.ExternalType",
			errTypes: []reflect.Type{reflect.TypeOf(nonSplitImportErr(""))},
		},
		"ImpossibleNestedType": {
			in:       "pkg.ExternalType.Field",
			errTypes: []reflect.Type{reflect.TypeOf(unexpectedTypeErr(""))},
		},
		"MapType": {
			in:         `map[LocalType]pkg.ExternalType`,
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
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
			pkgTosplit: map[string]*splits.Split{splitPkg: depSplit},
		},
	}

	for n := range tcs { // nolint: dupl
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			pkgToSplit := tc.pkgTosplit
			if pkgToSplit == nil {
				pkgToSplit = map[string]*splits.Split{}
			}
			a := &analyser{
				log:     l,
				fs:      token.NewFileSet(),
				imports: map[string]string{"pkg": splitPkg},
				pkgs:    map[string]bool{splitPkg: true},
				s:       &splits.Split{DataSplit: splits.DataSplit{Name: "test-split"}},
				sp:      &splits.Splits{DataSplits: splits.DataSplits{PkgToSplit: pkgToSplit}},
			}
			e, err := parser.ParseExprFrom(a.fs, "", tc.in, parser.AllErrors|parser.ParseComments)
			testlib.NoError(t, true, err)

			a.analyseCompositeType(e)

			testlib.True(t, true, len(tc.errTypes) == len(a.errs))
			for i := range tc.errTypes {
				testlib.True(t, false, reflect.TypeOf(a.errs[i]).ConvertibleTo(tc.errTypes[i]))
			}
		})
	}
}

func TestResolveImportsAndResiduals(t *testing.T) {
	t.Parallel()

	const testModulePath = "example.com/repo"
	pkgPath := func(p string) string { return filepath.Join(testModulePath, p) }
	depSplitA := &splits.Split{DataSplit: splits.DataSplit{Name: "a"}}
	depSplitB := &splits.Split{DataSplit: splits.DataSplit{Name: "b"}}

	tcs := map[string]struct {
		imports           []*ast.ImportSpec
		pkgToSplit        map[string]*splits.Split
		expectedImports   map[string]string
		expectedResiduals map[string]bool
		expectedSplitDeps map[string]bool
	}{
		"NoImports": {
			imports:           nil,
			pkgToSplit:        map[string]*splits.Split{},
			expectedImports:   map[string]string{},
			expectedResiduals: map[string]bool{},
			expectedSplitDeps: map[string]bool{},
		},
		"ThirdPartyImports": {
			imports: []*ast.ImportSpec{
				{Path: &ast.BasicLit{Value: `"third-party.com/module"`}},
			},
			pkgToSplit: map[string]*splits.Split{
				pkgPath("bar"): depSplitA,
			},
			expectedImports: map[string]string{
				"module": "third-party.com/module",
			},
			expectedResiduals: map[string]bool{},
			expectedSplitDeps: map[string]bool{},
		},
		"NoResiduals": {
			imports: []*ast.ImportSpec{
				{Path: &ast.BasicLit{Value: pkgPath("bar")}},
				{Name: ast.NewIdent("renamed"), Path: &ast.BasicLit{Value: pkgPath("bar/bar")}},
			},
			pkgToSplit: map[string]*splits.Split{
				pkgPath("bar"):     depSplitA,
				pkgPath("bar/bar"): depSplitA,
			},
			expectedImports: map[string]string{
				"bar":     pkgPath("bar"),
				"renamed": pkgPath("bar/bar"),
			},
			expectedResiduals: map[string]bool{},
			expectedSplitDeps: map[string]bool{},
		},
		"Residuals": {
			imports: []*ast.ImportSpec{
				{Path: &ast.BasicLit{Value: pkgPath("bar")}},
				{Name: ast.NewIdent("renamed"), Path: &ast.BasicLit{Value: pkgPath("bar/bar")}},
			},
			pkgToSplit: map[string]*splits.Split{
				pkgPath("bar/bar"): depSplitA,
			},
			expectedImports: map[string]string{
				"bar":     pkgPath("bar"),
				"renamed": pkgPath("bar/bar"),
			},
			expectedResiduals: map[string]bool{
				pkgPath("bar"): true,
			},
			expectedSplitDeps: map[string]bool{},
		},
		"SplitDeps": {
			imports: []*ast.ImportSpec{
				{Path: &ast.BasicLit{Value: pkgPath("bar")}},
				{Name: ast.NewIdent("renamed"), Path: &ast.BasicLit{Value: pkgPath("bar/bar")}},
			},
			pkgToSplit: map[string]*splits.Split{
				pkgPath("bar"):     depSplitA,
				pkgPath("bar/bar"): depSplitB,
			},
			expectedImports: map[string]string{
				"bar":     pkgPath("bar"),
				"renamed": pkgPath("bar/bar"),
			},
			expectedResiduals: map[string]bool{},
			expectedSplitDeps: map[string]bool{
				"b": true,
			},
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			fe := map[string]testcache.FakeFileCacheEntry{}
			for _, i := range tc.imports {
				fe[strings.TrimPrefix(filepath.Join(i.Path.Value, "file.go"), testModulePath+"/")] = testcache.FakeFileCacheEntry{}
			}
			fe["go.mod"] = testcache.FakeFileCacheEntry{Data: []byte("module example.com/repo")}

			fc, err := testcache.NewFakeFileCache("fake-cache-dir", fe)
			testlib.NoError(t, true, err)

			a := analyser{
				log:     l,
				fc:      fc,
				imports: map[string]string{},
				s: &splits.Split{DataSplit: splits.DataSplit{
					Name:      "a",
					Residuals: map[string]bool{},
					SplitDeps: map[string]bool{},
				}},
				sp: &splits.Splits{DataSplits: splits.DataSplits{PkgToSplit: tc.pkgToSplit}},
			}
			err = a.computeSplitDepsAndResiduals(tc.imports)
			testlib.NoError(t, true, err)
			testlib.Equal(t, false, tc.expectedImports, a.imports)
			testlib.Equal(t, false, tc.expectedResiduals, a.s.Residuals)
			testlib.Equal(t, false, tc.expectedSplitDeps, a.s.SplitDeps)
		})
	}
}
