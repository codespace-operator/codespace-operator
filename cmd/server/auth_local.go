package main

import (
	"encoding/json"
	"errors"
	"os"
	"sync"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type localUser struct {
	Username     string `json:"username" yaml:"username"`
	PasswordHash string `json:"passwordHash" yaml:"passwordHash"`
	Email        string `json:"email,omitempty" yaml:"email,omitempty"`
}

type localUsers struct {
	mu   sync.RWMutex
	byU  map[string]localUser
	path string
}

func loadLocalUsers(path string) (*localUsers, error) {
	if path == "" {
		return &localUsers{byU: map[string]localUser{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Users []localUser `json:"users" yaml:"users"`
	}
	if yaml.Unmarshal(raw, &wrap) != nil && json.Unmarshal(raw, &wrap) != nil {
		return nil, errors.New("local users: failed to parse yaml/json")
	}
	idx := make(map[string]localUser, len(wrap.Users))
	for _, u := range wrap.Users {
		idx[u.Username] = u
	}
	return &localUsers{byU: idx, path: path}, nil
}

func (l *localUsers) verify(username, password string) (*localUser, error) {
	l.mu.RLock()
	u, ok := l.byU[username]
	l.mu.RUnlock()
	if !ok || u.PasswordHash == "" {
		return nil, errors.New("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, errors.New("invalid credentials")
	}
	return &u, nil
}
