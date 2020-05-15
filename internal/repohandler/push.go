package repohandler

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"go.uber.org/zap"

	modularise_config "github.com/modularise/modularise/cmd/config"
)

// PushSplits iterates over the configured splits and, if they have a remote repository configured,
// pushed any new local content to the target branch.
//
// The prequisites on the fields of a config.Splits object for PushSplits to be able to operate are:
//  - For each config.Split in Splits the WorkDir field is populated and corrresponds to an existing directory.
//  - For each config.Split in Splits the Repo field is populated and corrresponds to an existing repository.
func PushSplits(log *zap.Logger, sp *modularise_config.Splits) error {
	for _, s := range sp.Splits {
		if s.Repo == nil {
			log.Error("Attempting to push new content without having initialised a repository.", zap.String("directory", s.WorkDir))
			return fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
		}
	}

	auth, err := sp.Credentials.ExtractAuth()
	if err != nil {
		log.Error("Could not set up authentication for Git operations.", zap.Error(err))
		return err
	}

	for _, s := range sp.Splits {
		if s.URL == "" {
			continue
		}

		err = s.Repo.Push(&git.PushOptions{Auth: auth})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			log.Error("Failed to push new split content to remote.", zap.String("directory", s.WorkDir), zap.String("url", s.URL))
			return err
		}
	}
	return nil
}
