package filecache

import (
	"errors"
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
	"github.com/sirupsen/logrus"

	"github.com/modularise/modularise/internal/filecache/cache"
	"github.com/modularise/modularise/internal/filecache/testcache"
	"github.com/modularise/modularise/internal/filecache/uncache"
	"github.com/modularise/modularise/internal/testlib"
)

func testFileCache(t *testing.T, cacheType Type, a *txtar.Archive) (cache FileCache, cleanup func()) {
	switch cacheType {
	case Cache:
		return testCache(t, a)
	case Uncache:
		return testUncache(t, a)
	case TestCache:
		return testTestCache(t, a)
	default:
		t.Fatalf("Can not initialise filecache content for cache type %v.", cacheType)
	}
	return nil, func() { t.Fatal("No cleanup") }
}

func testCache(t *testing.T, a *txtar.Archive) (*cache.Cache, func()) {
	c, err := populateCache(a)
	testlib.NoError(t, true, err)
	return c, func() { testlib.NoError(t, false, os.RemoveAll(c.Root())) }
}

func testUncache(t *testing.T, a *txtar.Archive) (*uncache.Uncache, func()) {
	c, err := populateUncache(a)
	testlib.NoError(t, true, err)
	return c, func() { testlib.NoError(t, false, os.RemoveAll(c.Root())) }
}

func testTestCache(t *testing.T, a *txtar.Archive) (*testcache.FakeFileCache, func()) {
	c, err := populateTestCache(a)
	testlib.NoError(t, true, err)
	return c, func() {}
}

func benchmarkFileCache(b *testing.B, cacheType Type, a *txtar.Archive) (cache FileCache, cleanup func()) {
	switch cacheType {
	case Cache:
		return benchmarkCache(b, a)
	case Uncache:
		return benchmarkUnache(b, a)
	case TestCache:
		return benchmarkTestCache(b, a)
	default:
		b.Fatalf("Can not initialise filecache content for cache type %v.", cacheType)
	}
	return nil, func() { b.Fatal("No cleanup") }
}

func benchmarkCache(b *testing.B, a *txtar.Archive) (*cache.Cache, func()) {
	c, err := populateCache(a)
	if err != nil {
		b.Fatalf("Failed to initialise benchmark cache: %v", err)
	}
	return c, func() {
		if err := os.RemoveAll(c.Root()); err != nil {
			b.Fatalf("Failed to clean up benchmark cache: %v", err)
		}
	}
}

func benchmarkUnache(b *testing.B, a *txtar.Archive) (*uncache.Uncache, func()) {
	c, err := populateUncache(a)
	if err != nil {
		b.Fatalf("Failed to initialise benchmark cache: %v", err)
	}
	return c, func() {
		if err := os.RemoveAll(c.Root()); err != nil {
			b.Fatalf("Failed to clean up benchmark cache: %v", err)
		}
	}
}

func benchmarkTestCache(b *testing.B, a *txtar.Archive) (*testcache.FakeFileCache, func()) {
	c, err := populateTestCache(a)
	if err != nil {
		b.Fatalf("Failed to initialise benchmark cache: %v", err)
	}
	return c, func() {}
}

func populateCache(a *txtar.Archive) (c *cache.Cache, err error) {
	cd, err := ioutil.TempDir("", "modularise-cache-test")
	if err != nil {
		return nil, err
	}

	complete := false
	defer func() {
		if !complete {
			err = os.RemoveAll(cd)
		}
	}()

	if err = txtar.Write(a, cd); err != nil {
		return nil, err
	}

	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	l.ReportCaller = true

	c, err = cache.NewCache(l, cd)
	if err != nil {
		return nil, err
	}

	complete = true
	return c, nil
}

func populateUncache(a *txtar.Archive) (c *uncache.Uncache, err error) {
	cd, err := ioutil.TempDir("", "modularise-uncache-test")
	if err != nil {
		return nil, err
	}

	complete := false
	defer func() {
		if !complete {
			err = os.RemoveAll(cd)
		}
	}()

	if err = txtar.Write(a, cd); err != nil {
		return nil, err
	}

	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	l.ReportCaller = true

	c, err = uncache.NewUncache(l, cd)
	if err != nil {
		return nil, err
	}

	complete = true
	return c, nil
}

func populateTestCache(a *txtar.Archive) (c *testcache.FakeFileCache, err error) {
	fe := map[string]testcache.FakeFileCacheEntry{}
	for _, f := range a.Files {
		fe[f.Name] = testcache.FakeFileCacheEntry{Data: f.Data}
	}

	mod, ok := fe["go.mod"]
	if !ok {
		return nil, errors.New("no go.mod present in cache content")
	}

	mp := regexp.MustCompile("^module ([^\\s]+)\n").FindSubmatch(mod.Data)
	if len(mp) != 2 {
		return nil, errors.New("go.mod does not contain a module statement")
	}

	c, err = testcache.NewFakeFileCache("fake-cache-dir", fe)
	if err != nil {
		return nil, err
	}
	return c, nil
}
