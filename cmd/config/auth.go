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
