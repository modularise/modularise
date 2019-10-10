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

func InitSplits(l *logrus.Logger, sp *splits.Splits) error {
	if err := initWorkTree(l, sp); err != nil {
		return err
	}

	for _, s := range sp.Splits {
		if err := cloneRepository(l, s); err != nil {
			return err
		}
	}
	return nil
}

func initWorkTree(l *logrus.Logger, sp *splits.Splits) error {
	if sp.WorkTree == "" {
		td, err := ioutil.TempDir("", "modularise-splits")
		if err != nil {
			l.WithError(err).Error("Failed to instantiate a temporary directory to store split data.")
			return err
		}
		sp.WorkTree = td
	} else {
		if err := os.RemoveAll(sp.WorkTree); err != nil {
			l.WithError(err).Errorf("Failed to clean out existing content of work directory %q.", sp.WorkTree)
			return err
		}
		if err := os.MkdirAll(sp.WorkTree, 0755); err != nil {
			l.WithError(err).Errorf("Failed to (re)create the specified work directory %q.", sp.WorkTree)
		}
	}
	l.Debugf("Using directory %q for split data.", sp.WorkTree)

	for _, s := range sp.Splits {
		s.WorkDir = filepath.Join(sp.WorkTree, s.Name)
		if err := initSplit(l, s); err != nil {
			return err
		}
	}
	return nil
}

func initSplit(l *logrus.Logger, s *splits.Split) error {
	if _, err := os.Stat(s.WorkDir); err == nil {
		l.Errorf("Can not use directory %q to store data for split %q as it already exists.", s.WorkDir, s.Name)
		return fmt.Errorf("directory %q already exists", s.WorkDir)
	} else if !os.IsNotExist(err) {
		l.WithError(err).Errorf("Failed to assert that work directory %q for split %q exists.", s.WorkDir, s.Name)
		return err
	}

	if err := os.Mkdir(s.WorkDir, 0755); err != nil {
		l.WithError(err).Errorf("Failed to create directory %q to store data for split %q.", s.WorkDir, s.Name)
		return err
	}

	l.Debugf("Using directory %q to store data for split %q.", s.WorkDir, s.Name)
	return nil
}

func cloneRepository(l *logrus.Logger, s *splits.Split) error {
	if s.URL == "" {
		l.Infof("No remote configured for split %q. It won't be synced to a Git repository but its content will be stored at %q.", s.Name, s.WorkDir)
		return initRepository(l, s)
	}

	l.Debugf("Cloning remote repository from %q into %q for split %q.", s.URL, s.WorkDir, s.Name)
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
		l.WithError(err).Errorf("Failed to clone the remote repository from %q into %q for split %q.", s.URL, s.WorkDir, s.Name)
		return err
	}
	s.Repo = r

	l.Debugf("Cleaning out all existing content from git repository at %q for split %q.", s.WorkDir, s.Name)
	tld, err := wt.ReadDir(".")
	if err != nil {
		l.WithError(err).Errorf("Failed to read the content of the git working tree at %q for split %q.", s.WorkDir, s.Name)
		return err
	}

	for _, tle := range tld {
		if tle.Name() == ".git" {
			continue
		}
		if err = wt.Remove(tle.Name()); err != nil {
			l.WithError(err).Errorf("Failed to clean out top-level %q in git working tree at %q for split %q.", tle.Name(), s.WorkDir, s.Name)
			return err
		}
	}
	return nil
}

func initRepository(l *logrus.Logger, s *splits.Split) error {
	r, err := git.Init(filesystem.NewStorage(osfs.New(filepath.Join(s.WorkDir, ".git")), cache.NewObjectLRUDefault()), osfs.New(s.WorkDir))
	if err != nil {
		l.WithError(err).Errorf("Failed to initialise a new git repository inside %q for split %q.", s.WorkDir, s.Name)
		return err
	}
	s.Repo = r
	return nil
}
