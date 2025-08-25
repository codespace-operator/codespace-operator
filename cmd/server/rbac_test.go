// cmd/server/rbac_test.go
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

const testModel = `
[request_definition]
r = sub, obj, act, dom

[policy_definition]
p = sub, obj, act, dom, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (g(r.sub, p.sub) || r.sub == p.sub) &&
    (r.obj == p.obj) &&
    (r.act == p.act || p.act == "*") &&
    (p.dom == "*" || r.dom == p.dom)
`

const testPolicy = `
p, admin,  *,           *,    *,  allow
p, admin,  namespaces,  list, *,  allow
p, editor, session,     get,  *,  allow
p, editor, session,     list, *,  allow
p, editor, session,     watch,*,  allow
p, editor, session,     create,*, allow
p, editor, session,     update,*, allow
p, editor, session,     delete,*, allow
p, editor, session,     scale, *, allow
p, viewer, session,     get,  *,  allow
p, viewer, session,     list, *,  allow
p, viewer, session,     watch,*,  allow
`

func newRBACForTest(t *testing.T, dir string) *RBAC {
	t.Helper()
	m := filepath.Join(dir, "model.conf")
	p := filepath.Join(dir, "policy.csv")
	write(t, m, testModel)
	write(t, p, testPolicy)
	t.Setenv(envModelPath, m)
	t.Setenv(envPolicyPath, p)
	r, err := NewRBACFromEnv(context.Background(), log.Default())
	if err != nil {
		t.Fatalf("NewRBACFromEnv: %v", err)
	}
	return r
}

func TestPolicyBasics(t *testing.T) {
	dir := t.TempDir()
	r := newRBACForTest(t, dir)

	// Admin: anything anywhere
	ok, _ := r.Enforce("admin", nil, "session", "delete", "team-a")
	if !ok {
		t.Fatal("admin should be allowed")
	}

	// Viewer: read-only
	ok, _ = r.Enforce("viewer", nil, "session", "get", "team-a")
	if !ok {
		t.Fatal("viewer get should be allowed")
	}
	ok, _ = r.Enforce("viewer", nil, "session", "create", "team-a")
	if ok {
		t.Fatal("viewer create should be denied")
	}

	// Role carried in JWT roles array
	ok, _ = r.Enforce("alice", []string{"viewer"}, "session", "list", "team-b")
	if !ok {
		t.Fatal("alice with viewer role should be allowed to list")
	}
}

func TestClusterNamespacesListRequiresClusterRole(t *testing.T) {
	dir := t.TempDir()
	r := newRBACForTest(t, dir)

	// viewer cannot list namespaces (cluster-level)
	ok, _ := r.Enforce("viewer", nil, "namespaces", "list", "*")
	if ok {
		t.Fatal("viewer should not be allowed to list namespaces")
	}

	// admin can
	ok, _ = r.Enforce("admin", nil, "namespaces", "list", "*")
	if !ok {
		t.Fatal("admin should be allowed to list namespaces")
	}
}

func TestHotReload(t *testing.T) {
	dir := t.TempDir()
	r := newRBACForTest(t, dir)

	// Initially denied
	ok, _ := r.Enforce("viewer", nil, "session", "create", "ns1")
	if ok {
		t.Fatal("viewer create should be denied before update")
	}

	// Update policy to allow viewer create
	p := filepath.Join(dir, "policy.csv")
	appendLine := "p, viewer, session, create, *, allow\n"
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open policy: %v", err)
	}
	if _, err := f.WriteString(appendLine); err != nil {
		t.Fatalf("append policy: %v", err)
	}
	_ = f.Close()

	// Give the watcher a moment to detect & reload
	time.Sleep(500 * time.Millisecond)

	ok, _ = r.Enforce("viewer", nil, "session", "create", "ns1")
	if !ok {
		t.Fatal("viewer create should be allowed after policy update")
	}
}
