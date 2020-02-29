package integrationtests

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v3"

	"github.com/modularise/modularise/internal/filecache/uncache"
	"github.com/modularise/modularise/internal/parser"
	"github.com/modularise/modularise/internal/splits"
	"github.com/modularise/modularise/internal/testlib"
)

type testSpec struct {
	t *testing.T

	// Test configuration.
	name     string
	scenario string

	expected resultSpec

	// Internal details.
	logger  *logrus.Logger
	files   *txtar.Archive
	workDir string
}

type resultSpec struct {
	ExpectedSplits map[string]map[string]bool `yaml:"expected_splits"`
}

func TestIntegration_Parse(t *testing.T) {
	t.Parallel()

	tcs, err := filepath.Glob("_test_scenarios/*.txtar")
	testlib.NoError(t, true, err)
	t.Logf("Found the following test scenarios: %v", tcs)

	for i := range tcs {
		s := tcs[i]
		n := strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			l := logrus.New()
			l.SetLevel(logrus.DebugLevel)
			ts := &testSpec{
				t:        t,
				name:     n,
				logger:   l,
				scenario: s,
			}
			ts.workDir, err = ioutil.TempDir("", "modularise")
			testlib.NoError(t, true, err)
			defer func() {
				testlib.NoError(t, false, os.RemoveAll(ts.workDir))
			}()

			ts.setup()
			ts.run()
		})
	}
}

func (s *testSpec) setup() {
	ts, err := ioutil.ReadFile(s.scenario)
	testlib.NoError(s.t, true, err)

	s.t.Logf("Setting up files for test %q.", s.name)
	s.files = txtar.Parse(ts)
	for _, f := range s.files.Files {
		if f.Name == "expected" {
			err = yaml.Unmarshal(f.Data, &s.expected)
			testlib.NoError(s.t, true, err)
		} else {
			p := filepath.Join(s.workDir, f.Name)
			err = os.MkdirAll(filepath.Dir(p), 0755)
			testlib.NoError(s.t, true, err)
			err = ioutil.WriteFile(filepath.Join(s.workDir, f.Name), f.Data, 0644)
			testlib.NoError(s.t, true, err)
		}
	}
}

func (s *testSpec) run() {
	err := os.Chdir(s.workDir)
	testlib.NoError(s.t, true, err)

	r, err := ioutil.ReadFile(".modularise.yaml")
	testlib.NoError(s.t, true, err)

	var sp splits.Splits
	err = yaml.Unmarshal(r, &sp)
	testlib.NoError(s.t, true, err)
	testlib.NotNil(s.t, true, sp.Splits)

	fc, err := uncache.NewUncache(s.logger, s.workDir)
	testlib.NoError(s.t, true, err)

	err = parser.Parse(s.logger, fc, &sp)
	testlib.NoError(s.t, true, err)

	for k, v := range s.expected.ExpectedSplits {
		testlib.Equal(s.t, false, v, sp.Splits[k].Files)
	}
}
