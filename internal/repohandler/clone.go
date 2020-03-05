package repohandler

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-git.v4"
	gitconfig "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/cache"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"

	"github.com/modularise/modularise/cmd/config"
)

const (
	defaultBranchName = "master"
	defaultRemoteName = "origin"
)

func InitSplits(log *logrus.Logger, sp *config.Splits) error {
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

func initWorkTree(log *logrus.Logger, sp *config.Splits) error {
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
		if err := initSplitDir(log, s); err != nil {
			return err
		}
	}
	return nil
}

func initSplitDir(log *logrus.Logger, s *config.Split) error {
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

func cloneRepository(log *logrus.Logger, s *config.Split, sp *config.Splits) error {
	if s.URL == "" {
		log.Infof("No remote configured for split %q. It won't be synced to a Git repository but its content will be stored at %q.", s.Name, s.WorkDir)
		return initRepository(log, s, sp)
	}

	log.Debugf("Extracting authentication information from config %v.", sp.Credentials)
	auth, err := sp.Credentials.ExtractAuth()
	if err != nil {
		log.WithError(err).Error("Could not setup authentication for Git operations.")
		return err
	}

	bn := s.Branch
	if bn == "" {
		bn = defaultBranchName
	}
	log.Debugf("Cloning remote repository from %q into %q for split %q.", s.URL, s.WorkDir, s.Name)
	r, err := git.PlainClone(
		s.WorkDir,
		false,
		&git.CloneOptions{
			Auth:          auth,
			URL:           s.URL,
			ReferenceName: plumbing.NewBranchReferenceName(bn),
			SingleBranch:  true,
		},
	)
	if err == transport.ErrEmptyRemoteRepository {
		return initRepository(log, s, sp)
	} else if err != nil {
		log.WithError(err).Errorf("Failed to clone the remote repository from %q into %q for split %q.", s.URL, s.WorkDir, s.Name)
		return err
	}
	s.Repo = r

	wt, err := s.Repo.Worktree()
	if err != nil {
		log.WithError(err).Errorf("Failed to open working tree of git repository for split %q located at %q.", s.Name, s.WorkDir)
		return err
	}

	log.Debugf("Cleaning out all existing content from git repository at %q for split %q.", s.WorkDir, s.Name)
	tld, err := wt.Filesystem.ReadDir(".")
	if err != nil {
		log.WithError(err).Errorf("Failed to read the content of the git working tree at %q for split %q.", s.WorkDir, s.Name)
		return err
	}

	for _, tle := range tld {
		if tle.Name() == ".git" {
			continue
		} else if sp.NonModuleSource && (tle.Name() == "go.mod" || tle.Name() == "go.sum") {
			// If the source project is not a Go module we want to preserve any existing module
			// context.
			continue
		}

		// We need to use the 'os.RemoveAll()' method instead of 'wt.Filesystem.Remove()' because
		// the latter will result in errors on non-empty directories.
		if err = os.RemoveAll(filepath.Join(wt.Filesystem.Root(), tle.Name())); err != nil {
			log.WithError(err).Errorf("Failed to clean out top-level %q in git working tree at %q for split %q.", tle.Name(), s.WorkDir, s.Name)
			return err
		}
	}
	return nil
}

func initRepository(log *logrus.Logger, s *config.Split, sp *config.Splits) error {
	r, err := git.Init(filesystem.NewStorage(osfs.New(filepath.Join(s.WorkDir, ".git")), cache.NewObjectLRUDefault()), osfs.New(s.WorkDir))
	if err != nil {
		log.WithError(err).Errorf("Failed to initialise a new git repository in %q.", s.WorkDir)
		return err
	}

	wt, err := r.Worktree()
	if err != nil {
		log.WithError(err).Errorf("Failed to obtain the worktree of the new git repository in %q.", s.WorkDir)
		return err
	}

	// We need to create an initial commit for references to be populated.
	h, err := wt.Commit("Initial commit", &git.CommitOptions{Author: sp.Author.ExtractAuthor()})
	if err != nil {
		log.WithError(err).Errorf("Failed to create initial empty commit in new git repository in %q.", s.WorkDir)
		return err
	}

	var gc *gitconfig.Config
	if gc, err = r.Config(); err != nil {
		log.WithError(err).Errorf("Failed to read configuration of new git repository in %q.", s.WorkDir)
		return err
	}

	bn := s.Branch
	if bn == "" {
		bn = defaultBranchName
	}
	br := &gitconfig.Branch{
		Name:  bn,
		Merge: plumbing.NewBranchReferenceName(bn),
	}
	gc.Branches[bn] = br

	brn := plumbing.NewBranchReferenceName(bn)
	if err = r.Storer.SetReference(plumbing.NewHashReference(brn, h)); err != nil {
		log.WithError(err).Errorf("Failed to set reference for branch %q to hash %q in new git repository in %q.", bn, h, s.WorkDir)
		return err
	}
	if err = wt.Checkout(&git.CheckoutOptions{Branch: brn}); err != nil {
		log.WithError(err).Errorf("Failed to checkout branch %q in new git repository in %q.", s.Branch, s.WorkDir)
		return err
	}

	if s.URL != "" {
		gc.Remotes[defaultRemoteName] = &gitconfig.RemoteConfig{
			Name: defaultRemoteName,
			URLs: []string{s.URL},
			Fetch: []gitconfig.RefSpec{
				gitconfig.RefSpec(fmt.Sprintf("+refs/heads/*:refs/remotes/%s/*", defaultRemoteName)),
			},
		}
		br.Remote = defaultRemoteName
	}

	gcb, err := gc.Marshal()
	if err != nil {
		log.WithError(err).Errorf("Failed to marshal updated git configuration for repository in %q. Config:\n%+v", s.WorkDir, gc)
		return err
	}
	gcp := filepath.Join(s.WorkDir, ".git", "config")
	if err = ioutil.WriteFile(gcp, gcb, 0644); err != nil {
		log.WithError(err).Errorf("Failed to write updated git configuation for repository to %q.", gcp)
		return err
	}

	s.Repo = r
	return nil
}
