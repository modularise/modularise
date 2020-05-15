package testrepo

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/modularise/modularise/internal/testlib"
)

type TestRepo struct {
	t    *testing.T
	r    *git.Repository
	path string
}

func CreateTestRepo(t *testing.T, actions []RepoAction) *TestRepo {
	r, err := git.Init(memory.NewStorage(), memfs.New())
	testlib.NoError(t, true, err)

	repo := &TestRepo{
		t: t,
		r: r,
	}

	repo.Apply(actions)
	return repo
}

func (r *TestRepo) Apply(actions []RepoAction) {
	for i := range actions {
		actions[i](r)
	}
}

func (r *TestRepo) WriteToDisk(path string) {
	testlib.Equal(r.t, true, "", r.path)

	_, err := os.Stat(path)
	testlib.True(r.t, true, os.IsNotExist(err))

	err = os.MkdirAll(path, 0755)
	testlib.NoError(r.t, true, err)

	mt, err := r.r.Worktree()
	testlib.NoError(r.t, true, err)

	dt := osfs.New(path)
	copyBillyFilesystem(r.t, mt.Filesystem, dt)

	ds := filesystem.NewStorage(osfs.New(filepath.Join(path, ".git")), cache.NewObjectLRUDefault())
	copyGitStorage(r.t, r.r.Storer, ds)

	nr, err := git.Open(ds, dt)
	testlib.NoError(r.t, true, err)

	r.r = nr
	r.path = path
}

func (r *TestRepo) Path() string {
	testlib.NotEqual(r.t, true, r.Path, "")
	return r.path
}

func (r *TestRepo) Repository() *git.Repository {
	return r.r
}

func (r *TestRepo) Head() *object.Commit {
	h, err := r.r.Head()
	testlib.NoError(r.t, true, err)

	c, err := r.r.CommitObject(h.Hash())
	testlib.NoError(r.t, true, err)

	return c
}

func copyBillyFilesystem(t *testing.T, src billy.Filesystem, dst billy.Filesystem) {
	todo := []string{"."}
	for {
		if len(todo) == 0 {
			return
		}
		curr := todo[len(todo)-1]
		todo = todo[:len(todo)-1]

		infos, rErr := src.ReadDir(curr)
		testlib.NoError(t, true, rErr)

		for _, info := range infos {
			p := filepath.Join(curr, info.Name())
			if info.IsDir() {
				todo = append(todo, p)
				continue
			}
			sf, err := src.OpenFile(p, os.O_RDONLY, info.Mode())
			testlib.NoError(t, true, err)

			tf, err := dst.OpenFile(p, os.O_CREATE|os.O_WRONLY, info.Mode())
			testlib.NoError(t, true, err)

			_, err = io.Copy(tf, sf)
			testlib.NoError(t, true, err)
		}
	}
}

func copyGitStorage(t *testing.T, src storage.Storer, dst storage.Storer) {
	c, err := src.Config()
	testlib.NoError(t, true, err)

	err = dst.SetConfig(c)
	testlib.NoError(t, true, err)

	idx, err := src.Index()
	testlib.NoError(t, true, err)

	err = dst.SetIndex(idx)
	testlib.NoError(t, true, err)

	oIter, err := src.IterEncodedObjects(plumbing.AnyObject)
	testlib.NoError(t, true, err)

	err = oIter.ForEach(func(obj plumbing.EncodedObject) error {
		_, err = dst.SetEncodedObject(obj)
		return err
	})
	testlib.NoError(t, true, err)

	rIter, err := src.IterReferences()
	testlib.NoError(t, true, err)

	err = rIter.ForEach(dst.SetReference)
	testlib.NoError(t, true, err)
}
