package repohandler

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.uber.org/zap"
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

// InitSplits iterates over the configured splits and initialises a working directory for each one
// of them in the configured WorkTree. If configured, the remote repository for each split is then
// fetched into this working directory. If the remote repository is empty or no remote is configured
// a new empty git repository is initialised instead.
//
// The prequisites on the fields of a config.Splits object for InitSplits to be able to operate are:
//  - The WorkTree field is populated and corresponds to an existing directory.
//  - For each config.Split in Splits the Name field has been populated.
//  - For each config.Split in Splits the WorkDir field is either unpopulated or it corrresponds to a non-existing or empty directory.
func InitSplits(log *zap.Logger, sp *config.Splits) error {
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

func initWorkTree(log *zap.Logger, sp *config.Splits) error {
	if sp.WorkTree == "" {
		td, err := ioutil.TempDir("", "modularise-splits")
		if err != nil {
			log.Error("Failed to instantiate a temporary directory to store split data.", zap.Error(err))
			return err
		}
		sp.WorkTree = td
	} else {
		if err := os.RemoveAll(sp.WorkTree); err != nil {
			log.Error("Failed to clean out existing content of working tree.", zap.String("directory", sp.WorkTree), zap.Error(err))
			return err
		}
		if err := os.MkdirAll(sp.WorkTree, 0755); err != nil {
			log.Error("Failed to (re)create the specified working tree.", zap.String("directory", sp.WorkTree), zap.Error(err))
		}
	}
	log.Debug("Created split data directory.", zap.String("directory", sp.WorkTree))

	for _, s := range sp.Splits {
		s.WorkDir = filepath.Join(sp.WorkTree, s.Name)
		if err := initSplitDir(log, s); err != nil {
			return err
		}
	}
	return nil
}

func initSplitDir(log *zap.Logger, s *config.Split) error {
	if _, err := os.Stat(s.WorkDir); err == nil {
		log.Error(
			"Can not use directory to store data for split as it already exists.",
			zap.String("split", s.Name),
			zap.String("directory", s.WorkDir),
		)
		return fmt.Errorf("directory %q already exists", s.WorkDir)
	} else if !os.IsNotExist(err) {
		log.Error(
			"Failed to assert that work directory for split exists.",
			zap.String("split", s.Name),
			zap.String("directory", s.WorkDir),
			zap.Error(err),
		)
		return err
	}

	if err := os.Mkdir(s.WorkDir, 0755); err != nil {
		log.Error(
			"Failed to create directory to store data for split.",
			zap.String("split", s.Name),
			zap.String("directory", s.WorkDir),
			zap.Error(err),
		)
		return err
	}

	log.Debug("Created directory to store data for split.", zap.String("split", s.Name), zap.String("directory", s.WorkDir))
	return nil
}

func cloneRepository(log *zap.Logger, s *config.Split, sp *config.Splits) error {
	if s.URL == "" {
		log.Info(
			"No remote configured for split. It won't be synced to a Git repository but its content will be stored locally.",
			zap.String("split", s.Name),
			zap.String("directory", s.WorkDir),
		)
		return initRepository(log, s, sp)
	}

	auth, err := sp.Credentials.ExtractAuth()
	if err != nil {
		log.Error("Could not determine authentication for Git operations.", zap.Error(err))
		return err
	}

	bn := s.Branch
	if bn == "" {
		bn = defaultBranchName
	}
	log.Debug("Cloning remote repository.", zap.String("directory", s.WorkDir), zap.String("url", s.URL))
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
		log.Error("Failed to clone repository.", zap.String("directory", s.WorkDir), zap.String("url", s.URL), zap.Error(err))
		return err
	}
	s.Repo = r

	wt, err := s.Repo.Worktree()
	if err != nil {
		log.Error("Failed to open a git repository's working tree.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	log.Debug("Cleaning out all existing content from git repository.", zap.String("directory", s.WorkDir))
	tld, err := wt.Filesystem.ReadDir(".")
	if err != nil {
		log.Error("Failed to read the content of a git working tree.", zap.String("directory", s.WorkDir), zap.Error(err))
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
			log.Error(
				"Failed to clean out top-level element in git working tree.", zap.String("path", tle.Name()), zap.Error(err))
			return err
		}
	}
	return nil
}

func initRepository(log *zap.Logger, s *config.Split, sp *config.Splits) error {
	r, err := git.Init(filesystem.NewStorage(osfs.New(filepath.Join(s.WorkDir, ".git")), cache.NewObjectLRUDefault()), osfs.New(s.WorkDir))
	if err != nil {
		log.Error("Failed to initialise a new git repository.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	wt, err := r.Worktree()
	if err != nil {
		log.Error("Failed to obtain the worktree of a git repository.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	// We need to create an initial commit for references to be populated.
	h, err := wt.Commit("Initial commit", &git.CommitOptions{Author: sp.Author.ExtractAuthor()})
	if err != nil {
		log.Error("Failed to create initial empty commit in git repository.", zap.String("directory", s.WorkDir), zap.Error(err))
		return err
	}

	var gc *gitconfig.Config
	if gc, err = r.Config(); err != nil {
		log.Error("Failed to read configuration of git repository.", zap.String("directory", s.WorkDir), zap.Error(err))
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
		log.Error(
			"Failed to set reference for branch to hash in git repository.",
			zap.String("directory", s.WorkDir),
			zap.String("branch", br.Name),
			zap.String("hash", h.String()),
			zap.Error(err),
		)
		return err
	}
	if err = wt.Checkout(&git.CheckoutOptions{Branch: brn}); err != nil {
		log.Error("Failed to checkout branch in git repository.", zap.String("directory", s.WorkDir), zap.String("branch", br.Name), zap.Error(err))
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
		log.Error("Failed to marshal updated git configuration", zap.Any("configuratio", gc), zap.Error(err))
		return err
	}
	gcp := filepath.Join(s.WorkDir, ".git", "config")
	if err = ioutil.WriteFile(gcp, gcb, 0644); err != nil {
		log.Error("Failed to write updated git configuration to disk.", zap.String("file", gcp), zap.Error(err))
		return err
	}

	s.Repo = r
	return nil
}
