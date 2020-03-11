package splitapi

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
)

// AnalyseAPI iterates over the configured splits and performs the residuals analysis for each one
// of them. For the details of the residual analysis please consult the
// ./docs/design/technical_breakdown.md document residing with the source code.
//
// The prequisites on the fields of a config.Splits object for CleaveSplits to be able to operate
// are:
//  - For each config.Split in Splits the Name and Files fields have been populated.
func AnalyseAPI(log *zap.Logger, fc filecache.FileCache, sp *config.Splits) error {
	a := analyser{
		log: log,
		fc:  fc,
		sp:  sp,
	}

	var fail bool
	for _, s := range sp.Splits {
		analysisErrs, err := a.analyseSplitAPI(&analysis{split: s})
		if err != nil {
			return err
		} else if len(analysisErrs) == 0 {
			continue
		}

		fail = true
		msgs := map[string]bool{}
		for i := range analysisErrs {
			if log.Core().Enabled(zap.DebugLevel) {
				msgs[analysisErrs[i].Details()] = true
			} else {
				msgs[analysisErrs[i].Error()] = true
			}
		}
		log.Error("Detected errors while computing split residuals:")
		for msg := range msgs {
			log.Error(" - " + msg)
		}
	}
	if fail {
		return errors.New("errors detected during computation of split residuals")
	}

	if err := a.analyseSplitDepGraph(); err != nil {
		return err
	}
	return nil
}

type analyser struct {
	log *zap.Logger
	fc  filecache.FileCache
	sp  *config.Splits
}

type analysis struct {
	split   *config.Split
	fs      *token.FileSet
	imports map[string]string
}

func (az *analyser) analyseSplitAPI(a *analysis) ([]residualError, error) {
	var analysisErrs []residualError

	az.log.Debug("Analysing split.", zap.String("split", a.split.Name))

	for f := range a.split.Files {
		if filepath.Ext(f) != ".go" {
			az.log.Debug("Skipping analysis of non-Go file.", zap.String("file", f))
			continue
		}
		az.log.Debug("Analysing file for residuals.", zap.String("file", f))

		fa, fs, err := az.fc.ReadGoFile(f, parser.AllErrors)
		if err != nil {
			return nil, err
		}

		a.fs = fs
		a.imports = map[string]string{}
		for _, imp := range fa.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			n := filepath.Base(p)
			if imp.Name != nil {
				n = imp.Name.Name
			}
			a.imports[n] = p
		}

		if filepath.Base(f) != "test.go" && !strings.HasSuffix(f, "_test.go") {
			analysisErrs = append(analysisErrs, az.analyseFile(a, fa)...)
		}
	}
	return analysisErrs, nil
}

func (az *analyser) analyseFile(a *analysis, f *ast.File) (errs []residualError) {
	for _, tld := range f.Decls {
		switch td := tld.(type) {
		case *ast.FuncDecl:
			if td.Name.IsExported() {
				errs = append(errs, az.analyseFunc(a, td.Type)...)
			}
		case *ast.GenDecl:
			switch td.Tok {
			case token.TYPE:
				for _, sp := range td.Specs {
					tsp, ok := sp.(*ast.TypeSpec)
					if !ok {
						sb := strings.Builder{}
						printer.Fprint(&sb, a.fs, sp)
						errs = append(errs, &unexpectedTypeErr{
							Split:  a.split.Name,
							Symbol: sb.String(),
							Loc:    a.fs.Position(sp.Pos()).String(),
						})
						continue
					}
					if tsp.Name.IsExported() {
						errs = append(errs, az.analyseCompositeType(a, tsp.Type)...)
					}
				}
			case token.CONST, token.VAR:
				for _, sp := range td.Specs {
					vs, ok := sp.(*ast.ValueSpec)
					if !ok {
						sb := strings.Builder{}
						printer.Fprint(&sb, a.fs, sp)
						errs = append(errs, &unexpectedTypeErr{
							Split:  a.split.Name,
							Symbol: sb.String(),
							Loc:    a.fs.Position(sp.Pos()).String(),
						})
						continue
					}
					for _, n := range vs.Names {
						if n.IsExported() {
							errs = append(errs, az.analyseCompositeType(a, vs.Type)...)
							break
						}
					}
				}
			}
		}
	}
	return errs
}

func (az *analyser) analyseFunc(a *analysis, t *ast.FuncType) (errs []residualError) {
	if t.Params != nil {
		for _, f := range t.Params.List {
			errs = append(errs, az.analyseCompositeType(a, f.Type)...)
		}
	}
	if t.Results != nil {
		for _, f := range t.Results.List {
			errs = append(errs, az.analyseCompositeType(a, f.Type)...)
		}
	}
	return errs
}

func (az *analyser) analyseCompositeType(a *analysis, e ast.Expr) (errs []residualError) {
	switch te := e.(type) {
	case *ast.FuncType:
		errs = append(errs, az.analyseFunc(a, te)...)
	case *ast.InterfaceType:
		for _, f := range te.Methods.List {
			errs = append(errs, az.analyseCompositeType(a, f.Type)...)
		}
	case *ast.StructType:
		for _, f := range te.Fields.List {
			errs = append(errs, az.analyseCompositeType(a, f.Type)...)
		}
	default:
		// This is some form of (composite) type re-declaration.
		errs = append(errs, az.analyseType(a, te)...)
	}
	return errs
}

func (az *analyser) analyseType(a *analysis, e ast.Expr) (errs []residualError) {
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
		errs = append(errs, az.analyseCompositeType(a, te.Key)...)
		errs = append(errs, az.analyseCompositeType(a, te.Value)...)
	case *ast.SelectorExpr:
		// This is a type from another package.
		x, ok := te.X.(*ast.Ident)
		if !ok {
			// Selector expression can't be nested for types as there is no such thing as
			// nested types in Go.
			sb := &strings.Builder{}
			printer.Fprint(sb, a.fs, e)
			errs = append(errs, &unexpectedTypeErr{
				Split:  a.split.Name,
				Symbol: sb.String(),
				Loc:    a.fs.Position(e.Pos()).String(),
			})
			break
		}

		if !te.Sel.IsExported() {
			sb := &strings.Builder{}
			printer.Fprint(sb, a.fs, e)
			errs = append(errs, &unexportedImportErr{
				Split:  a.split.Name,
				Pkg:    a.imports[x.Name],
				Symbol: sb.String(),
				Loc:    a.fs.Position(e.Pos()).String(),
			})
		} else if az.fc.Pkgs()[a.imports[x.Name]] {
			if az.sp.PkgToSplit[a.imports[x.Name]] == "" {
				sb := &strings.Builder{}
				printer.Fprint(sb, a.fs, te)
				errs = append(
					errs,
					&nonSplitImportErr{
						Split:  a.split.Name,
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
	return errs
}
