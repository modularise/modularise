package repohandler

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"

	"github.com/modularise/modularise/internal/splits"
)

func PushSplits(l *logrus.Logger, sp *splits.Splits) error {
	for _, s := range sp.Splits {
		if s.Repo == nil {
			l.Errorf("Attempting to push new content for split %q in %q without having initialised a repository.", s.Name, s.WorkDir)
			return fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
		}
	}

	for _, s := range sp.Splits {
		if s.URL == "" {
			l.Infof("Not pushing content for split %q as there is no remote configured. The new content can be found in %q.", s.Name, s.WorkDir)
			continue
		}

		if err := s.Repo.Push(&git.PushOptions{
			RemoteName: "origin",
			RefSpecs: []config.RefSpec{
				config.RefSpec(fmt.Sprintf("%s:refs/remotes/origin/%s", s.Branch, s.Branch)),
			},
		}); err != nil {
			l.WithError(err).Errorf("Failed to push new split content for %q to the remote at %q.", s.Name, s.URL)
			return err
		}
	}
	return nil
}
