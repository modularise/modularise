package modworks

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"go.uber.org/zap"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
)

// CreateSplitModules iterates over the configures splits and initialise a Go module in each split's
// working directory.
//
// The prequisites on the fields of a config.Splits object for CreateSplitModules to be able to
// operate are:
//  - NonModuleSource is set to true if relevant.
//  - For each config.Split in Splits the Name, SplitDeps fields are populated.
//  - For each config.Split in Splits the WorkDir field is populated and corrresponds to an existing directory.
//  - For each config.Split in Splits the Repo field is populated and corrresponds to an existing repository.
func CreateSplitModules(log *zap.Logger, fc filecache.FileCache, sp *config.Splits) error {
	if !sp.NonModuleSource {
		// Ensure the module-cache is preheated such that future runs of 'go mod tidy' can be done with
		// only a temporary and partial local module proxy with split content.
		log.Debug("Pre-heating the module cache by running 'go mod tidy' on the source project.", zap.String("directory", fc.Root()))
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = fc.Root()
		cmd.Env = append(os.Environ(), "GODEBUG=") // Don't pass any debug options to the lower-level invocation.
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Error(
				"Failed to run 'go mod tidy' on source project",
				zap.String("directory", fc.Root()),
				zap.ByteString("output", out),
				zap.Error(err),
			)
			return err
		}
	}

	r, err := setupResolver(log, fc, sp)
	if err != nil {
		return err
	}

	for sn := range sp.Splits {
		if err = r.createSplitModule(sp.Splits[sn], []string{sn}); err != nil {
			return err
		}
	}
	return nil
}

type resolver struct {
	log        *zap.Logger
	fc         filecache.FileCache
	sp         *config.Splits
	mod        string
	sourceVer  string
	localProxy string
	done       map[string]bool
	todo       map[string]bool
	transDeps  map[string]map[string]bool
}

func setupResolver(log *zap.Logger, fc filecache.FileCache, sp *config.Splits) (*resolver, error) {
	var err error
	var smc []byte
	if !sp.NonModuleSource {
		sm := filepath.Join(fc.Root(), "go.mod")
		smc, err = ioutil.ReadFile(sm)
		if err != nil {
			log.Error("Failed to read the source go.mod file.", zap.String("file", sm), zap.Error(err))
		}
	}

	repo, err := git.PlainOpen(fc.Root())
	if err != nil {
		log.Error("Could not open the source project's git repository.", zap.String("directory", fc.Root()), zap.Error(err))
		return nil, err
	}

	h, err := repo.Head()
	if err != nil {
		log.Error("Could not determine the source project's HEAD commit.", zap.String("directory", fc.Root()), zap.Error(err))
		return nil, err
	}

	lpp, err := ioutil.TempDir("", "modularise-local-proxy")
	if err != nil {
		log.Error("Could not create directory for temporary local module proxy content.", zap.Error(err))
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
		transDeps:  map[string]map[string]bool{},
	}, nil
}

const tempReplaceMarker = "// modularise"

func (r *resolver) createSplitModule(s *config.Split, stack []string) error {
	if r.done[s.Name] {
		return nil
	} else if r.todo[s.Name] {
		// Circular dependencies should have already been detected in the API analysis.
		r.log.Error("A circular dependency exists between the configured splits. This is not allowed.", zap.Strings("split-stack", stack))
		return errors.New("circular split dependency found")
	}
	r.todo[s.Name] = true
	defer func() {
		delete(r.todo, s.Name)
		r.done[s.Name] = true
	}()

	// Process upstream splits first.
	r.transDeps[s.Name] = map[string]bool{}
	for sn := range s.SplitDeps {
		if err := r.createSplitModule(r.sp.Splits[sn], append(stack, sn)); err != nil {
			return err
		}
		r.transDeps[s.Name][sn] = true
		for tsn := range r.transDeps[sn] {
			r.transDeps[s.Name][tsn] = true
		}
	}

	if err := r.initSplitModule(s); err != nil {
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

func (r *resolver) initSplitModule(s *config.Split) error {
	modFile := filepath.Join(s.WorkDir, "go.mod")
	if !r.sp.NonModuleSource {
		// We need to change the module path in the source project's go.mod file before writing it to
		// the split's working directory. We also need to add temporary local 'replace' statements for
		// each of the splits in the transitive dependency set of the current one.
		r.log.Debug("Copying over source 'go.mod' to split.", zap.String("file", modFile))
		var newContent []string
		for _, l := range strings.Split(r.mod, "\n") {
			if l == fmt.Sprintf("module %s", r.fc.ModulePath()) {
				newContent = append(newContent, fmt.Sprintf("module %s", s.ModulePath))
				continue
			}

			var skip bool
			for sn := range r.transDeps[s.Name] {
				if strings.Contains(strings.SplitAfter(l, "//")[0], r.sp.Splits[sn].ModulePath) {
					skip = true
					break
				}
			}
			if !skip {
				newContent = append(newContent, l)
			}
		}
		if err := ioutil.WriteFile(modFile, []byte(strings.Join(newContent, "\n")), 0644); err != nil {
			r.log.Error("Failed to write go.mod.", zap.String("file", modFile), zap.Error(err))
			return err
		}
	} else {
		if _, err := os.Stat(modFile); err != nil && !os.IsNotExist(err) {
			r.log.Error("Failed to determine whether a 'go.mod' file already exists.", zap.String("file", modFile), zap.Error(err))
			return err
		} else if os.IsNotExist(err) {
			cmd := exec.Command("go", "mod", "init", s.ModulePath)
			cmd.Dir = s.WorkDir
			cmd.Env = append(os.Environ(), "GODEBUG=") // Don't pass any debug options to the lower-level invocation.
			if out, err := cmd.CombinedOutput(); err != nil {
				r.log.Error("Failed to initialise Go module.", zap.String("directory", s.WorkDir), zap.ByteString("output", out))
				return err
			}
		}
	}

	fd, err := os.OpenFile(modFile, os.O_WRONLY|os.O_EXCL|os.O_APPEND, 0644)
	if err != nil {
		r.log.Error("Failed to open go.mod file.", zap.String("file", modFile), zap.Error(err))
		return err
	}
	for sn := range r.transDeps[s.Name] {
		r.log.Debug(
			"Adding a temporary 'require' statement for a dependency on split.",
			zap.String("target-split", sn),
			zap.String("target-directory", r.sp.Splits[sn].WorkDir),
		)
		_, err = fd.WriteString(fmt.Sprintf("\nreplace %s => %s %s", r.sp.Splits[sn].ModulePath, r.sp.Splits[sn].WorkDir, tempReplaceMarker))
		if err != nil {
			r.log.Error("Failed to append temporary 'replace' statement to go.mod.", zap.String("file", modFile), zap.Error(err))
			return err
		}
	}
	_ = fd.Close()

	// Clean up the split's 'go.mod' to remove any unnecessary dependencies copied over from the
	// source project.
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = s.WorkDir
	cmd.Env = append(os.Environ(), "GODEBUG=") // Don't pass any debug options to the lower-level invocation.

	r.log.Debug("Pre-cleaning with 'go mod tidy' using 'replace' statements.", zap.String("split", s.Name), zap.String("directory", s.WorkDir))
	if out, err := cmd.CombinedOutput(); err != nil {
		r.log.Error("Failed to clean up go.mod via 'go mod tidy'.", zap.String("directory", s.WorkDir), zap.ByteString("output", out), zap.Error(err))
		return err
	}
	return nil
}
