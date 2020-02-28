package repohandler

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"
	gitconfig "gopkg.in/src-d/go-git.v4/config"

	"github.com/Helcaraxan/modularise/internal/splits"
)

func PushSplits(log *logrus.Logger, sp *splits.Splits) error {
	for _, s := range sp.Splits {
		if s.Repo == nil {
			log.Errorf("Attempting to push new content for split %q in %q without having initialised a repository.", s.Name, s.WorkDir)
			return fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
		}
	}

	log.Debugf("Extracting authentication information from config %v.", sp.Credentials)
	auth, err := sp.Credentials.ExtractAuth()
	if err != nil {
		log.WithError(err).Error("Could not setup authentication for Git operations.")
		return err
	}

	for _, s := range sp.Splits {
		if s.URL == "" {
			continue
		}

		bn := s.Branch
		if bn == "" {
			bn = defaultBranchName
		}
		po := git.PushOptions{
			Auth:       auth,
			RemoteName: defaultRemoteName,
			RefSpecs: []gitconfig.RefSpec{
				gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", bn, defaultRemoteName, bn)),
			},
		}
		if err = s.Repo.Push(&po); err != nil {
			log.WithError(err).Errorf("Failed to push new split content for %q to the remote at %q.", s.Name, s.URL)
			return err
		}
	}
	return nil
}
