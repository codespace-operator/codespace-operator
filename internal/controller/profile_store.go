package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

type UserProfile struct {
  // flat KV; we care about git.name, git.email, extensions, ide defaults, etc.
  Data map[string]string `json:"data"`
}

type ProfileStore interface {
  GetProfile(ctx context.Context, userLabelID string) (*UserProfile, error)
}

type PostgresStore struct{ dsn string }

func NewPostgresStore(dsn string) *PostgresStore { return &PostgresStore{dsn} }

func (s *PostgresStore) GetProfile(ctx context.Context, id string) (*UserProfile, error) {
  if s.dsn == "" { return &UserProfile{Data: map[string]string{}}, nil }
  conn, err := pgx.Connect(ctx, s.dsn)
  if err != nil { return nil, err }
  defer conn.Close(ctx)
  var raw []byte
  // table schema: user_profiles(id text primary key, data jsonb)
  if err := conn.QueryRow(ctx, `select data from user_profiles where id=$1`, id).Scan(&raw); err != nil {
    return &UserProfile{Data: map[string]string{}}, nil // default-empty
  }
  u := &UserProfile{Data: map[string]string{}}
  _ = json.Unmarshal(raw, &u.Data)
  return u, nil
}

type HTTPStore struct {
  base  string
  token string
}

func NewHTTPStore(base, token string) *HTTPStore { return &HTTPStore{base: strings.TrimRight(base, "/"), token: token} }

func (s *HTTPStore) GetProfile(ctx context.Context, id string) (*UserProfile, error) {
  if s.base == "" || id == "" { return &UserProfile{Data: map[string]string{}}, nil }
  req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/user-profiles/%s", s.base, id), nil)
  if s.token != "" {
    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.token))
  }
  resp, err := http.DefaultClient.Do(req)
  if err != nil || resp.StatusCode >= 400 { return &UserProfile{Data: map[string]string{}}, nil }
  defer resp.Body.Close()
  var u UserProfile
  if err := json.NewDecoder(resp.Body).Decode(&u); err != nil { return &UserProfile{Data: map[string]string{}}, nil }
  if u.Data == nil { u.Data = map[string]string{} }
  return &u, nil
}
