package config

import (
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type AuthConfig struct {
	PubKey      string       `yaml:"pub_key,omitempty"`
	TokenFile   string       `yaml:"token_file,omitempty"`
	TokenEnvVar string       `yaml:"token,omitempty"`
	UserPass    UserPassword `yaml:"userpass,omitempty"`
}

type UserPassword struct {
	Username     string `yaml:"username,omitempty"`
	PasswordFile string `yaml:"password_file,omitempty"`
}

func (a AuthConfig) ExtractAuth() (transport.AuthMethod, error) {
	switch {
	case a.TokenEnvVar != "":
		return a.extractAuthEnvAT()
	case a.TokenFile != "":
		return a.extractAuthFileAT()
	case a.PubKey != "":
		return a.extractAuthSSH()
	case a.UserPass.PasswordFile != "":
		return a.extractAuthUserPass()
	default:
		return nil, nil
	}
}

func (a AuthConfig) extractAuthSSH() (transport.AuthMethod, error) {
	sshKey, err := ioutil.ReadFile(a.PubKey)
	if err != nil {
		return nil, err
	}
	publicKey, err := ssh.NewPublicKeys("git", sshKey, "")
	if err != nil {
		return nil, err
	}
	return publicKey, nil
}

func (a AuthConfig) extractAuthEnvAT() (transport.AuthMethod, error) {
	token, ok := os.LookupEnv(a.TokenEnvVar)
	if !ok {
		return nil, fmt.Errorf("environment variable %q was not set with a github authentication token", a.TokenEnvVar)
	}
	return &http.TokenAuth{Token: token}, nil
}

func (a AuthConfig) extractAuthFileAT() (transport.AuthMethod, error) {
	token, err := ioutil.ReadFile(a.TokenFile)
	if err != nil {
		return nil, err
	}
	return &http.TokenAuth{Token: string(token)}, nil
}

func (a AuthConfig) extractAuthUserPass() (transport.AuthMethod, error) {
	pwd, err := ioutil.ReadFile(a.UserPass.PasswordFile)
	if err != nil {
		return nil, err
	}
	return &http.BasicAuth{
		Username: a.UserPass.Username,
		Password: string(pwd),
	}, nil
}
