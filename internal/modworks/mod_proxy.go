package modworks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"unicode"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"

	"github.com/Helcaraxan/modularise/cmd/config"
	"github.com/Helcaraxan/modularise/internal/modworks/pseudo"
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
		r.log.WithError(err).Errorf("Failed to create local proxy storage directory at %q.", proxyPath)
		return err
	}

	// Info file for hash redirection.
	ji, err := json.Marshal(&info)
	if err != nil {
		r.log.WithError(err).Errorf("Failed to marshal the .info file '%+v' for split located at %q.", info, s.WorkDir)
		return err
	}
	p := filepath.Join(proxyPath, fmt.Sprintf("%s.info", info.Hash))
	if err = ioutil.WriteFile(p, ji, 0644); err != nil {
		r.log.WithError(err).Errorf("Failed to write the .info file for split located at %q to %q.", s.Name, p)
		return err
	}

	// Mod files and zip archives.
	mp := filepath.Join(s.WorkDir, "go.mod")
	mc, err := ioutil.ReadFile(mp)
	if err != nil {
		r.log.WithError(err).Errorf("Failed to read %q.", mp)
		return err
	}
	p = filepath.Join(proxyPath, fmt.Sprintf("%s.mod", info.Version))
	if err = ioutil.WriteFile(p, mc, 0644); err != nil {
		r.log.WithError(err).Errorf("Failed to write mod file to temporary module proxy for \"%s@%s\" of split %q.", s.ModulePath, info.Version, s.Name)
		return err
	}

	p = filepath.Join(proxyPath, fmt.Sprintf("%s.zip", info.Version))
	var zf *os.File
	if zf, err = os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644); err != nil {
		r.log.WithError(err).Errorf("Failed to open a new file at %q.", p)
		return err
	}
	defer func() { _ = zf.Close() }()

	if err = zip.CreateFromDir(zf, module.Version{Path: s.ModulePath, Version: info.Version}, s.WorkDir); err != nil {
		r.log.WithError(err).Errorf("Failed to zip content of split %q at %q into archive at %q.", s.Name, s.WorkDir, p)
		return err
	}
	return nil
}
