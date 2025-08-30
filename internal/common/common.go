package common

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/spf13/viper"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

// retryOnConflict runs fn with standard backoff if a 409 occurs.
func RetryOnConflict(fn func() error) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return fn()
	})
}

// buildKubeConfig creates a Kubernetes client config that works both locally and in-cluster
func BuildKubeConfig() (*rest.Config, error) {
	// 1. Try in-cluster config first (when running inside a pod)
	if cfg, err := rest.InClusterConfig(); err == nil {
		log.Println("Using in-cluster Kubernetes config")
		return cfg, nil
	}

	// 2. Try KUBECONFIG environment variable
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		log.Printf("Using KUBECONFIG from environment: %s", kubeconfig)
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	// 3. Try default kubeconfig location (~/.kube/config)
	if home := homedir.HomeDir(); home != "" {
		kubeconfig := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(kubeconfig); err == nil {
			log.Printf("Using default kubeconfig: %s", kubeconfig)
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}

	// 4. Try service account token (alternative in-cluster method)
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		log.Println("Using service account token")
		return rest.InClusterConfig()
	}

	return nil, fmt.Errorf("unable to create Kubernetes client config: tried in-cluster, KUBECONFIG, and ~/.kube/config")
}
func RandB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// TestKubernetesConnection verifies that we can connect to the Kubernetes API
func TestKubernetesConnection(c client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sessionList codespacev1.SessionList
	// It would make more sense to test to its own namespace
	if err := c.List(ctx, &sessionList, client.InNamespace("default"), client.Limit(1)); err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}
	log.Printf("✅ Successfully connected to Kubernetes API (found %d sessions in default namespace)", len(sessionList.Items))
	return nil
}
func Itoa(i int32) string { return fmt.Sprintf("%d", i) }

// setupViper configures common Viper settings.
// envPrefix: e.g. "CODESPACE_SERVER"
// fileBase:  e.g. "server-config" (-> server-config.yaml)
func SetupViper(v *viper.Viper, envPrefix, fileBase string) {
	// --- Environment (UPPER_SNAKE with prefix) ---
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Logging helper (default logger only after logger is setup)
	now := func() string { return time.Now().Format(time.RFC3339) }
	log := func(f string, a ...any) {
		fmt.Fprintf(os.Stderr, now()+" "+f+"\n", a...)
	}

	// --- Single knob: <PREFIX>_CONFIG_DEFAULT_PATH (file OR directory) ---
	var dirOverride string
	if raw := strings.TrimSpace(os.Getenv(envPrefix + "_CONFIG_DEFAULT_PATH")); raw != "" {
		p := os.ExpandEnv(raw)

		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			// IMPORTANT: pass the DIRECTORY to AddConfigPath, not a file path.
			dirOverride = p // remember to search here first
		} else {
			// Treat as a FILE path (relative or absolute)
			if !filepath.IsAbs(p) {
				if abs, err := filepath.Abs(p); err == nil {
					p = abs
				}
			}
			if _, err := os.Stat(p); err != nil {
				panic(fmt.Errorf("%s_CONFIG_DEFAULT_PATH points to missing file: %s (err=%w)", envPrefix, p, err))
			}
			v.SetConfigFile(p)
			if err := v.ReadInConfig(); err != nil {
				panic(fmt.Errorf("failed to read %s_CONFIG_DEFAULT_PATH=%s: %w", envPrefix, p, err))
			}
			log("loaded config override (file): %s", v.ConfigFileUsed())
			return
		}
	}

	// --- Directory search mode ---
	v.SetConfigName(fileBase)
	v.SetConfigType("yaml")

	// If a dir override was provided, search it FIRST (can be relative)
	if dirOverride != "" {
		v.AddConfigPath(dirOverride)
	}

	// Default search locations (in order)
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/codespace-operator/")
	v.AddConfigPath("$HOME/.codespace-operator/")

	// Optional file - ignore if missing
	if err := v.ReadInConfig(); err == nil {
		log("loaded config (search): %s", v.ConfigFileUsed())
	} else {
		log("no config file found via search (env-only is fine)")
	}
}

func SplitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// StructToMapStringString converts any struct (or pointer to struct)
// into a flat map[string]string. Only exported fields are included.
func StructToMapStringString(v any) (map[string]string, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, errors.New("nil value")
	}
	// Dereference pointers
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, errors.New("nil pointer")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", rv.Kind())
	}

	out := make(map[string]string)
	walkStruct(rv, "", out)
	return out, nil
}

func walkStruct(val reflect.Value, prefix string, out map[string]string) {
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)
		if sf.PkgPath != "" { // unexported
			continue
		}
		// Respect json tags
		name, ok := jsonFieldName(sf)
		if !ok {
			continue // tag "-"
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		fv := val.Field(i)
		// Deref pointers
		for fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				out[key] = ""
				goto nextField
			}
			fv = fv.Elem()
		}

		switch fv.Kind() {
		case reflect.Struct:
			// Special-case time.Time
			if fv.Type().PkgPath() == "time" && fv.Type().Name() == "Time" {
				out[key] = fv.Interface().(time.Time).Format(time.RFC3339)
			} else {
				walkStruct(fv, key, out)
			}
		case reflect.Slice, reflect.Array:
			var parts []string
			for j := 0; j < fv.Len(); j++ {
				parts = append(parts, fmt.Sprint(fv.Index(j).Interface()))
			}
			out[key] = strings.Join(parts, ",")
		case reflect.Map:
			// If it's a map with string keys, expand it; otherwise stringify.
			if fv.Type().Key().Kind() == reflect.String {
				keys := fv.MapKeys()
				sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
				for _, mk := range keys {
					subKey := key + "." + mk.String()
					out[subKey] = fmt.Sprint(fv.MapIndex(mk).Interface())
				}
			} else {
				out[key] = fmt.Sprint(fv.Interface())
			}
		default:
			out[key] = fmt.Sprint(fv.Interface())
		}
	nextField:
	}
}

func jsonFieldName(sf reflect.StructField) (string, bool) {
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	if tag == "" {
		// For anonymous embedded structs without a tag, use their type name
		// (walkStruct will still recurse into them anyway).
		return sf.Name, true
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" { // `json:",omitempty"`
		name = sf.Name
	}
	return name, true
}

func lowerKind(k string) string { return strings.ToLower(k) }

func K8sHexHash(s string, bytes int) string {
	if bytes <= 0 || bytes > 32 {
		bytes = 10 // 10 bytes -> 20 hex chars
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:bytes])
}

// SubjectToLabelID returns a stable, label-safe ID for a user/subject.
// Format: s256-<40 hex> (first 20 bytes of SHA-256 => 40 hex chars). Total length 45.
func SubjectToLabelID(sub string) string {
	if sub == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sub))
	return "s256-" + hex.EncodeToString(sum[:20]) // 160-bit truncation; label-safe; <=63
}
