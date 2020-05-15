package pseudo

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"go.uber.org/zap"
	"golang.org/x/mod/semver"

	"github.com/modularise/modularise/cmd/config"
)

type ProxyModuleInfo struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
	Hash    string    `json:"-"`
}

func Version(l *zap.Logger, s *config.Split) (*ProxyModuleInfo, error) {
	if s.Repo == nil {
		if s.Repo == nil {
			l.Error(
				"Attempting to push new content for split without having initialised a repository.",
				zap.String("split", s.Name),
				zap.String("directory", s.WorkDir),
			)
			return nil, fmt.Errorf("split %q in %q has no initialised repository", s.Name, s.WorkDir)
		}
	}
	return versioner{log: l.With(zap.String("split", s.Name), zap.String("directory", s.WorkDir)), s: s}.pseudoVersion()
}

type versioner struct {
	log *zap.Logger
	s   *config.Split
}

func (v versioner) pseudoVersion() (*ProxyModuleInfo, error) {
	href, err := v.s.Repo.Head()
	if err != nil {
		v.log.Error("Failed to load the current HEAD in git repository.", zap.Error(err))
		return nil, err
	}

	head, err := v.s.Repo.CommitObject(href.Hash())
	if err != nil {
		v.log.Error("Failed to retrieve HEAD commit info for git repository.", zap.Error(err))
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
	v.log.Debug("Determined pseudo-version.", zap.String("pseudo-version", version))
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
			tc, err = to.Commit()
		case plumbing.ErrObjectNotFound:
			tc, err = v.s.Repo.CommitObject(tag.Hash())
		default:
			v.log.Error("Could not retrieve tag object.", zap.String("tag", n), zap.Error(err))
			return "", err
		}
		if err != nil {
			v.log.Error("Could not retrieve commit information for tag.", zap.String("tag", n), zap.Error(err))
			return "", err
		}

		// Determine ancestry.
		ok, err := tc.IsAncestor(c)
		if err != nil {
			v.log.Error("Failed to determine ancestor relationship.", zap.String("commit", c.Hash.String()), zap.String("tag", n), zap.Error(err))
			return "", err
		} else if !ok {
			v.log.Debug("Commit is not an ancestor of tag.", zap.String("commit", c.Hash.String()), zap.String("tag", n))
			continue
		}
		v.log.Debug(
			"Tag selected as highest semver release that is an ancestor of commit.",
			zap.String("tag", n),
			zap.String("commit", c.Hash.String()),
		)

		// Determine base version.
		patch, err := strconv.Atoi(strings.TrimPrefix(n, semver.MajorMinor(n)+"."))
		if err != nil {
			v.log.Error("Failed to determine patch version of tag.", zap.String("tag", n), zap.Error(err))
			return "", err
		}
		return fmt.Sprintf("%s.%d", semver.MajorMinor(n), patch+1), nil
	}
	return fmt.Sprintf("%s.0.0", major), nil
}

func (v versioner) tagsForMajor(major string) ([]*plumbing.Reference, error) {
	ti, err := v.s.Repo.Tags()
	if err != nil {
		v.log.Error("Failed to retrieve iterator over tags.", zap.Error(err))
		return nil, err
	}

	var tags []*plumbing.Reference
	err = ti.ForEach(func(tag *plumbing.Reference) error {
		n := tag.Name().Short()
		if !semver.IsValid(n) {
			v.log.Debug("Not selecting tag - it is not valid semver.", zap.String("tag", n))
			return nil
		} else if semver.Prerelease(n) != "" || semver.Build(n) != "" {
			v.log.Debug("Not selecting tag - it is not a stable release.", zap.String("tag", n))
			return nil
		}
		if (major > "v1" || semver.Major(n) > "v1") && major != semver.Major(n) {
			v.log.Debug(
				"Not selecting tag - it's major differs from the target one.",
				zap.String("tag", n),
				zap.String("tag-major", semver.Major(n)),
				zap.String("target-major", major),
			)
			return nil
		}
		v.log.Debug("Selecting tag for major.", zap.String("tag", n), zap.String("major", major))
		tags = append(tags, tag)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(tags, func(i int, j int) bool { return semver.Compare(tags[i].Name().Short(), tags[j].Name().Short()) > 0 })
	return tags, nil
}
