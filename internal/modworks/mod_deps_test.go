package modworks

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
	"github.com/modularise/modularise/internal/testrepo"
)

func TestCommitChanges(t *testing.T) {
	t.Parallel()

	td, err := ioutil.TempDir("", "modularise-test-commit-changes")
	testlib.NoError(t, true, err)
	defer cleanupTestDir(t, td)

	fc, err := testcache.NewFakeFileCache("", map[string]testcache.FakeFileCacheEntry{
		"go.mod": {Data: []byte("module example.com/project\n")},
	})
	testlib.NoError(t, true, err)

	repo := testrepo.CreateTestRepo(t, []testrepo.RepoAction{
		testrepo.AddFile(testrepo.RepoFile{Path: "test.txt", Content: []byte("foo")}),
		testrepo.Commit("First commit"),
	})
	repo.WriteToDisk(filepath.Join(td, "repo"))
	h := repo.Head()

	r := &resolver{
		log:       testlib.NewTestLogger(),
		fc:        fc,
		sp:        &config.Splits{},
		sourceVer: "v0.0.0-sourcever",
	}
	s := &config.Split{DataSplit: splits.DataSplit{
		Name:    "split",
		WorkDir: td,
		Repo:    repo.Repository(),
	}}

	// Test that we do not create a new commit if the repo is clean.
	err = r.commitChanges(s)
	testlib.NoError(t, true, err)

	nh := repo.Head()
	testlib.Equal(t, true, h.Hash, nh.Hash)

	// Test that we create a new commit when the repo is dirty.
	repo.Apply([]testrepo.RepoAction{
		testrepo.AddFile(testrepo.RepoFile{Path: "new.txt", Content: []byte("foo")}),
	})

	err = r.commitChanges(s)
	testlib.NoError(t, true, err)

	nh = repo.Head()
	testlib.NotEqual(t, true, h.Hash, nh.Hash)
}

func TestLocalProxy(t *testing.T) {
	t.Parallel()

	td, err := ioutil.TempDir("", "modularise-local-proxy-test")
	testlib.NoError(t, true, err)
	defer cleanupTestDir(t, td)

	t.Logf("Test directory: %s", td)

	proxyPath := filepath.Join(td, "proxy-storage")
	testlib.NoError(t, true, os.MkdirAll(proxyPath, 0755))

	gopath := filepath.Join(td, "gopath")
	testlib.NoError(t, true, os.MkdirAll(gopath, 0755))

	depRepo := testrepo.CreateTestRepo(t, []testrepo.RepoAction{
		testrepo.AddFile(testrepo.RepoFile{Path: "main.go", Content: []byte("package main\n\nfunc main() {}\n")}),
		testrepo.AddFile(testrepo.RepoFile{Path: "go.mod", Content: []byte("module fake.com/dep\n\ngo 1.13\n")}),
		testrepo.Commit("First commit"),
	})
	depRepoPath := filepath.Join(td, "downstream")
	depRepo.WriteToDisk(depRepoPath)

	wp := filepath.Join(td, "upstream")
	testlib.NoError(t, true, os.MkdirAll(wp, 0755))
	testlib.NoError(t, true, ioutil.WriteFile(filepath.Join(wp, "go.mod"), []byte("module fake.com/mod\n\ngo 1.13\n"), 0644))

	split := config.Split{
		ModulePath: "fake.com/dep",
		DataSplit: splits.DataSplit{
			Name:    "test-split",
			WorkDir: depRepoPath,
			Repo:    depRepo.Repository(),
		},
	}

	err = (&resolver{
		log:        testlib.NewTestLogger(),
		localProxy: proxyPath,
	}).populateLocalProxy(&split)
	testlib.NoError(t, true, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "get", "-x", fmt.Sprintf("%s@%s", split.ModulePath, depRepo.Head().ID().String()))
	cmd.Dir = wp
	cmd.Env = append(
		os.Environ(),
		"GODEBUG=", // Don't pass any debug options to the lower-level invocation.
		fmt.Sprintf("GOPATH=%s", gopath),
		fmt.Sprintf("GOPROXY=file://%s", proxyPath),
		fmt.Sprintf("GOSUMDB=off"),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run 'go %v': %s", cmd.Args, out)
	}
}

// Cleaning up a testing directory might be complicated by the fact that the content of the module
// cache is read-only by design. As a result we need to first ensure that the entirety of the
// testing directory's content is writeable before deleting it.
func cleanupTestDir(t *testing.T, dir string) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, wErr error) error {
		if wErr != nil {
			return wErr
		}
		return os.Chmod(path, info.Mode()|0200)
	})
	testlib.NoError(t, false, err)
	testlib.NoError(t, false, os.RemoveAll(dir))
}
