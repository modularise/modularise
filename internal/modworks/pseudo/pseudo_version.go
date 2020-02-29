package pseudo

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"github.com/modularise/modularise/internal/splits"
)

type ProxyModuleInfo struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
	Hash    string    `json:"-"`
}

func Version(l *logrus.Logger, s *splits.Split) (*ProxyModuleInfo, error) {
	if s.Repo == nil {
		if s.Repo == nil {
			l.Errorf("Attempting to push new content for split %q in %q without having initialised a repository.", s.Name, s.WorkDir)
			return nil, fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
		}
	}
	return versioner{log: l, s: s}.pseudoVersion()
}

type versioner struct {
	log *logrus.Logger
	s   *splits.Split
}

func (v versioner) pseudoVersion() (*ProxyModuleInfo, error) {
	href, err := v.s.Repo.Head()
	if err != nil {
		v.log.WithError(err).Errorf("Failed to load the current HEAD in git repository at %q.", v.s.WorkDir)
		return nil, err
	}

	head, err := v.s.Repo.CommitObject(href.Hash())
	if err != nil {
		v.log.WithError(err).Errorf("Failed to retrieve HEAD commit info for git repository at %q.", v.s.WorkDir)
		return nil, err
	}

	major := "v0"
	if m := regexp.MustCompile(`^.*?(?:/(v[1-9][0-9]*))?$`).FindStringSubmatch(v.s.ModulePath); m[1] != "" {
		major = m[1]
	}

	baseVersion, err := v.baseVersionForCommit(major, head)
	if err != nil {
		return nil, err
	}

	version := fmt.Sprintf("%s-%s-%s", baseVersion, head.Committer.When.UTC().Format("20060102150405"), href.Hash().String()[:12])
	v.log.Debugf("Determined pseudo-version %q for split %q.", version, v.s.Name)
	return &ProxyModuleInfo{
		Version: version,
		Time:    head.Committer.When,
		Hash:    href.Hash().String(),
	}, nil
}

func (v versioner) baseVersionForCommit(major string, c *object.Commit) (string, error) {
	tags, tErr := v.tagsForMajor(major)
	if tErr != nil {
		return "", tErr
	}

	for _, tag := range tags {
		n := tag.Name().Short()

		// Resolve tag.
		var tc *object.Commit
		to, err := v.s.Repo.TagObject(tag.Hash())
		switch err {
		case nil:
			v.log.Debugf("Tag %q is an annotated reference.", n)
			tc, err = to.Commit()
		case plumbing.ErrObjectNotFound:
			v.log.Debugf("Tag %q is a light-weight reference.", n)
			tc, err = v.s.Repo.CommitObject(tag.Hash())
		default:
			v.log.WithError(err).Errorf("Could not retrieve tag object for %q.", n)
			return "", err
		}
		if err != nil {
			v.log.WithError(err).Errorf("Could not retrieve commit information for tag %q.", n)
			return "", err
		}

		// Determine ancestry.
		ok, err := tc.IsAncestor(c)
		if err != nil {
			v.log.WithError(err).Errorf("Failed to determine ancestor relationship between commit %q and tag %q.", c.Hash.String(), n)
			return "", err
		} else if !ok {
			v.log.Debugf("Tag %q is not an ancestor of %q.", n, c.Hash.String())
			continue
		}
		v.log.Debugf("Tag %q selected as highest semver release that is an ancestor of %q.", n, c.Hash.String())

		// Determine base version.
		patch, err := strconv.Atoi(strings.TrimPrefix(n, semver.MajorMinor(n)+"."))
		if err != nil {
			v.log.WithError(err).Errorf("Failed to determine patch version of tag %q.", n)
			return "", err
		}
		return fmt.Sprintf("%s.%d", semver.MajorMinor(n), patch+1), nil
	}
	return fmt.Sprintf("%s.0.0", major), nil
}

func (v versioner) tagsForMajor(major string) ([]*plumbing.Reference, error) {
	ti, err := v.s.Repo.Tags()
	if err != nil {
		v.log.WithError(err).Error("Failed to retrieve iterator over tags.")
		return nil, err
	}

	var tags []*plumbing.Reference
	err = ti.ForEach(func(tag *plumbing.Reference) error {
		n := tag.Name().Short()
		if !semver.IsValid(n) {
			v.log.Debugf("Not selecting tag %q - it is not valid semver.", n)
			return nil
		} else if semver.Prerelease(n) != "" || semver.Build(n) != "" {
			v.log.Debugf("Not selecting tag %q - it is not a stable release.", n)
			return nil
		}
		if (major > "v1" || semver.Major(n) > "v1") && major != semver.Major(n) {
			v.log.Debugf("Not selecting tag %q - it's major %q differs from the target %q.", n, semver.Major(n), major)
			return nil
		}
		v.log.Debugf("Selecting tag %q for major %q.", n, major)
		tags = append(tags, tag)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(tags, func(i int, j int) bool { return semver.Compare(tags[i].Name().Short(), tags[j].Name().Short()) > 0 })
	return tags, nil
}
