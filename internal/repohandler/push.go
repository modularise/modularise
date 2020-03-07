package repohandler

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"

	modularise_config "github.com/modularise/modularise/cmd/config"
)

// PushSplits iterates over the configured splits and, if they have a remote repository configured,
// pushed any new local content to the target branch.
//
// The prequisites on the fields of a config.Splits object for PushSplits to be able to operate are:
//  - For each config.Split in Splits the WorkDir field is populated and corrresponds to an existing directory.
//  - For each config.Split in Splits the Repo field is populated and corrresponds to an existing repository.
func PushSplits(log *logrus.Logger, sp *modularise_config.Splits) error {
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

		if err = s.Repo.Push(&git.PushOptions{Auth: auth}); err != nil {
			log.WithError(err).Errorf("Failed to push new split content for %q to the remote at %q.", s.Name, s.URL)
			return err
		}
	}
	return nil
}
