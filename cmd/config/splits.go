package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/modularise/modularise/internal/splits"
)

type Splits struct {
	// Authentication setup to clone / push Git repositories.
	Credentials AuthConfig `yaml:"credentials,omitempty"`
	// ID used for new commits generated in split repositories.
	Author AuthorData `yaml:"author,omitempty"`
	// Map of all configured splits.
	Splits map[string]*Split `yaml:"splits,omitempty"`

	// Internal state.
	splits.DataSplits `yaml:"-"`
}

type Split struct {
	// Module path for the split
	ModulePath string `yaml:"module_path,omitempty"`
	// List of paths relative to the source module's root. Any Go packages below these paths will be
	// made part of this split, unless:
	// - they are explicitly exluded by a longer prefix path in the Excludes list.
	// - they are explicitly included in another split.
	Includes []string `yaml:"includes,omitempty"`
	// List of paths relative to the source module's root. Any Go packages below these paths will
	// not be made part of this split, unless they are explicitly included by a longer prefix path
	// in the Includes list.
	Excludes []string `yaml:"excludes,omitempty"`
	// URL of the Git VCS where this split resides.
	URL string `yaml:"url,omitempty"`
	// Branch on the remote VCS that should be cloned from / pushed to for split content, defaults to 'master'.
	Branch string `yaml:"branch,omitempty"`

	// Internal state.
	splits.DataSplit `yaml:"-"`
}

type AuthConfig struct {
	PubKey      *string       `yaml:"pub_key,omitempty"`
	TokenEnvVar *string       `yaml:"token_envvar,omitempty"`
	UserPass    *UserPassword `yaml:"userpass,omitempty"`
}

type UserPassword struct {
	Username     string `yaml:"username,omitempty"`
	PasswordFile string `yaml:"password_file,omitempty"`
}

func (a AuthConfig) ExtractAuth() (transport.AuthMethod, error) {
	switch {
	case a.PubKey != nil:
		return a.extractAuthSSH()
	case a.TokenEnvVar != nil:
		return a.extractAuthToken()
	case a.UserPass != nil:
		return a.extractAuthUserPass()
	default:
		return nil, nil
	}
}

func (a AuthConfig) extractAuthSSH() (transport.AuthMethod, error) {
	sshKey, err := ioutil.ReadFile(*a.PubKey)
	if err != nil {
		return nil, err
	}
	publicKey, err := ssh.NewPublicKeys("git", sshKey, "")
	if err != nil {
		return nil, err
	}
	return publicKey, nil
}

func (a AuthConfig) extractAuthToken() (transport.AuthMethod, error) {
	token, ok := os.LookupEnv(*a.TokenEnvVar)
	if !ok {
		return nil, fmt.Errorf("authentication environment variable %q was not set", *a.TokenEnvVar)
	}
	return &http.TokenAuth{Token: token}, nil
}

func (a AuthConfig) extractAuthUserPass() (transport.AuthMethod, error) {
	if a.UserPass.Username == "" || a.UserPass.PasswordFile == "" {
		return nil, errors.New("no username and / or password source configured")
	}

	token, err := ioutil.ReadFile(a.UserPass.PasswordFile)
	if err != nil {
		return nil, err
	}

	return &http.BasicAuth{
		Username: a.UserPass.Username,
		Password: string(token),
	}, nil
}

type AuthorData struct {
	Name  string `yaml:"name,omitempty"`
	Email string `yaml:"email,omitempty"`
}

func (a AuthorData) ExtractAuthor() *object.Signature {
	n := a.Name
	if n == "" {
		n = "modularise"
	}
	e := a.Email
	if e == "" {
		n = "modularise@modularise.io"
	}
	return &object.Signature{Name: n, Email: e, When: time.Now()}
}
