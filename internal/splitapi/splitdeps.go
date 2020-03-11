package splitapi

import (
	"errors"

	"go.uber.org/zap"

	"github.com/modularise/modularise/cmd/config"
)

func (az *analyser) analyseSplitDepGraph() error {
	a := &depAnalysis{
		done: map[string]bool{},
		todo: map[string]bool{},
	}
	for _, s := range az.sp.Splits {
		if err := az.analyseSplitDeps(s, a); err != nil {
			return err
		}
	}
	return nil
}

type depAnalysis struct {
	done  map[string]bool
	todo  map[string]bool
	stack []string
}

type CyclicDependencyErr error

func (az *analyser) analyseSplitDeps(s *config.Split, a *depAnalysis) error {
	// Prevent double-processing and detect circular dependencies between splits.
	if a.done[s.Name] {
		return nil
	} else if a.todo[s.Name] {
		az.log.Error("A circular dependency exists between the configured splits. This is not allowed.", zap.Strings("split-stack", a.stack))
		return CyclicDependencyErr(errors.New("circular split dependency found"))
	}

	a.todo[s.Name] = true
	defer func() {
		delete(a.todo, s.Name)
		a.done[s.Name] = true
	}()

	a.stack = append(a.stack, s.Name)
	for sn := range s.SplitDeps {
		if err := az.analyseSplitDeps(az.sp.Splits[sn], a); err != nil {
			return err
		}
	}
	a.stack = a.stack[:len(a.stack)-1]
	return nil
}
