package filecache

import (
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
	"github.com/sirupsen/logrus"

	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/filecache/uncache"
	"github.com/modularise/modularise/internal/testlib"
)

func initFileCache(t *testing.T, cacheType Type, a *txtar.Archive) (cache FileCache, cleanup func()) {
	switch cacheType {
	case TestCache:
		return populateTestcache(t, a)
	case Uncache:
		return populateUncache(t, a)
	default:
		t.Fatalf("Can not initialise filecache content for cache type %v.", cacheType)
	}
	return nil, func() { t.Fatal("No cleanup") }
}

func populateTestcache(t *testing.T, a *txtar.Archive) (*testcache.FakeFileCache, func()) {
	fe := map[string]testcache.FakeFileCacheEntry{}
	for _, f := range a.Files {
		fe[f.Name] = testcache.FakeFileCacheEntry{Data: f.Data}
	}

	mod, ok := fe["go.mod"]
	testlib.True(t, true, ok)

	mp := regexp.MustCompile("^module ([^\\s]+)\n").FindSubmatch(mod.Data)
	testlib.True(t, true, len(mp) == 2)

	c, err := testcache.NewFakeFileCache("fake-cache-dir", fe)
	testlib.NoError(t, true, err)
	return c, func() {}
}

func populateUncache(t *testing.T, a *txtar.Archive) (*uncache.Uncache, func()) {
	cd, err := ioutil.TempDir("", "modularise-uncache-test")
	testlib.NoError(t, true, err)

	complete := false
	defer func() {
		if !complete {
			testlib.NoError(t, false, os.RemoveAll(cd))
		}
	}()

	testlib.NoError(t, true, txtar.Write(a, cd))

	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	l.ReportCaller = true

	uc, err := uncache.NewUncache(l, cd)
	testlib.NoError(t, true, err)

	complete = true
	return uc, func() { testlib.NoError(t, false, os.RemoveAll(cd)) }
}
