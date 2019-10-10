package modworks

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"github.com/Helcaraxan/modularise/internal/splits"
)

func (r *resolver) resolveSplitDeps(s *splits.Split) error {
	if err := r.cleanupGoMod(s); err != nil {
		return err
	}

	if err := r.commitChanges(s); err != nil {
		return err
	}
	return nil
}

func (r *resolver) cleanupGoMod(s *splits.Split) error {
	r.log.Debugf("Cleaning up go.mod for split %q located in %q.", s.Name, s.WorkDir)

	c, err := ioutil.ReadFile(filepath.Join(s.WorkDir, "go.mod"))
	if err != nil {
		r.log.WithError(err).Errorf("Failed to read go.mod for split in %q.", s.WorkDir)
		return err
	}
	oldMod := strings.Split(string(c), "\n")

	newMod := make([]string, 0, len(oldMod))
	for _, l := range oldMod {
		if strings.HasSuffix(l, tempReplaceMarker) {
			r.log.Debugf("Eliminated temporary line %q.", l)
			continue
		}

		for dn := range s.SplitDeps {
			// We need to filter against the module-path suffixed with a space to deal with nested modules.
			if strings.HasPrefix(strings.TrimSpace(l), r.sp.Splits[dn].ModulePath+" ") {
				r.log.Debugf("Adding dependency on split %q at version %q.", dn, r.sp.Splits[dn].Version)
				l = fmt.Sprintf("\t%s %s", r.sp.Splits[dn].ModulePath, r.sp.Splits[dn].Version)
				break
			}
		}
		newMod = append(newMod, l)
	}

	if err = ioutil.WriteFile(filepath.Join(s.WorkDir, "go.mod"), []byte(strings.Join(newMod, "\n")), 0644); err != nil {
		r.log.WithError(err).Errorf("Failed to write modified content of go.mod for split in %q.", s.WorkDir)
		return err
	}

	out, err := exec.Command("go", "env", "GOPROXY").CombinedOutput()
	if err != nil {
		r.log.WithError(err).Error("Failed to determine current GORPOXY value via 'go env'.")
		return err
	}

	var splitPaths []string
	for sn := range s.SplitDeps {
		splitPaths = append(splitPaths, r.sp.Splits[sn].ModulePath)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = s.WorkDir
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("GONOSUMDB=%s", strings.Join(splitPaths, ",")),
		fmt.Sprintf("GOPROXY=file://%s", r.localProxy),
	)

	r.log.Debugf("Pre-cleaning 'go mod tidy' on split %q located at %q using definitive versions.", s.Name, s.WorkDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		r.log.WithError(err).Errorf("Failed to clean up go.mod for split in %q.\nOutput was:\n%s", s.WorkDir, out)
		return err
	}
	return nil
}

func (r *resolver) commitChanges(s *splits.Split) error {
	if s.Repo == nil {
		r.log.Errorf("Attempting to push new content for split %q in %q without having initialised a repository.", s.Name, s.WorkDir)
		return fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
	}

	wt, err := s.Repo.Worktree()
	if err != nil {
		r.log.WithError(err).Errorf("Failed to load the git working tree for split %q at %q.", s.Name, s.WorkDir)
		return err
	}

	if err = wt.AddGlob("."); err != nil {
		r.log.WithError(err).Errorf("Failed to add all new files to the index in %q.", s.WorkDir)
		return err
	}

	_, err = wt.Commit(
		fmt.Sprintf("Splice from %s@%s", r.fc.ModulePath(), r.sourceVer),
		&git.CommitOptions{
			All: true,
			Author: &object.Signature{
				Name:  "Modularise",
				Email: "modularise@modularise.com",
				When:  time.Now(),
			},
		},
	)
	if err != nil {
		r.log.WithError(err).Errorf("Failed to commit changes to split in %q.", s.WorkDir)
		return err
	}
	return nil
}
