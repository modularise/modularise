package testrepo

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/modularise/modularise/internal/testlib"
)

type RepoAction func(*TestRepo)

type RepoFile struct {
	Path    string
	Content []byte
	Mode    os.FileMode
}

const (
	TestAuthor = "modularise-tester"
	TestEmail  = "test@modularise.com"
)

func AddFile(file RepoFile) RepoAction {
	return func(r *TestRepo) {
		tree, err := r.r.Worktree()
		testlib.NoError(r.t, true, err)

		m := file.Mode
		if m == 0 {
			m = 0644
		}

		fd, err := tree.Filesystem.OpenFile(file.Path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, m)
		testlib.NoError(r.t, true, err)

		_, err = fd.Write(file.Content)
		testlib.NoError(r.t, true, err)

		_, err = tree.Add(file.Path)
		testlib.NoError(r.t, true, err)
	}
}

func Commit(message string) RepoAction {
	return func(r *TestRepo) {
		tree, err := r.r.Worktree()
		testlib.NoError(r.t, true, err)

		_, err = tree.Commit(message, &git.CommitOptions{
			Author: &object.Signature{
				Name:  TestAuthor,
				Email: TestEmail,
			},
		})
		testlib.NoError(r.t, true, err)
	}
}

func LightTag(name string) RepoAction {
	return func(r *TestRepo) {
		h, err := r.r.Head()
		testlib.NoError(r.t, true, err)

		err = r.r.Storer.SetReference(plumbing.NewHashReference(plumbing.NewTagReferenceName(name), h.Hash()))
		testlib.NoError(r.t, true, err)
	}
}

func AnnotatedTag(name string) RepoAction {
	return func(r *TestRepo) {
		h, err := r.r.Head()
		testlib.NoError(r.t, true, err)

		_, err = r.r.CreateTag(name, h.Hash(), &git.CreateTagOptions{
			Tagger: &object.Signature{
				Name:  TestAuthor,
				Email: TestEmail,
			},
			Message: fmt.Sprintf("Tag for %s", name),
		})
		testlib.NoError(r.t, true, err)
	}
}
