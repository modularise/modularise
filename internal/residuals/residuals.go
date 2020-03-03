package residuals

import (
	"errors"
	"go/ast"
	"go/printer"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
)

func ComputeResiduals(l *logrus.Logger, fc filecache.FileCache, s *config.Splits) error {
	pkgs, err := fc.Pkgs()
	if err != nil {
		return err
	}

	var fail bool
	for _, v := range s.Splits {
		a := analyser{
			log:  l,
			fc:   fc,
			s:    v,
			sp:   s,
			pkgs: pkgs,
		}
		if err := a.analyseSplit(); err != nil {
			return err
		} else if len(a.errs) == 0 {
			continue
		}

		fail = true
		msgs := map[string]bool{}
		for _, err := range a.errs {
			if l.GetLevel() < logrus.DebugLevel {
				msgs[err.Error()] = true
			} else {
				msgs[err.Details()] = true
			}
		}
		l.Errorf("Detected errors while computing residuals for split %q:", v.Name)
		for msg := range msgs {
			l.Errorf(" - %s", msg)
		}
	}

	if fail {
		return errors.New("errors detected during computation of split residuals")
	}
	return nil
}

type analyser struct {
	log *logrus.Logger
	fc  filecache.FileCache
	s   *config.Split
	sp  *config.Splits

	// Fields used internally by the analyser.
	errs    []residualError
	fs      *token.FileSet
	imports map[string]string
	pkgs    map[string]bool
}

func (a *analyser) analyseSplit() error {
	a.log.Debugf("Analysing split %q for residuals.", a.s.Name)

	a.s.Residuals = map[string]bool{}
	a.s.SplitDeps = map[string]bool{}
	for f := range a.s.Files {
		if filepath.Ext(f) != ".go" {
			a.log.Debugf("Skipping analysis of %q in split %q as it's not a Go source file.", f, a.s.Name)
			continue
		}
		a.log.Debugf("Analysing %q part of split %q for residuals.", f, a.s.Name)
		fa, fs, err := a.fc.ReadGoFile(f)
		if err != nil {
			return err
		}

		a.fs = fs
		a.imports = map[string]string{}
		if err = a.computeSplitDepsAndResiduals(fa.Imports); err != nil {
			return err
		}

		if err = a.computeIndirectDependencies(); err != nil {
			return err
		}

		if filepath.Base(f) != "test.go" && !strings.HasSuffix(f, "_test.go") {
			a.analyseFile(fa)
		}
	}
	return nil
}

func (a *analyser) computeSplitDepsAndResiduals(imports []*ast.ImportSpec) error {
	pkgs, err := a.fc.Pkgs()
	if err != nil {
		return err
	}

	for _, imp := range imports {
		a.log.Debugf("Considering import of %q.", imp.Path.Value)

		p := strings.Trim(imp.Path.Value, `"`)
		n := filepath.Base(p)
		if imp.Name != nil {
			n = imp.Name.Name
		}
		a.imports[n] = p

		if !pkgs[p] {
			continue
		}

		if ts := a.sp.PkgToSplit[p]; ts != "" && ts != a.s.Name {
			a.log.Debugf("Import of %q induces a dependency from split %q on split %q.", imp.Path.Value, a.s.Name, ts)
			a.s.SplitDeps[ts] = true
		} else if ts == "" {
			a.log.Debugf("Import of %q results in the package being a residual of split %q.", imp.Path.Value, a.s.Name)
			a.s.Residuals[p] = true
		}
	}
	return nil
}

func (a *analyser) analyseFile(f *ast.File) {
	a.log.Debugf("Parsing file %q of package %q for residuals.", a.fs.Position(f.Pos()).Filename, f.Name)
	for _, tld := range f.Decls {
		switch td := tld.(type) {
		case *ast.FuncDecl:
			if td.Name.IsExported() {
				a.analyseFunc(td.Type)
			}
		case *ast.GenDecl:
			switch td.Tok {
			case token.TYPE:
				for _, sp := range td.Specs {
					tsp, ok := sp.(*ast.TypeSpec)
					if !ok {
						sb := strings.Builder{}
						printer.Fprint(&sb, a.fs, sp)
						a.errs = append(a.errs, &unexpectedTypeErr{
							Split:  a.s.Name,
							Symbol: sb.String(),
							Loc:    a.fs.Position(sp.Pos()).String(),
						})
						continue
					}
					if tsp.Name.IsExported() {
						a.analyseCompositeType(tsp.Type)
					}
				}
			case token.CONST, token.VAR:
				for _, sp := range td.Specs {
					vs, ok := sp.(*ast.ValueSpec)
					if !ok {
						sb := strings.Builder{}
						printer.Fprint(&sb, a.fs, sp)
						a.errs = append(a.errs, &unexpectedTypeErr{
							Split:  a.s.Name,
							Symbol: sb.String(),
							Loc:    a.fs.Position(sp.Pos()).String(),
						})
						continue
					}
					for _, n := range vs.Names {
						if n.IsExported() {
							a.analyseCompositeType(vs.Type)
							break
						}
					}
				}
			default:
				a.log.Debugf("Skipping top-level declaration %q at '%v' as it can't contain residuals.", td.Tok, a.fs.Position(td.TokPos))
			}
		}
	}
}

func (a *analyser) analyseFunc(t *ast.FuncType) {
	if t.Params != nil {
		for _, f := range t.Params.List {
			a.analyseCompositeType(f.Type)
		}
	}
	if t.Results != nil {
		for _, f := range t.Results.List {
			a.analyseCompositeType(f.Type)
		}
	}
}

func (a *analyser) analyseCompositeType(e ast.Expr) {
	switch te := e.(type) {
	case *ast.FuncType:
		a.analyseFunc(te)
	case *ast.InterfaceType:
		for _, f := range te.Methods.List {
			a.analyseCompositeType(f.Type)
		}
	case *ast.StructType:
		for _, f := range te.Fields.List {
			a.analyseCompositeType(f.Type)
		}
	default:
		// This is some form of (composite) type re-declaration.
		a.analyseType(te)
	}
}

func (a *analyser) analyseType(e ast.Expr) {
	// Composite types (pointers, slices, etc) need to be "unnested" to obtain the relevant type
	// information.
	var done bool
	for !done {
		switch te := e.(type) {
		case *ast.StarExpr:
			e = te.X
		case *ast.ParenExpr:
			e = te.X
		case *ast.ArrayType:
			e = te.Elt
		case *ast.ChanType:
			e = te.Value
		default:
			done = true
		}
	}

	switch te := e.(type) {
	case *ast.MapType:
		// We treat map-types differently as they potentially require us to resolve two types.
		a.analyseCompositeType(te.Key)
		a.analyseCompositeType(te.Value)
	case *ast.SelectorExpr:
		// This is a type from another package.
		x, ok := te.X.(*ast.Ident)
		if !ok {
			// Selector expression can't be nested for types as there is no such thing as
			// nested types in Go.
			sb := &strings.Builder{}
			printer.Fprint(sb, a.fs, e)
			a.errs = append(a.errs, &unexpectedTypeErr{
				Split:  a.s.Name,
				Symbol: sb.String(),
				Loc:    a.fs.Position(e.Pos()).String(),
			})
			break
		}

		if !te.Sel.IsExported() {
			sb := &strings.Builder{}
			printer.Fprint(sb, a.fs, e)
			a.errs = append(a.errs, &unexportedImportErr{
				Split:  a.s.Name,
				Pkg:    a.imports[x.Name],
				Symbol: sb.String(),
				Loc:    a.fs.Position(e.Pos()).String(),
			})
		} else if a.pkgs[a.imports[x.Name]] {
			if a.sp.PkgToSplit[a.imports[x.Name]] == "" {
				sb := &strings.Builder{}
				printer.Fprint(sb, a.fs, te)
				a.errs = append(
					a.errs,
					&nonSplitImportErr{
						Split:  a.s.Name,
						Pkg:    a.imports[x.Name],
						Symbol: sb.String(),
						Loc:    a.fs.Position(x.Pos()).String(),
					},
				)
			}
		}
	default:
		// No further analysis is required at this point.
	}
}

func (a *analyser) computeIndirectDependencies() error {
	a.s.ResidualFiles = map[string]bool{}

	var todo []string
	for pkg := range a.s.Residuals {
		fs, err := a.fc.FilesInPkg(pkg)
		if err != nil {
			return err
		}
		for f := range fs {
			a.s.ResidualFiles[f] = true
			if filepath.Ext(f) == ".go" {
				todo = append(todo, f)
			}
		}
	}

	for len(todo) > 0 {
		f := todo[0]
		todo = todo[1:]
		a.log.Debugf("Parsing residual file %q for indirect dependencies.", f)

		fa, fs, err := a.fc.ReadGoFile(f)
		if err != nil {
			return err
		}

		for _, imp := range fa.Imports {
			p := strings.Trim(imp.Path.Value, "\"")
			a.log.Debugf("Considering import of %q in (in)direct residual at %q", p, fs.Position(imp.Pos()))
			if a.s.Residuals[p] {
				a.log.Debugf("Skipping as the package is already a residual of split %q.", a.s.Name)
				continue
			} else if !a.pkgs[p] {
				continue
			}

			if ts := a.sp.PkgToSplit[p]; ts == a.s.Name {
				a.log.Debugf("Import of %q is internal to split %q.", p, ts)
				continue
			} else if ts != "" {
				a.log.Debugf("Import of %q results in a dependency from split %q on split %q.", p, a.s.Name, ts)
				a.s.SplitDeps[ts] = true
				continue
			}

			a.log.Debugf("Import of %q results in it becoming a residual of split %q.", p, a.s.Name)
			a.s.Residuals[p] = true

			pkgFiles, err := a.fc.FilesInPkg(p)
			if err != nil {
				return err
			}
			for f := range pkgFiles {
				if filepath.Ext(f) == ".go" {
					todo = append(todo, f)
				}
			}
		}
	}
	return nil
}
