package main

import (
	"net/http"
)

// Check user perms for various actions
func handleRBACIntrospect(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl := fromContext(r)
		if cl == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Namespaces to check; default to a useful pair.
		namespaces := splitCSVQuery(r.URL.Query().Get("namespaces"))
		if len(namespaces) == 0 {
			namespaces = []string{"default", "*"}
		}

		// Actions we care about for the UI.
		sessionActions := []string{"get", "list", "watch", "create", "update", "delete", "scale"}

		// Cluster-level permission: listing namespaces (explicitly required)
		nsListAllowed, _ := deps.rbac.Enforce(cl.Sub, cl.Roles, "namespaces", "list", "*")

		resp := map[string]any{
			"subject": cl.Sub,
			"roles":   cl.Roles,
			"cluster": map[string]any{
				"namespaces": map[string]bool{"list": nsListAllowed},
			},
			"domains": map[string]any{},
		}

		doms := map[string]any{}
		for _, ns := range namespaces {
			perms := map[string]bool{}
			for _, act := range sessionActions {
				ok, _ := deps.rbac.Enforce(cl.Sub, cl.Roles, "session", act, ns)
				perms[act] = ok
			}
			doms[ns] = map[string]any{"session": perms}
		}
		resp["domains"] = doms
		writeJSON(w, resp)
	}
}
