package config

import (
	"io/ioutil"

	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type AuthConfig struct {
	PubKey    string       `yaml:"pub_key"`
	TokenFile string       `yaml:"token_file"`
	UserPass  UserPassword `yaml:"userpass"`
}

type UserPassword struct {
	Username     string `yaml:"username"`
	PasswordFile string `yaml:"password_file"`
}

func (a AuthConfig) ExtractAuth() (transport.AuthMethod, error) {
	switch {
	case a.TokenFile != "":
		return a.extractAuthAT()
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

func (a AuthConfig) extractAuthAT() (transport.AuthMethod, error) {
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
