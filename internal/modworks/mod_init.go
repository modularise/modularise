package modworks

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"

	"github.com/modularise/modularise/internal/filecache"
	"github.com/modularise/modularise/internal/splits"
)

func CreateSplitModules(log *logrus.Logger, fc filecache.FileCache, sp *splits.Splits) error {
	if !sp.NonModuleSource {
		// Ensure the module-cache is preheated such that future runs of 'go mod tidy' can be done with
		// only a temporary and partial local module proxy with split content.
		log.Debugf("Pre-heating the module cache by running 'go mod tidy' on the source project at %q.", fc.Root())
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = fc.Root()
		if out, err := cmd.CombinedOutput(); err != nil {
			log.WithError(err).Errorf("Failed to run 'go mod tidy' on source project in %q. Output was:\n%s", fc.Root(), out)
			return err
		}
	}

	r, err := setupResolver(log, fc, sp)
	if err != nil {
		return err
	}

	for sn := range sp.Splits {
		if err := r.createSplitModule(sp.Splits[sn], map[string]bool{}, []string{sn}); err != nil {
			return err
		}
	}
	return nil
}

type resolver struct {
	log        *logrus.Logger
	fc         filecache.FileCache
	sp         *splits.Splits
	mod        string
	sourceVer  string
	localProxy string
	done       map[string]bool
	todo       map[string]bool
}

func setupResolver(log *logrus.Logger, fc filecache.FileCache, sp *splits.Splits) (*resolver, error) {
	var err error
	var smc []byte
	if !sp.NonModuleSource {
		sm := filepath.Join(fc.Root(), "go.mod")
		smc, err = ioutil.ReadFile(sm)
		if err != nil {
			log.WithError(err).Errorf("Failed to read the source go.mod file at %q.", sm)
		}
	}

	repo, err := git.PlainOpen(fc.Root())
	if err != nil {
		log.WithError(err).Errorf("Could not open the source project's git repository at %q.", fc.Root())
		return nil, err
	}

	h, err := repo.Head()
	if err != nil {
		log.WithError(err).Errorf("Could not determine the source project's HEAD commit at %q.", fc.Root())
		return nil, err
	}

	lpp, err := ioutil.TempDir("", "modularise-local-proxy")
	if err != nil {
		log.WithError(err).Error("Could not create directory for temporary local module proxy content.")
		return nil, err
	}

	return &resolver{
		log:        log,
		fc:         fc,
		sp:         sp,
		mod:        string(smc),
		sourceVer:  h.Hash().String()[:12],
		localProxy: lpp,
		done:       map[string]bool{},
		todo:       map[string]bool{},
	}, nil
}

const tempReplaceMarker = "// modularise"

func (r *resolver) createSplitModule(s *splits.Split, deps map[string]bool, stack []string) error {
	// Prevent double-processing and detect circular dependencies between splits.
	if r.done[s.Name] {
		return nil
	} else if r.todo[s.Name] {
		r.log.Errorf("A circular dependency exists between the configured splits. This is not allowed. Split stack: %v.", stack)
		return errors.New("circular split dependency found")
	}
	r.todo[s.Name] = true
	defer func() {
		delete(r.todo, s.Name)
		r.done[s.Name] = true
	}()

	// Process upstream splits first.
	for sn := range s.SplitDeps {
		if err := r.createSplitModule(r.sp.Splits[sn], deps, append(stack, sn)); err != nil {
			return err
		}
		deps[sn] = true
	}

	if err := r.initSplitModule(s, deps); err != nil {
		return err
	}
	if err := r.resolveSplitDeps(s); err != nil {
		return err
	}
	if err := r.populateLocalProxy(s); err != nil {
		return err
	}
	return nil
}

func (r *resolver) initSplitModule(s *splits.Split, deps map[string]bool) error {
	modFile := filepath.Join(s.WorkDir, "go.mod")
	if !r.sp.NonModuleSource {
		// We need to change the module path in the source project's go.mod file before writing it to
		// the split's working directory. We also need to add temporary local 'replace' statements for
		// each of the splits in the transitive dependency set of the current one.
		r.log.Debugf("Copying over source 'go.mod' from %q to split %q located at %q.", r.fc.ModulePath(), s.Name, s.WorkDir)
		content := strings.Replace(r.mod, fmt.Sprintf("module %s", r.fc.ModulePath()), fmt.Sprintf("module %s", s.ModulePath), 1)
		if err := ioutil.WriteFile(modFile, []byte(content), 0644); err != nil {
			r.log.WithError(err).Errorf("Failed to write go.mod for split %q at %q.", s.Name, modFile)
			return err
		}
	} else {
		if _, err := os.Stat(modFile); err != nil && !os.IsNotExist(err) {
			r.log.WithError(err).Errorf("Failed to determine whether a 'go.mod' file already exists in split %q in %q.", s.Name, s.WorkDir)
			return err
		} else if os.IsNotExist(err) {
			cmd := exec.Command("go", "mod", "init", s.ModulePath)
			cmd.Dir = s.WorkDir
			if out, err := cmd.CombinedOutput(); err != nil {
				r.log.WithError(err).Errorf("Failed to initialise Go module for split %q in %q. Output was:\n%s", s.Name, s.WorkDir, out)
				return err
			}
		}
	}

	fd, err := os.OpenFile(modFile, os.O_WRONLY|os.O_EXCL|os.O_APPEND, 0644)
	if err != nil {
		r.log.WithError(err).Errorf("Failed to open go.mod file at %q.", modFile)
		return err
	}
	for sn := range deps {
		r.log.Debugf("Adding a temporary 'require' statement for a dependency on split %q stored at %q.", sn, r.sp.Splits[sn].WorkDir)
		_, err = fd.WriteString(fmt.Sprintf("\nreplace %s => %s %s", r.sp.Splits[sn].ModulePath, r.sp.Splits[sn].WorkDir, tempReplaceMarker))
		if err != nil {
			r.log.WithError(err).Errorf("Failed to append temporary 'replace' statement to go.mod at %q.", modFile)
			return err
		}
	}
	_ = fd.Close()

	// Clean up the split's 'go.mod' to remove any unnecessary dependencies copied over from the
	// source project.
	var splitPaths []string
	for sn := range s.SplitDeps {
		splitPaths = append(splitPaths, r.sp.Splits[sn].ModulePath)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = s.WorkDir

	r.log.Debugf("Pre-cleaning 'go mod tidy' on split %q located at %q using 'replace' statements.", s.Name, s.WorkDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.log.WithError(err).Errorf("Failed to clean up go.mod in %s via 'go mod tidy'.\nOutput was:\n%s", s.WorkDir, out)
		return err
	}
	return nil
}
