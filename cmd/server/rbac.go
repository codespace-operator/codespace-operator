package main

import (
	"net/http"
	"slices"
)

// TODO: make RBAC configurable per roles
// Permissions we’ll use on API routes.
const (
	PermSessionsList   = "sessions.list"
	PermSessionsWatch  = "sessions.watch"
	PermSessionsGet    = "sessions.get"
	PermSessionsCreate = "sessions.create"
	PermSessionsUpdate = "sessions.update"
	PermSessionsDelete = "sessions.delete"
	PermSessionsScale  = "sessions.scale"
)

// Built-in roles. Admin = everything. Extend/move to config later.
var rolePermissions = map[string][]string{
	"codespace-operator:admin": {"*"},
	"codespace-operator:user": {
		PermSessionsList, PermSessionsWatch, PermSessionsGet, PermSessionsCreate, PermSessionsScale,
	},
}

// hasWildcard checks if a role grants all permissions.
func hasWildcard(perms []string) bool {
	return slices.Contains(perms, "*")
}

// hasPermission checks if any granted role allows a specific permission.
func hasPermission(cl *claims, required string) bool {
	if cl == nil {
		return false
	}
	for _, role := range cl.Roles {
		perms := rolePermissions[role]
		if len(perms) == 0 {
			continue
		}
		if hasWildcard(perms) || slices.Contains(perms, required) {
			return true
		}
	}
	return false
}

// mustHavePermission is a tiny guard you can call at the top of handlers.
func mustHavePermission(w http.ResponseWriter, r *http.Request, perm string) (*claims, bool) {
	cl := fromContext(r)
	if !hasPermission(cl, perm) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return cl, true
}
