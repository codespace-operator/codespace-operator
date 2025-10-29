package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	auth "github.com/codespace-operator/common/auth/pkg/auth"
)

// Wire up in server.go via registerProfileHandlers(mux, h).

// ---------- public API shapes ----------

type UserProfile struct {
	// flat KV e.g. git.name, git.email, ide defaults...
	Data map[string]string `json:"data"`
}

// ---------- storage interfaces ----------

type profileStore interface {
	Get(ctx context.Context, id string) (*UserProfile, error)
	Put(ctx context.Context, id string, u *UserProfile) error
}

// memory store (useful for dev)
type memProfileStore struct {
	mu   sync.RWMutex
	data map[string]*UserProfile
}

func newMemProfileStore() *memProfileStore {
	return &memProfileStore{data: map[string]*UserProfile{}}
}
func (m *memProfileStore) Get(_ context.Context, id string) (*UserProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if u, ok := m.data[id]; ok {
		// clone to avoid accidental sharing
		out := &UserProfile{Data: map[string]string{}}
		for k, v := range u.Data {
			out.Data[k] = v
		}
		return out, nil
	}
	return &UserProfile{Data: map[string]string{}}, nil
}
func (m *memProfileStore) Put(_ context.Context, id string, u *UserProfile) error {
	if u == nil {
		return errors.New("nil profile")
	}
	if u.Data == nil {
		u.Data = map[string]string{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// shallow copy
	cp := &UserProfile{Data: map[string]string{}}
	for k, v := range u.Data {
		cp.Data[k] = v
	}
	m.data[id] = cp
	return nil
}

// postgres-backed store
type pgProfileStore struct{ dsn string }

func newPgProfileStore(dsn string) *pgProfileStore { return &pgProfileStore{dsn: dsn} }

func (s *pgProfileStore) Get(ctx context.Context, id string) (*UserProfile, error) {
	type row struct {
		Data []byte
	}
	conn, err := openPg(ctx, s.dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	var raw []byte
	if err := conn.QueryRow(ctx, `select data from user_profiles where id=$1`, id).Scan(&raw); err != nil {
		// default-empty if not found
		return &UserProfile{Data: map[string]string{}}, nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		m = map[string]string{}
	}
	return &UserProfile{Data: m}, nil
}

func (s *pgProfileStore) Put(ctx context.Context, id string, u *UserProfile) error {
	if u == nil {
		return errors.New("nil profile")
	}
	if u.Data == nil {
		u.Data = map[string]string{}
	}
	conn, err := openPg(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	js, _ := json.Marshal(u.Data)
	err = conn.Exec(ctx, `
		insert into user_profiles(id, data, updated_at)
		values ($1, $2::jsonb, now())
		on conflict (id) do update set data=excluded.data, updated_at=now()
	`, id, string(js))
	return err
}

type pgConn interface {
   Close(context.Context) error
   Exec(context.Context, string, ...any) error
   QueryRow(context.Context, string, ...any) pgRow
}
type pgRow interface{ Scan(...any) error }

var openPg = func(ctx context.Context, dsn string) (pgConn, error) {
	// Late-bind to pgx to keep this file focused; see server.go where we set openPg.
	return nil, errors.New("postgres not wired")
}

// ---------- handlers ----------

func registerProfileHandlers(mux *http.ServeMux, h *handlers) {
	// Build store from server config
	var store profileStore
	switch strings.ToLower(strings.TrimSpace(h.deps.config.ProfileStoreKind)) {
	case "postgres", "pg", "postgresql":
		store = newPgProfileStore(h.deps.config.ProfileStoreDSN)
	default:
		store = newMemProfileStore()
	}
	h.profile = store

	base := "/user-profiles/"
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		// path must be /user-profiles/{id}
		if r.URL.Path == "/user-profiles" || r.URL.Path == "/user-profiles/" {
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, base)
		switch r.Method {
		case http.MethodGet:
			if !h.allowProfileRead(r, id) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			u, err := h.profile.Get(r.Context(), id)
			if err != nil {
				errJSON(w, fmt.Errorf("failed to get profile: %w", err))
				return
			}
			writeJSON(w, u)

		case http.MethodPut, http.MethodPost, http.MethodPatch:
			if !h.allowProfileWrite(r, id) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			body, _ := io.ReadAll(r.Body)
			defer r.Body.Close()
			var u UserProfile
			if err := json.Unmarshal(body, &u); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if u.Data == nil {
				u.Data = map[string]string{}
			}
			if err := h.profile.Put(r.Context(), id, &u); err != nil {
				errJSON(w, fmt.Errorf("failed to save profile: %w", err))
				return
			}
			writeJSON(w, map[string]any{"status": "ok", "updatedAt": time.Now().UTC().Format(time.RFC3339)})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

// authZ helpers:
// - Bearer <PROFILE_STORE_TOKEN>  => full access (controller path)
// - cookie session                => same-user read/write, or role=admin for any id

func (h *handlers) hasSharedToken(r *http.Request) bool {
	want := strings.TrimSpace(h.deps.config.ProfileStoreToken)
	if want == "" {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return got != "" && got == want
}

func contains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}

// internal/server/profile_handlers.go
func (h *handlers) allowProfileRead(r *http.Request, id string) bool {
    if h.hasSharedToken(r) { return true }
    cl := auth.FromContext(r)
    if cl == nil {
        if claims, err := h.deps.authManager.ValidateRequest(r); err == nil {
            cl = claims
        } else {
            return false
        }
    }
    return cl.Sub == id || cl.Username == id || contains(cl.Roles, "admin")
}

func (h *handlers) allowProfileWrite(r *http.Request, id string) bool {
    if h.hasSharedToken(r) { return true }
    cl := auth.FromContext(r)
    if cl == nil {
        if claims, err := h.deps.authManager.ValidateRequest(r); err == nil {
            cl = claims
        } else {
            return false
        }
    }
    return cl.Sub == id || cl.Username == id || contains(cl.Roles, "admin")
}
