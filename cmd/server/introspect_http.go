package main

import (
	"net/http"
	"sort"
	"strings"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func handleIntrospect(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl := fromContext(r)
		if cl == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse inputs
		namespaces := splitCSVQuery(r.URL.Query().Get("namespaces"))
		actions := splitCSVQuery(r.URL.Query().Get("actions"))
		if len(actions) == 0 {
			actions = []string{"get", "list", "watch", "create", "update", "delete", "scale"}
		}
		extraSubjects := splitCSVQuery(r.URL.Query().Get("roles")) // roles or arbitrary subjects
		discover := r.URL.Query().Get("discover") == "1"

		// Cluster-level (Casbin) permission: list namespaces
		nsListAllowed, _ := deps.rbac.Enforce(cl.Sub, cl.Roles, "namespaces", "list", "*")

		// Server SA capabilities (K8s RBAC via SSAR)
		sa := map[string]any{}
		nsCaps := map[string]bool{
			"list":  k8sCan(r.Context(), deps.typed, authv1.ResourceAttributes{Group: "", Resource: "namespaces", Verb: "list"}),
			"watch": k8sCan(r.Context(), deps.typed, authv1.ResourceAttributes{Group: "", Resource: "namespaces", Verb: "watch"}),
		}
		sa["namespaces"] = nsCaps

		sessCaps := map[string]bool{}
		for _, v := range []string{"get", "list", "watch", "create", "update", "delete", "patch"} {
			ok := k8sCan(r.Context(), deps.typed, authv1.ResourceAttributes{
				Group:    gvr.Group,
				Resource: gvr.Resource,
				Verb:     v,
			})
			sessCaps[v] = ok
		}
		sa["session"] = sessCaps

		// Discover namespaces (if asked + allowed)
		var allNamespaces []string
		var sessionNamespaces []string
		if discover && nsListAllowed && nsCaps["list"] {
			var nsl corev1.NamespaceList
			if err := deps.typed.List(r.Context(), &nsl); err == nil {
				for _, n := range nsl.Items {
					allNamespaces = append(allNamespaces, n.Name)
				}
				sort.Strings(allNamespaces)
			}
			ul, err := deps.dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{})
			if err == nil {
				set := map[string]struct{}{}
				for _, u := range ul.Items {
					set[u.GetNamespace()] = struct{}{}
				}
				for ns := range set {
					sessionNamespaces = append(sessionNamespaces, ns)
				}
				sort.Strings(sessionNamespaces)
			}
		}

		// If caller didn't pass namespaces, use a sensible default
		if len(namespaces) == 0 {
			if len(allNamespaces) > 0 {
				namespaces = allNamespaces
			} else {
				namespaces = []string{"default", "*"}
			}
		}

		// Effective permissions for current user
		domains := map[string]any{}
		for _, ns := range namespaces {
			perms := map[string]bool{}
			for _, act := range actions {
				ok, _ := deps.rbac.Enforce(cl.Sub, cl.Roles, "session", act, ns)
				perms[act] = ok
			}
			domains[ns] = map[string]any{"session": perms}
		}

		// Convenience: namespaces where user may list sessions
		var userAllowed []string
		for _, ns := range namespaces {
			ok, _ := deps.rbac.Enforce(cl.Sub, cl.Roles, "session", "list", ns)
			if ok {
				userAllowed = append(userAllowed, ns)
			}
		}
		sort.Strings(userAllowed)

		// Optional: introspect extra subjects (roles/groups)
		subjects := map[string]any{}
		if len(extraSubjects) > 0 {
			for _, sub := range extraSubjects {
				sub = strings.TrimSpace(sub)
				if sub == "" {
					continue
				}
				out := map[string]any{}
				for _, ns := range namespaces {
					perms := map[string]bool{}
					for _, act := range actions {
						ok, _ := deps.rbac.EnforceAsSubject(sub, "session", act, ns)
						perms[act] = ok
					}
					out[ns] = map[string]any{"session": perms}
				}
				subjects[sub] = out
			}
		}

		// Build response
		resp := map[string]any{
			"user": map[string]any{
				"subject":  cl.Sub,
				"roles":    cl.Roles,
				"provider": cl.Provider,
				"exp":      cl.ExpiresAt, // NEW
				"iat":      cl.IssuedAt,  // NEW
			},
			"cluster": map[string]any{
				"casbin": map[string]any{
					"namespaces": map[string]bool{"list": nsListAllowed},
				},
				"serverServiceAccount": sa,
			},
			"domains": domains,
			"namespaces": map[string]any{
				"userAllowed": userAllowed,
			},
		}

		if discover {
			resp["namespaces"].(map[string]any)["all"] = allNamespaces
			resp["namespaces"].(map[string]any)["withSessions"] = sessionNamespaces
		}
		if len(subjects) > 0 {
			resp["subjects"] = subjects
		}

		writeJSON(w, resp)
	}
}
