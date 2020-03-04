package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type AuthConfig struct {
	PubKey   *string       `yaml:"pub_key,omitempty"`
	UserPass *UserPassword `yaml:"userpass,omitempty"`
}

type UserPassword struct {
	Username       string `yaml:"username,omitempty"`
	PasswordFile   string `yaml:"password_file,omitempty"`
	PasswordEnvVar string `yaml:"password_envvar,omitempty"`
}

func (a AuthConfig) ExtractAuth() (transport.AuthMethod, error) {
	switch {
	case a.PubKey != nil:
		return a.extractAuthSSH()
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

func (a AuthConfig) extractAuthUserPass() (transport.AuthMethod, error) {
	c := a.UserPass
	if c.Username == "" || (c.PasswordEnvVar == "" && c.PasswordFile == "") {
		return nil, errors.New("no username and / or password source configured")
	}

	var token string
	if c.PasswordEnvVar != "" {
		var ok bool
		if token, ok = os.LookupEnv(c.PasswordEnvVar); !ok {
			return nil, fmt.Errorf("authentication environment variable %q was not set", c.PasswordEnvVar)
		}
	} else {
		b, err := ioutil.ReadFile(a.UserPass.PasswordFile)
		if err != nil {
			return nil, err
		}
		token = string(b)
	}

	return &http.BasicAuth{
		Username: a.UserPass.Username,
		Password: token,
	}, nil
}
