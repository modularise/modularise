package splitapi

import (
	"testing"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
)

func TestAnalyseSplitDepGraph(t *testing.T) {
	tcs := map[string]struct {
		splits config.Splits
		valid  bool
	}{
		"NoDeps": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{}}},
				},
			},
			valid: true,
		},
		"SingleDep": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{"b": true}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{}}},
				},
			},
			valid: true,
		},
		"MultipleDeps": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{"b": true, "c": true}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{}}},
					"c": {DataSplit: splits.DataSplit{Name: "c", SplitDeps: map[string]bool{}}},
				},
			},
			valid: true,
		},
		"ChainedDeps": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{"b": true}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{"c": true}}},
					"c": {DataSplit: splits.DataSplit{Name: "c", SplitDeps: map[string]bool{}}},
				},
			},
			valid: true,
		},
		"SimpleCycle": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{"b": true}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{"a": true}}},
				},
			},
			valid: false,
		},
		"DeepCycle": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{"b": true}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{"c": true}}},
					"c": {DataSplit: splits.DataSplit{Name: "c", SplitDeps: map[string]bool{"a": true}}},
				},
			},
			valid: false,
		},
		"ForkedCycle": {
			splits: config.Splits{
				Splits: map[string]*config.Split{
					"a": {DataSplit: splits.DataSplit{Name: "a", SplitDeps: map[string]bool{"b": true}}},
					"b": {DataSplit: splits.DataSplit{Name: "b", SplitDeps: map[string]bool{"c": true, "d": true}}},
					"c": {DataSplit: splits.DataSplit{Name: "c", SplitDeps: map[string]bool{}}},
					"d": {DataSplit: splits.DataSplit{Name: "d", SplitDeps: map[string]bool{"e": true}}},
					"e": {DataSplit: splits.DataSplit{Name: "d", SplitDeps: map[string]bool{"a": true}}},
				},
			},
			valid: false,
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			r := analyser{
				log: testlib.NewTestLogger(),
				sp:  &tc.splits,
			}
			err := r.analyseSplitDepGraph()
			if tc.valid {
				testlib.NoError(t, false, err)
			} else {
				testlib.Error(t, true, err)
				_, ok := err.(CyclicDependencyErr)
				testlib.True(t, false, ok)
			}
		})
	}
}
