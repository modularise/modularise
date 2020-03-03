package repohandler

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"gopkg.in/src-d/go-git.v4"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
	"github.com/modularise/modularise/internal/testrepo"
)

func TestInitRepository(t *testing.T) {
	t.Parallel()

	tcs := map[string]struct {
		config string
		branch string
	}{
		"DefaultBranch": {
			config: "",
			branch: defaultBranchName,
		},
		"CustomBranch": {
			config: "test-branch",
			branch: "test-branch",
		},
	}

	for n := range tcs {
		tc := tcs[n]
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			td, err := ioutil.TempDir("", "modularise-test-repository")
			testlib.NoError(t, true, err)
			defer func() { testlib.NoError(t, false, os.RemoveAll(td)) }()

			logger := logrus.New()
			logger.SetLevel(logrus.DebugLevel)
			logger.SetReportCaller(true)

			err = initRepository(logger, &config.Split{
				Branch: tc.config,
				DataSplit: splits.DataSplit{
					Name:    "a",
					WorkDir: td,
				},
			})
			testlib.NoError(t, true, err)
			r, err := git.PlainOpen(td)
			testlib.NoError(t, true, err)
			_, err = r.Branch(tc.branch)
			testlib.NoError(t, false, err)
		})
	}
}

func TestCloneRepository(t *testing.T) {
	t.Parallel()

	td, err := ioutil.TempDir("", "modularise-test-repository")
	testlib.NoError(t, true, err)
	defer func() { testlib.NoError(t, false, os.RemoveAll(td)) }()

	tr := testrepo.CreateTestRepo(t, []testrepo.RepoAction{
		testrepo.AddFile(testrepo.RepoFile{
			Path:    "file.txt",
			Content: []byte("file"),
		}),
		testrepo.Commit("First commit"),
	})
	tr.WriteToDisk(filepath.Join(td, "source"))

	he := tr.Head()

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetReportCaller(true)

	testlib.NoError(t, true, os.Mkdir(filepath.Join(td, "target"), 0755))

	err = cloneRepository(logger, &config.Split{
		URL:       fmt.Sprintf("file://%s", tr.Path()),
		DataSplit: splits.DataSplit{Name: "test-split", WorkDir: filepath.Join(td, "target")},
	}, &config.Splits{})
	testlib.NoError(t, true, err)

	r, err := git.PlainOpen(filepath.Join(td, "target"))
	testlib.NoError(t, true, err)
	hr, err := r.Head()
	testlib.NoError(t, true, err)
	hc, err := r.CommitObject(hr.Hash())
	testlib.NoError(t, true, err)
	testlib.Equal(t, false, he.Hash, hc.Hash)
}

func TestInitWorkTree(t *testing.T) {
	t.Parallel()

	sns := []string{"a", "b"}

	t.Run("New", func(t *testing.T) {
		t.Parallel()

		sp := config.Splits{Splits: map[string]*config.Split{}}
		for _, sn := range sns {
			sp.Splits[sn] = &config.Split{DataSplit: splits.DataSplit{Name: sn}}
		}

		testInitWorkTree(t, &sp)
	})

	t.Run("New", func(t *testing.T) {
		t.Parallel()

		sp := config.Splits{Splits: map[string]*config.Split{}}
		for _, sn := range sns {
			sp.Splits[sn] = &config.Split{DataSplit: splits.DataSplit{Name: sn}}
		}

		td, err := ioutil.TempDir("", "modularise-test-worktree")
		testlib.NoError(t, true, err)

		sp.WorkTree = td
		testInitWorkTree(t, &sp)

		testlib.Equal(t, false, td, sp.WorkTree)
		testlib.NoError(t, false, os.RemoveAll(td))
	})
}

func testInitWorkTree(t *testing.T, sp *config.Splits) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetReportCaller(true)

	err := initWorkTree(logger, sp)
	testlib.NoError(t, true, err)
	testlib.NotEqual(t, true, "", sp.WorkTree)
	info, err := os.Stat(sp.WorkTree)
	testlib.NoError(t, true, err)
	testlib.True(t, true, info.IsDir())
	defer func() { testlib.NoError(t, false, os.RemoveAll(sp.WorkTree)) }()

	for i := range sp.Splits {
		s := sp.Splits[i]
		testlib.NotEqual(t, true, "", s.WorkDir)
		info, err = os.Stat(sp.WorkTree)
		testlib.NoError(t, true, err)
		testlib.True(t, true, info.IsDir())
		defer func() { testlib.NoError(t, false, os.RemoveAll(s.WorkDir)) }()
	}
}
