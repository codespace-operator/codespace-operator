package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	codespacev1alpha1 "github.com/codespace-operator/codespace-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

//go:embed all:static
var staticFS embed.FS

var gvr = schema.GroupVersionResource{
	Group:    codespacev1alpha1.GroupVersion.Group,
	Version:  codespacev1alpha1.GroupVersion.Version,
	Resource: "sessions",
}

func main() {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("in-cluster config: %v", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("dynamic client: %v", err)
	}

	mux := http.NewServeMux()

	// JSON API
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			ns := q(r, "namespace", "default")
			list, err := dyn.Resource(gvr).Namespace(ns).List(r.Context(), metav1.ListOptions{})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, list.Object["items"])
		case http.MethodPost:
			var u unstructured.Unstructured
			if err := json.NewDecoder(r.Body).Decode(&u.Object); err != nil {
				errJSON(w, err)
				return
			}
			ns := u.GetNamespace()
			if ns == "" {
				ns = "default"
			}
			obj, err := dyn.Resource(gvr).Namespace(ns).Create(r.Context(), &u, metav1.CreateOptions{})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, obj.Object)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		// /api/sessions/:ns/:name[/scale]
		p := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		parts := strings.Split(p, "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		ns, name := parts[0], parts[1]

		switch {
		case r.Method == http.MethodGet && len(parts) == 2:
			obj, err := dyn.Resource(gvr).Namespace(ns).Get(r.Context(), name, metav1.GetOptions{})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, obj.Object)

		case r.Method == http.MethodDelete && len(parts) == 2:
			if err := dyn.Resource(gvr).Namespace(ns).Delete(r.Context(), name, metav1.DeleteOptions{}); err != nil {
				errJSON(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPatch && len(parts) == 2:
			raw, _ := io.ReadAll(r.Body)
			obj, err := dyn.Resource(gvr).Namespace(ns).Patch(r.Context(), name, types.MergePatchType, raw, metav1.PatchOptions{})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, obj.Object)

		case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "scale":
			var body struct {
				Replicas int32 `json:"replicas"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				errJSON(w, err)
				return
			}
			patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, body.Replicas))
			obj, err := dyn.Resource(gvr).Namespace(ns).Patch(r.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, obj.Object)

		default:
			http.NotFound(w, r)
		}
	})

	// SSE watch
	mux.HandleFunc("/api/watch/sessions", func(w http.ResponseWriter, r *http.Request) {
		ns := q(r, "namespace", "default")
		watcher, err := dyn.Resource(gvr).Namespace(ns).Watch(r.Context(), metav1.ListOptions{Watch: true})
		if err != nil {
			errJSON(w, err)
			return
		}
		defer watcher.Stop()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return
				}
				if ev.Type == watch.Error {
					continue
				}
				out := map[string]any{"type": ev.Type, "object": ev.Object}
				b, _ := json.Marshal(out)
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
		}
	})

	// Static UI (embedded from /static)
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/ui/", func(w http.ResponseWriter, r *http.Request) {
		// fallback to index.html for client routing
		if strings.HasSuffix(r.URL.Path, "/") || path.Ext(r.URL.Path) == "" {
			r.URL.Path = "/static/index.html"
		}
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
	})

	addr := ":8080"
	srv := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("gateway listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

func q(r *http.Request, key, dflt string) string {
	if v := r.URL.Query().Get(key); v != "" {
		return v
	}
	return dflt
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func errJSON(w http.ResponseWriter, err error) {
	w.WriteHeader(500)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(200)
			return
		}
		next.ServeHTTP(w, r)
	})
}
