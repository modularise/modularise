package modworks

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/src-d/go-git.v4"

	"github.com/modularise/modularise/cmd/config"
)

func (r *resolver) resolveSplitDeps(s *config.Split) error {
	if err := r.cleanupGoMod(s); err != nil {
		return err
	}

	if err := r.commitChanges(s); err != nil {
		return err
	}
	return nil
}

func (r *resolver) cleanupGoMod(s *config.Split) error {
	modFile := filepath.Join(s.WorkDir, "go.mod")
	r.log.Debug("Cleaning up go.mod.", zap.String("file", modFile))

	c, err := ioutil.ReadFile(modFile)
	if err != nil {
		r.log.Error("Failed to read go.mod.", zap.String("file", modFile), zap.Error(err))
		return err
	}
	oldMod := strings.Split(string(c), "\n")

	newMod := make([]string, 0, len(oldMod))
	for _, l := range oldMod {
		if strings.HasSuffix(l, tempReplaceMarker) {
			r.log.Debug("Eliminated temporary line.", zap.String("line", l))
			continue
		}

		for dn := range s.SplitDeps {
			// We need to filter against the module-path suffixed with a space to deal with nested modules.
			if strings.HasPrefix(strings.TrimSpace(l), r.sp.Splits[dn].ModulePath+" ") {
				r.log.Debug(
					"Adding dependency on version of split.",
					zap.String("split-module", r.sp.Splits[dn].ModulePath),
					zap.String("version", r.sp.Splits[dn].Version),
				)
				l = fmt.Sprintf("\t%s %s", r.sp.Splits[dn].ModulePath, r.sp.Splits[dn].Version)
				break
			}
		}
		newMod = append(newMod, l)
	}

	if err = ioutil.WriteFile(modFile, []byte(strings.Join(newMod, "\n")), 0644); err != nil {
		r.log.Error("Failed to write modified content of go.mod.", zap.String("file", modFile), zap.Error(err))
		return err
	}

	cmd := exec.Command("go", "env", "GOPROXY")
	cmd.Env = append(os.Environ(), "GODEBUG=") // Don't pass any debug options to the lower-level invocation.
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.log.Error("Failed to determine current GORPOXY value via 'go env'.", zap.Error(err))
		return err
	}

	var splitPaths []string
	for sn := range s.SplitDeps {
		splitPaths = append(splitPaths, r.sp.Splits[sn].ModulePath)
	}

	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = s.WorkDir
	cmd.Env = append(
		os.Environ(),
		"GODEBUG=", // Don't pass any debug options to the lower-level invocation.
		fmt.Sprintf("GONOSUMDB=%s", strings.Join(splitPaths, ",")),
		fmt.Sprintf("GOPROXY=file://%s", r.localProxy),
	)

	r.log.Debug("Running 'go mod tidy' using definitive versions.", zap.String("directory", s.WorkDir))
	out, err = cmd.CombinedOutput()
	if err != nil {
		r.log.Error("Failed to clean up go.mod.", zap.String("file", modFile), zap.ByteString("output", out))
		return err
	}
	return nil
}

func (r *resolver) commitChanges(s *config.Split) error {
	if s.Repo == nil {
		r.log.Error(
			"Attempting to push new split content without having initialised a repository.",
			zap.String("split", s.Name),
			zap.String("directory", s.WorkDir),
		)
		return fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
	}

	wt, err := s.Repo.Worktree()
	if err != nil {
		r.log.Error("Failed to load a git working tree.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	if err = wt.AddGlob("."); err != nil {
		r.log.Error("Failed to add new, deleted or modified files to repository staging area.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	st, err := wt.Status()
	if err != nil {
		r.log.Error("Failed to get git status.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	var dirty bool
	for _, fs := range st {
		if fs.Staging != git.Unmodified {
			dirty = true
			break
		}
	}
	if !dirty {
		return nil
	}

	_, err = wt.Commit(
		fmt.Sprintf("Splice from %s@%s", r.fc.ModulePath(), r.sourceVer),
		&git.CommitOptions{
			All:    true,
			Author: r.sp.Author.ExtractAuthor(),
		},
	)
	if err != nil {
		r.log.Error("Failed to commit changes.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}
	return nil
}
