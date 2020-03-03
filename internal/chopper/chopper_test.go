package chopper

import (
	"fmt"
	"go/ast"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
	"github.com/sirupsen/logrus"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/filecache/uncache"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
)

func TestComputeSplitRoot(t *testing.T) {
	t.Parallel()

	tcs := map[string]struct {
		files map[string]bool
		root  string
	}{
		"EmptyList": {
			files: map[string]bool{},
			root:  "",
		},
		"OneElement": {
			files: map[string]bool{"my/own/path/file.go": true},
			root:  "my/own/path",
		},
		"AbsolutePath": {
			files: map[string]bool{"/my/own/path/file.go": true},
			root:  "my/own/path",
		},
		"TwoRelatedPaths": {
			files: map[string]bool{"my/own/path/file.go": true, "my/own/way/file.go": true},
			root:  "my/own",
		},
		"TwoUnrelatedPaths": {
			files: map[string]bool{"my/own/path/file.go": true, "your/other/way/file.go": true},
			root:  "",
		},
		"ShortLong": {
			files: map[string]bool{"my/own/file.go": true, "my/own/path/file.go": true},
			root:  "my/own",
		},
		"LongShort": {
			files: map[string]bool{"my/own/path/file.go": true, "my/own/file.go": true},
			root:  "my/own",
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			r := computeSplitRoot(tc.files)
			testlib.Equal(t, false, tc.root, r)
		})
	}
}

func TestRewriteImports(t *testing.T) {
	t.Parallel()

	depSplitA := &config.Split{ModulePath: "split.com/root/a", DataSplit: splits.DataSplit{Name: "a", Root: "a", ResidualsRoot: "."}}
	depSplitB := &config.Split{ModulePath: "split.com/root/b", DataSplit: splits.DataSplit{Name: "b", Root: "b"}}

	importLiteral := func(i string) *ast.BasicLit {
		return &ast.BasicLit{Value: fmt.Sprintf(`"%s"`, i)}
	}

	tcs := map[string]struct {
		imports    []*ast.ImportSpec
		pkgsA      []string
		pkgsB      []string
		residualsA map[string]bool
		expected   []*ast.ImportSpec
	}{
		"NoSplits": {
			imports: []*ast.ImportSpec{
				{Path: importLiteral("foo.com/bar")},
				{Name: ast.NewIdent("renamed_import"), Path: importLiteral("foo.com/colliding/bar")},
			},
			expected: []*ast.ImportSpec{
				{Path: importLiteral("foo.com/bar")},
				{Name: ast.NewIdent("renamed_import"), Path: importLiteral("foo.com/colliding/bar")},
			},
		},
		"NoRenames": {
			imports: []*ast.ImportSpec{
				{Path: importLiteral("foo.com/bar")},
				{Name: ast.NewIdent("renamed_import"), Path: importLiteral("foo.com/colliding/bar")},
			},
			pkgsA: []string{
				"foo.com/a",
				"foo.com/a/lib",
			},
			expected: []*ast.ImportSpec{
				{Path: importLiteral("foo.com/bar")},
				{Name: ast.NewIdent("renamed_import"), Path: importLiteral("foo.com/colliding/bar")},
			},
		},
		"Renames": {
			imports: []*ast.ImportSpec{
				{Path: importLiteral("foo.com/bar/a/pkg")},
				{Path: importLiteral("foo.com/bar/b")},
				{Path: importLiteral("foo.com/bar/b/lib")},
			},
			pkgsA: []string{
				"foo.com/bar/a",
				"foo.com/bar/a/pkg",
			},
			pkgsB: []string{
				"foo.com/bar/b",
				"foo.com/bar/b/lib",
			},
			expected: []*ast.ImportSpec{
				{Path: importLiteral("split.com/root/a/pkg")},
				{Path: importLiteral("split.com/root/b")},
				{Path: importLiteral("split.com/root/b/lib")},
			},
		},
		"RenamesWithInternal": {
			imports: []*ast.ImportSpec{
				{Path: importLiteral("foo.com/bar/a/pkg")},
				{Path: importLiteral("foo.com/bar/a/deadbeef")},
				{Path: importLiteral("foo.com/bar/a/internal/helper")},
				{Path: importLiteral("foo.com/bar/util/lib")},
			},
			pkgsA: []string{
				"foo.com/bar/a",
				"foo.com/bar/a/pkg",
			},
			residualsA: map[string]bool{
				"foo.com/bar/a/deadbeef":        true,
				"foo.com/bar/a/internal/helper": true,
				"foo.com/bar/util/lib":          true,
			},
			expected: []*ast.ImportSpec{
				{Path: importLiteral("split.com/root/a/pkg")},
				{Path: importLiteral("split.com/root/a/internal/residuals/a/deadbeef")},
				{Path: importLiteral("split.com/root/a/internal/helper")},
				{Path: importLiteral("split.com/root/a/internal/residuals/util/lib")},
			},
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			// Make local copies of the test splits to allow for parallel testing.
			a := *depSplitA
			a.Residuals = tc.residualsA
			b := *depSplitB

			sp := &config.Splits{
				Splits: map[string]*config.Split{
					a.Name:         &a,
					depSplitB.Name: depSplitB,
				},
				DataSplits: splits.DataSplits{
					PathToSplit: map[string]string{
						a.ModulePath:         a.Name,
						depSplitB.ModulePath: depSplitB.Name,
					},
					PkgToSplit: map[string]string{},
				},
			}
			for _, pkg := range tc.pkgsA {
				sp.PkgToSplit[pkg] = a.Name
			}
			for _, pkg := range tc.pkgsB {
				sp.PkgToSplit[pkg] = b.Name
			}

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			fc, err := testcache.NewFakeFileCache("invalid-root", map[string]testcache.FakeFileCacheEntry{
				"go.mod": {Data: []byte("module foo.com/bar")},
			})
			testlib.NoError(t, true, err)

			c := cleaver{
				log: l,
				fc:  fc,
				s:   &a,
				sp:  sp,
			}

			f := &ast.File{Imports: tc.imports}
			c.rewriteImports(f)
			testlib.Equal(t, false, tc.expected, f.Imports)
		})
	}
}

func TestCleaveSplit(t *testing.T) {
	t.Parallel()

	tcs, gErr := filepath.Glob("./testdata/*_in.txtar")
	testlib.NoError(t, true, gErr)
	testlib.True(t, true, len(tcs) > 0)

	for i := range tcs {
		tf := tcs[i]
		n := regexp.MustCompile(`testdata/(.*)_in\.txtar`).FindStringSubmatch(tf)[1]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			tin, err := ioutil.ReadFile(tf)
			testlib.NoError(t, true, err)

			tout, err := ioutil.ReadFile(fmt.Sprintf("./testdata/%s_out.txtar", n))
			testlib.NoError(t, true, err)

			td, err := ioutil.TempDir("", "modularise-chopper-test")
			testlib.NoError(t, true, err)
			defer func() { testlib.NoError(t, false, os.RemoveAll(td)) }()

			ain := txtar.Parse(tin)
			aout := txtar.Parse(tout)

			p := filepath.Join(td, "source")
			err = os.MkdirAll(p, 0755)
			testlib.NoError(t, true, err)

			err = txtar.Write(ain, p)
			testlib.NoError(t, true, err)

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			fc, err := uncache.NewUncache(l, p)
			testlib.NoError(t, true, err)

			p = filepath.Join(td, "target")
			err = os.MkdirAll(p, 0755)
			testlib.NoError(t, true, err)

			s := config.Split{DataSplit: splits.DataSplit{
				Name:      "test-split",
				Files:     map[string]bool{},
				Residuals: map[string]bool{},
				WorkDir:   p,
			}}
			sp := config.Splits{
				Splits: map[string]*config.Split{"test": &s},
				DataSplits: splits.DataSplits{
					PkgToSplit: map[string]string{},
				},
			}

			for _, l := range strings.Split(string(ain.Comment), "\n") {
				switch {
				case strings.HasPrefix(l, "split:"):
					s.ModulePath = strings.TrimSpace(strings.TrimPrefix(l, "split:"))
				case strings.HasPrefix(l, "root:"):
					s.Root = strings.TrimSpace(strings.TrimPrefix(l, "root:"))
				case strings.HasPrefix(l, "residual_root:"):
					s.ResidualsRoot = strings.TrimSpace(strings.TrimPrefix(l, "residual_root:"))
				case strings.HasPrefix(l, "file:"):
					f := strings.TrimSpace(strings.TrimPrefix(l, "file:"))
					s.Files[f] = true
					sp.PkgToSplit[filepath.Join(fc.ModulePath(), filepath.Dir(f))] = s.Name
				case strings.HasPrefix(l, "residual:"):
					s.Residuals[strings.TrimSpace(strings.TrimPrefix(l, "residual:"))] = true
				default:
					// ignore comments
				}
			}
			testlib.NotEqual(t, true, "", s.ModulePath)
			testlib.NotEqual(t, true, "", s.Root)

			s.ResidualFiles = map[string]bool{}
			for _, f := range ain.Files {
				if s.Residuals[filepath.Join(fc.ModulePath(), filepath.Dir(f.Name))] {
					s.ResidualFiles[f.Name] = true
				}
			}

			cl := cleaver{
				log: l,
				fc:  fc,
				s:   &s,
				sp:  &sp,
			}
			err = cl.cleaveSplit()
			testlib.NoError(t, true, err)

			expected := map[string]bool{}
			for _, f := range aout.Files {
				expected[f.Name] = true
			}

			found := map[string]bool{}
			err = filepath.Walk(s.WorkDir, func(p string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !info.IsDir() {
					found[strings.TrimPrefix(p, s.WorkDir+"/")] = true
				}
				return nil
			})
			testlib.NoError(t, true, err)
			testlib.Equal(t, true, expected, found)

			for _, f := range aout.Files {
				var c []byte
				c, err = ioutil.ReadFile(filepath.Join(s.WorkDir, f.Name))
				testlib.NoError(t, true, err)
				testlib.Equal(t, false, f.Data, c)
			}
		})
	}
}
