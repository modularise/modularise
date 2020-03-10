package residuals

import (
	"go/parser"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/filecache"
)

func ComputeResiduals(log *zap.Logger, fc filecache.FileCache, sp *config.Splits) error {
	for _, s := range sp.Splits {
		if err := computeSplitResiduals(log, fc, sp, s); err != nil {
			return err
		}
	}
	return nil
}

func computeSplitResiduals(log *zap.Logger, fc filecache.FileCache, sp *config.Splits, s *config.Split) error {
	log.Debug("Resolving split dependencies and residuals.", zap.String("split", s.Name))

	r := &resolver{
		log:   log,
		fc:    fc,
		sp:    sp,
		s:     s,
		limit: make(chan struct{}, runtime.NumCPU()),
	}

	var files []string
	for f := range s.Files {
		if filepath.Ext(f) == ".go" {
			files = append(files, f)
		}
	}
	r.enqueue(files...)

	r.wait()
	if r.err != nil {
		return r.err
	}

	return r.finalise()
}

type resolver struct {
	log *zap.Logger
	fc  filecache.FileCache
	sp  *config.Splits
	s   *config.Split

	err       error
	residuals sync.Map
	splitDeps sync.Map

	lock  sync.Mutex
	wg    sync.WaitGroup
	files []string
	limit chan struct{}
}

func (r *resolver) wait() {
	r.wg.Wait()
}

func (r *resolver) enqueue(files ...string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.files = append(r.files, files...)
	for i := 0; i < len(r.files); i++ {
		select {
		case r.limit <- struct{}{}:
			r.wg.Add(1)
			go r.analyse()
		default:
			return
		}
	}
}

func (r *resolver) dequeue() (string, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if len(r.files) == 0 || r.err != nil {
		return "", false
	}
	file := r.files[0]
	r.files = r.files[1:]
	return file, true
}

func (r *resolver) analyse() {
	defer r.wg.Done()

	for {
		f, ok := r.dequeue()
		if !ok {
			return
		}

		r.log.Debug("Parsing residual file for indirect dependencies.", zap.String("file", f))

		fa, _, err := r.fc.ReadGoFile(f, parser.ImportsOnly)
		if err != nil {
			r.err = err
			return
		}

		for _, imp := range fa.Imports {
			p := strings.Trim(imp.Path.Value, "\"")
			if _, ok := r.residuals.Load(p); ok {
				continue
			} else if !r.fc.Pkgs()[p] {
				continue
			}

			if ts := r.sp.PkgToSplit[p]; ts == r.s.Name {
				continue
			} else if ts != "" {
				r.log.Debug("Inter-split dependency detected.", zap.String("import", p), zap.String("source", r.s.Name), zap.String("target", ts))
				r.splitDeps.Store(ts, true)
				continue
			}

			r.log.Debug("Residual detected.", zap.String("split", r.s.Name), zap.String("residual", p))
			r.residuals.Store(p, true)

			pkgFiles, err := r.fc.FilesInPkg(p)
			if err != nil {
				r.err = err
				return
			}

			var files []string
			for f := range pkgFiles {
				if filepath.Ext(f) == ".go" {
					files = append(files, f)
				}
			}
			r.enqueue(files...)
		}
	}
}

func (r *resolver) finalise() error {
	r.s.SplitDeps = map[string]bool{}
	r.splitDeps.Range(func(key interface{}, _ interface{}) bool {
		r.s.SplitDeps[key.(string)] = true
		return true
	})

	r.s.Residuals = map[string]bool{}
	r.s.ResidualFiles = map[string]bool{}

	var err error
	r.residuals.Range(func(key interface{}, _ interface{}) bool {
		residual := key.(string)
		r.s.Residuals[residual] = true

		var fs map[string]bool
		fs, err = r.fc.FilesInPkg(residual)
		if err != nil {
			return false
		}
		for f := range fs {
			r.s.ResidualFiles[f] = true
		}
		return true
	})
	return err
}
