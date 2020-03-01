package pseudo

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/Helcaraxan/modularise/cmd/config"
	"github.com/Helcaraxan/modularise/internal/splits"
	"github.com/Helcaraxan/modularise/internal/testlib"
	"github.com/Helcaraxan/modularise/internal/testrepo"
)

func TestPseudoVersion(t *testing.T) {
	t.Parallel()

	tcs := map[string]struct {
		actions []testrepo.RepoAction
		module  string
		prefix  string
	}{
		"NoTags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
			},
			module: "fake.com/mod",
			prefix: "v0.0.0",
		},
		"V1Tag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("v1.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
			},
			module: "fake.com/mod",
			prefix: "v1.0.1",
		},
		"V2Tag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("v1.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
				testrepo.LightTag("v2.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "util.go", Content: []byte("mooh")}),
				testrepo.Commit("Third commit"),
			},
			module: "fake.com/mod/v2",
			prefix: "v2.0.1",
		},
		"V2NoTag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("v1.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
			},
			module: "fake.com/mod/v2",
			prefix: "v2.0.0",
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			repo := testrepo.CreateTestRepo(t, tc.actions)

			td, err := ioutil.TempDir("", "modularise-pseudo-version-test")
			testlib.NoError(t, true, err)
			t.Logf("Test-directory: %s", td)
			defer func() { testlib.NoError(t, false, os.RemoveAll(td)) }()

			repo.WriteToDisk(filepath.Join(td, "repo"))

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			info, err := Version(l, &config.Split{
				ModulePath: tc.module,
				DataSplit: splits.DataSplit{
					WorkDir: repo.Path(),
					Repo:    repo.Repository(),
				},
			})
			testlib.NoError(t, true, err)

			ev := fmt.Sprintf("%s-%s-%s", tc.prefix, repo.Head().Committer.When.UTC().Format("20060102150405"), repo.Head().Hash.String()[:12])
			testlib.Equal(t, false, ev, info.Version)
			testlib.Equal(t, false, repo.Head().Committer.When.UTC(), info.Time.UTC())
		})
	}
}

func TestBaseVersionForCommit(t *testing.T) {
	t.Parallel()

	tcs := map[string]struct {
		actions     []testrepo.RepoAction
		major       string
		baseVersion string
	}{
		"NoTag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
			},
			major:       "v0",
			baseVersion: "v0.0.0",
		},
		"V0Tag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.AnnotatedTag("v0.1.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
			},
			major:       "v0",
			baseVersion: "v0.1.1",
		},
		"V2Tag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.AnnotatedTag("v1.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
				testrepo.AnnotatedTag("v2.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "util.go", Content: []byte("mooh")}),
				testrepo.Commit("Third commit"),
			},
			major:       "v2",
			baseVersion: "v2.0.1",
		},
		"V2ButNoTag": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.AnnotatedTag("v1.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
			},
			major:       "v2",
			baseVersion: "v2.0.0",
		},
		"V2TagButV1Major": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.AnnotatedTag("v1.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
				testrepo.AnnotatedTag("v2.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "util.go", Content: []byte("mooh")}),
				testrepo.Commit("Third commit"),
			},
			major:       "v1",
			baseVersion: "v1.0.1",
		},
		"MixedTags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("v2.0.0"),
				testrepo.AnnotatedTag("v3.0.0"),
				testrepo.AddFile(testrepo.RepoFile{Path: "lib.go", Content: []byte("bar")}),
				testrepo.Commit("Second commit"),
				testrepo.LightTag("v3.0.1"),
				testrepo.AddFile(testrepo.RepoFile{Path: "util.go", Content: []byte("mooh")}),
				testrepo.Commit("Third commit"),
			},
			major:       "v3",
			baseVersion: "v3.0.2",
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			repo := testrepo.CreateTestRepo(t, tc.actions)

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			bv, err := versioner{
				log: l,
				s: &config.Split{
					DataSplit: splits.DataSplit{
						WorkDir: repo.Path(),
						Repo:    repo.Repository(),
					},
				},
			}.baseVersionForCommit(tc.major, repo.Head())
			testlib.NoError(t, true, err)
			testlib.Equal(t, false, tc.baseVersion, bv)
		})
	}
}

func TestTagsForMajor(t *testing.T) {
	t.Parallel()

	tcs := map[string]struct {
		actions []testrepo.RepoAction
		major   string
		tags    []string
	}{
		"v0v1NoTags": {major: "v0"},
		"v2NoTags":   {major: "v2"},
		"v0v1Tags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{
					Path:    "main.go",
					Content: []byte("foo"),
				}),
				testrepo.Commit("Test commit"),
				testrepo.AnnotatedTag("v0.1.0"),
				testrepo.AnnotatedTag("v0.1.1"),
				testrepo.AnnotatedTag("v1.0.0"),
				testrepo.AnnotatedTag("v2.0.0"),
			},
			major: "v0",
			tags: []string{
				"v1.0.0",
				"v0.1.1",
				"v0.1.0",
			},
		},
		"v2Tags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{
					Path:    "main.go",
					Content: []byte("foo"),
				}),
				testrepo.Commit("Test commit"),
				testrepo.AnnotatedTag("v0.1.0"),
				testrepo.AnnotatedTag("v0.1.1"),
				testrepo.AnnotatedTag("v1.0.0"),
				testrepo.AnnotatedTag("v2.0.0"),
			},
			major: "v2",
			tags: []string{
				"v2.0.0",
			},
		},
		"LightTags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("v0.1.0"),
				testrepo.LightTag("v0.1.1"),
				testrepo.LightTag("v1.0.0"),
				testrepo.LightTag("v2.0.0"),
			},
			major: "v0",
			tags: []string{
				"v1.0.0",
				"v0.1.1",
				"v0.1.0",
			},
		},
		"NonSemVerTags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("foo"),
				testrepo.LightTag("v0.1.1.2"),
				testrepo.LightTag("v1.02.0"),
			},
			major: "v0",
		},
		"NonReleaseTags": {
			actions: []testrepo.RepoAction{
				testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("foo")}),
				testrepo.Commit("First commit"),
				testrepo.LightTag("v0.1.0-rc1"),
				testrepo.LightTag("v1.0.1+metadata"),
				testrepo.LightTag("v1.2.0-prelease+buildinfo"),
			},
			major: "v0",
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			repo := testrepo.CreateTestRepo(t, tc.actions)

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			l.ReportCaller = true

			tags, err := versioner{
				log: l,
				s: &config.Split{
					DataSplit: splits.DataSplit{
						WorkDir: repo.Path(),
						Repo:    repo.Repository(),
					},
				},
			}.tagsForMajor(tc.major)
			testlib.NoError(t, true, err)

			testlib.True(t, true, len(tc.tags) == len(tags))
			for i := range tc.tags {
				testlib.Equal(t, false, tc.tags[i], tags[i].Name().Short())
			}
		})
	}
}
