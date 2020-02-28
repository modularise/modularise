package residuals

import "fmt"

type residualError interface {
	error
	Details() string
}

type unexpectedTypeErr struct {
	Split  string
	Symbol string
	Loc    string
}

func (e unexpectedTypeErr) Error() string {
	return fmt.Sprintf("public interface of split %q contains an unexpected syntax", e.Split)
}

func (e unexpectedTypeErr) Details() string {
	return fmt.Sprintf("public interface of split %q contains an unexpected syntax %q at %q", e.Split, e.Symbol, e.Loc)
}

type unexportedImportErr struct {
	Split  string
	Pkg    string
	Symbol string
	Loc    string
}

func (e unexportedImportErr) Error() string {
	return fmt.Sprintf("public interface of split %q imports an unexpected symbol from package %q", e.Split, e.Pkg)
}

func (e unexportedImportErr) Details() string {
	return fmt.Sprintf("public interface of split %q imports an unexpected symbol %q from package %q at %q", e.Split, e.Symbol, e.Pkg, e.Loc)
}

type nonSplitImportErr struct {
	Split  string
	Pkg    string
	Symbol string
	Loc    string
}

func (e nonSplitImportErr) Error() string {
	return fmt.Sprintf("public interface of split %q refers to package %q which is not part of any configured split", e.Split, e.Pkg)
}

func (e nonSplitImportErr) Details() string {
	return fmt.Sprintf(
		"public interface of split %q refers to symbol %q of package %q at %q which is not part of any configured split",
		e.Split,
		e.Symbol,
		e.Pkg,
		e.Loc,
	)
}
