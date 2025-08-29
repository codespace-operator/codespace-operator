package server

import (
	"encoding/json"
	"errors"
	"os"
	"sync"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type LocalUser struct {
	Username     string `json:"username" yaml:"username"`
	PasswordHash string `json:"passwordHash" yaml:"passwordHash"`
	Email        string `json:"email,omitempty" yaml:"email,omitempty"`
}

type LocalUsers struct {
	mu   sync.RWMutex
	byU  map[string]LocalUser
	path string
}

func loadLocalUsers(path string) (*LocalUsers, error) {
	if path == "" {
		return &LocalUsers{byU: map[string]LocalUser{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Users []LocalUser `json:"users" yaml:"users"`
	}
	if yaml.Unmarshal(raw, &wrap) != nil && json.Unmarshal(raw, &wrap) != nil {
		return nil, errors.New("local users: failed to parse yaml/json")
	}
	idx := make(map[string]LocalUser, len(wrap.Users))
	for _, u := range wrap.Users {
		idx[u.Username] = u
	}
	return &LocalUsers{byU: idx, path: path}, nil
}

func (l *LocalUsers) verify(username, password string) (*LocalUser, error) {
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
