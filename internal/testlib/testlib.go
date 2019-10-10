package testlib

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Equal(t *testing.T, strict bool, expected interface{}, actual interface{}) {
	if cmp.Equal(expected, actual) {
		return
	}
	failWithDiff(t, strict, "Mismatched values", cmp.Diff(expected, actual))
}

func NotEqual(t *testing.T, strict bool, expected interface{}, actual interface{}) {
	if !cmp.Equal(expected, actual) {
		return
	}
	fail(t, strict, fmt.Sprintf("Unexpected value %v", expected))
}

func Error(t *testing.T, strict bool, actual error) {
	if !cmp.Equal(nil, actual) {
		return
	}
	fail(t, strict, "Expected an error but got none")
}

func NoError(t *testing.T, strict bool, actual error) {
	if cmp.Equal(nil, actual) {
		return
	}
	failWithDiff(t, strict, "Expected no error.", fmt.Sprintf("Error: %s", actual.Error()))
}

func Nil(t *testing.T, strict bool, actual interface{}) {
	if cmp.Equal(nil, actual) {
		return
	}
	failWithDiff(t, strict, "Expected nil.", cmp.Diff(nil, actual))
}

func NotNil(t *testing.T, strict bool, actual interface{}) {
	if !cmp.Equal(nil, actual) {
		return
	}
	fail(t, strict, "Expected a non-nil value.")
}

func True(t *testing.T, strict bool, actual bool) {
	if actual {
		return
	}
	fail(t, strict, "Expected condition to be true.")
}

func False(t *testing.T, strict bool, actual bool) {
	if !actual {
		return
	}
	fail(t, strict, "Expected condition to be false.")
}

func fail(t *testing.T, strict bool, msg string) {
	reportFunc(t, strict)("\nISSUE: %s\n\nCALLSTACK:\n%s", msg, location())
}

func failWithDiff(t *testing.T, strict bool, msg string, diff string) {
	reportFunc(t, strict)("\nISSUE: %s\n\nDIFF:\n%s\n\nCALLSTACK:\n%s", msg, diff, location())
}

func reportFunc(t *testing.T, strict bool) func(fmt string, args ...interface{}) {
	if strict {
		return t.Fatalf
	}
	return t.Errorf
}

func location() string {
	var locs []string

	sp := 3
	visited := map[string]int{}
	for {
		_, f, l, ok := runtime.Caller(sp)
		if !ok || strings.HasPrefix(f, runtime.GOROOT()) {
			break
		}
		fl := fmt.Sprintf("%s:%d", f, l)
		if visited[fl] > 1 {
			locs = append(locs, " - ...apparent call cycle...")
			break
		}
		sp++
		visited[fl]++
		locs = append(locs, fmt.Sprintf(" - %2d. %s", sp-3, fl))
	}

	if len(locs) == 0 {
		locs = []string{"<WARNING> Could not determine location of failed assertion."}
	}
	return strings.Join(locs, "\n")
}
