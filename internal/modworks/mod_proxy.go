package modworks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"unicode"

	"go.uber.org/zap"
	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"

	"github.com/modularise/modularise/cmd/config"
	"github.com/modularise/modularise/internal/modworks/pseudo"
)

func (r *resolver) populateLocalProxy(s *config.Split) error {
	info, err := pseudo.Version(r.log, s)
	if err != nil {
		return err
	}
	s.Version = info.Version

	var moduleCachePath string
	for _, r := range s.ModulePath {
		if unicode.IsUpper(r) {
			moduleCachePath += "!"
		}
		moduleCachePath += string(unicode.ToLower(r))
	}
	proxyPath := filepath.Join(r.localProxy, filepath.FromSlash(moduleCachePath), "@v")
	if err = os.MkdirAll(proxyPath, 0755); err != nil {
		r.log.Error("Failed to create local proxy storage directory.", zap.String("directory", proxyPath), zap.Error(err))
		return err
	}

	// Info file for hash redirection.
	ji, err := json.Marshal(&info)
	if err != nil {
		r.log.Error("Failed to marshal .info file.", zap.String("split", s.Name), zap.Any("content", info), zap.Error(err))
		return err
	}
	p := filepath.Join(proxyPath, fmt.Sprintf("%s.info", info.Hash))
	if err = ioutil.WriteFile(p, ji, 0644); err != nil {
		r.log.Error("Failed to write .info file.", zap.String("path", p), zap.Error(err))
		return err
	}

	// Mod files and zip archives.
	mp := filepath.Join(s.WorkDir, "go.mod")
	mc, err := ioutil.ReadFile(mp)
	if err != nil {
		r.log.Error("Failed to read file.", zap.String("file", mp), zap.Error(err))
		return err
	}
	p = filepath.Join(proxyPath, fmt.Sprintf("%s.mod", info.Version))
	if err = ioutil.WriteFile(p, mc, 0644); err != nil {
		r.log.Error(
			"Failed to write mod file to temporary module proxy.",
			zap.String("module", s.ModulePath),
			zap.String("version", info.Version),
			zap.Error(err),
		)
		return err
	}

	p = filepath.Join(proxyPath, fmt.Sprintf("%s.zip", info.Version))
	var zf *os.File
	if zf, err = os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644); err != nil {
		r.log.Error("Failed to open a new file.", zap.String("file", p))
		return err
	}
	defer func() { _ = zf.Close() }()

	if err = zip.CreateFromDir(zf, module.Version{Path: s.ModulePath, Version: info.Version}, s.WorkDir); err != nil {
		r.log.Error(
			"Failed to zip content of split into cache archive.",
			zap.String("directory", s.WorkDir),
			zap.String("archive", p),
			zap.Error(err),
		)
		return err
	}
	return nil
}
