package repohandler

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/cache"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"

	"github.com/Helcaraxan/modularise/internal/splits"
)

func InitSplits(log *logrus.Logger, sp *splits.Splits) error {
	if err := initWorkTree(log, sp); err != nil {
		return err
	}

	for _, s := range sp.Splits {
		if err := cloneRepository(log, s, sp); err != nil {
			return err
		}
	}
	return nil
}

func initWorkTree(log *logrus.Logger, sp *splits.Splits) error {
	if sp.WorkTree == "" {
		td, err := ioutil.TempDir("", "modularise-splits")
		if err != nil {
			log.WithError(err).Error("Failed to instantiate a temporary directory to store split data.")
			return err
		}
		sp.WorkTree = td
	} else {
		if err := os.RemoveAll(sp.WorkTree); err != nil {
			log.WithError(err).Errorf("Failed to clean out existing content of work directory %q.", sp.WorkTree)
			return err
		}
		if err := os.MkdirAll(sp.WorkTree, 0755); err != nil {
			log.WithError(err).Errorf("Failed to (re)create the specified work directory %q.", sp.WorkTree)
		}
	}
	log.Debugf("Using directory %q for split data.", sp.WorkTree)

	for _, s := range sp.Splits {
		s.WorkDir = filepath.Join(sp.WorkTree, s.Name)
		if err := initSplit(log, s); err != nil {
			return err
		}
	}
	return nil
}

func initSplit(log *logrus.Logger, s *splits.Split) error {
	if _, err := os.Stat(s.WorkDir); err == nil {
		log.Errorf("Can not use directory %q to store data for split %q as it already exists.", s.WorkDir, s.Name)
		return fmt.Errorf("directory %q already exists", s.WorkDir)
	} else if !os.IsNotExist(err) {
		log.WithError(err).Errorf("Failed to assert that work directory %q for split %q exists.", s.WorkDir, s.Name)
		return err
	}

	if err := os.Mkdir(s.WorkDir, 0755); err != nil {
		log.WithError(err).Errorf("Failed to create directory %q to store data for split %q.", s.WorkDir, s.Name)
		return err
	}

	log.Debugf("Using directory %q to store data for split %q.", s.WorkDir, s.Name)
	return nil
}

func cloneRepository(log *logrus.Logger, s *splits.Split, sp *splits.Splits) error {
	if s.URL == "" {
		log.Infof("No remote configured for split %q. It won't be synced to a Git repository but its content will be stored at %q.", s.Name, s.WorkDir)
		return initRepository(log, s)
	}

	log.Debugf("Cloning remote repository from %q into %q for split %q.", s.URL, s.WorkDir, s.Name)
	wt := osfs.New(s.WorkDir)
	r, err := git.Clone(
		filesystem.NewStorage(osfs.New(filepath.Join(s.WorkDir, ".git")), cache.NewObjectLRUDefault()),
		wt,
		&git.CloneOptions{
			URL:           s.URL,
			ReferenceName: plumbing.NewBranchReferenceName(s.Branch),
			SingleBranch:  true,
		},
	)
	if err != nil {
		log.WithError(err).Errorf("Failed to clone the remote repository from %q into %q for split %q.", s.URL, s.WorkDir, s.Name)
		return err
	}
	s.Repo = r

	log.Debugf("Cleaning out all existing content from git repository at %q for split %q.", s.WorkDir, s.Name)
	tld, err := wt.ReadDir(".")
	if err != nil {
		log.WithError(err).Errorf("Failed to read the content of the git working tree at %q for split %q.", s.WorkDir, s.Name)
		return err
	}

	for _, tle := range tld {
		if tle.Name() == ".git" {
			continue
		} else if sp.NonModuleSource && (tle.Name() == "go.mod" || tle.Name() == "go.sum") {
			// If the source project is not a Go module we want to preserve any existing module context.
			continue
		}

		if err = wt.Remove(tle.Name()); err != nil {
			log.WithError(err).Errorf("Failed to clean out top-level %q in git working tree at %q for split %q.", tle.Name(), s.WorkDir, s.Name)
			return err
		}
	}
	return nil
}

func initRepository(log *logrus.Logger, s *splits.Split) error {
	r, err := git.Init(filesystem.NewStorage(osfs.New(filepath.Join(s.WorkDir, ".git")), cache.NewObjectLRUDefault()), osfs.New(s.WorkDir))
	if err != nil {
		log.WithError(err).Errorf("Failed to initialise a new git repository inside %q for split %q.", s.WorkDir, s.Name)
		return err
	}
	s.Repo = r
	return nil
}
