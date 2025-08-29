package helpers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

type topMeta struct {
	Kind        string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

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

// testKubernetesConnection verifies that we can connect to the Kubernetes API
func TestKubernetesConnection(c client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sessionList codespacev1.SessionList
	if err := c.List(ctx, &sessionList, client.InNamespace("default"), client.Limit(1)); err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}
	log.Printf("✅ Successfully connected to Kubernetes API (found %d sessions in default namespace)", len(sessionList.Items))
	return nil
}
func Itoa(i int32) string { return fmt.Sprintf("%d", i) }

type claims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

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

func GetBuildInfo() map[string]string {
	// These would typically be injected at build time with -ldflags
	return map[string]string{
		"version":   "1.0.0",                         // -X main.Version=$(git describe --tags)
		"gitCommit": "abc123",                        // -X main.GitCommit=$(git rev-parse HEAD)
		"buildDate": time.Now().Format(time.RFC3339), // -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
	}
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

func controllerOf(obj metav1.Object) *metav1.OwnerReference {
	for i := range obj.GetOwnerReferences() {
		if obj.GetOwnerReferences()[i].Controller != nil && *obj.GetOwnerReferences()[i].Controller {
			return &obj.GetOwnerReferences()[i]
		}
	}
	return nil
}

func ResolveTopController(ctx context.Context, cl client.Client, ns string, pod *corev1.Pod) (top topMeta, ok bool) {
	// 0) no controller → pod is top
	if ref := controllerOf(pod); ref == nil {
		return topMeta{"pod", pod.Name, pod.Labels, pod.Annotations}, false
	} else {
		return followController(ctx, cl, ns, *ref)
	}
}

func followController(ctx context.Context, cl client.Client, ns string, ref metav1.OwnerReference) (top topMeta, ok bool) {
	switch ref.Kind {

	case "ReplicaSet":
		var rs appsv1.ReplicaSet
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &rs); err != nil || string(rs.UID) != string(ref.UID) {
			return topMeta{}, false
		}
		// Promote RS to its controller if present (Deployment, Rollout, etc.)
		if parent := controllerOf(&rs); parent != nil {
			// known parent kinds first
			if parent.Kind == "Deployment" {
				var dep appsv1.Deployment
				if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: parent.Name}, &dep); err == nil && string(dep.UID) == string(parent.UID) {
					return topMeta{"deployment", dep.Name, dep.Labels, dep.Annotations}, true
				}
			}
			// generic follow (covers Rollout and other CRDs)
			if tm, ok := followGeneric(ctx, cl, ns, *parent); ok {
				return tm, true
			}
		}
		return topMeta{"replicaset", rs.Name, rs.Labels, rs.Annotations}, true

	case "StatefulSet":
		var ss appsv1.StatefulSet
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &ss); err != nil || string(ss.UID) != string(ref.UID) {
			return topMeta{}, false
		}
		return topMeta{"statefulset", ss.Name, ss.Labels, ss.Annotations}, true

	case "DaemonSet":
		var ds appsv1.DaemonSet
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &ds); err != nil || string(ds.UID) != string(ref.UID) {
			return topMeta{}, false
		}
		return topMeta{"daemonset", ds.Name, ds.Labels, ds.Annotations}, true

	case "Job":
		var job batchv1.Job
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &job); err != nil || string(job.UID) != string(ref.UID) {
			return topMeta{}, false
		}
		// Promote Job → CronJob if CronJob is controller
		if parent := controllerOf(&job); parent != nil && parent.Kind == "CronJob" {
			var cj batchv1.CronJob
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: parent.Name}, &cj); err == nil && string(cj.UID) == string(parent.UID) {
				return topMeta{"cronjob", cj.Name, cj.Labels, cj.Annotations}, true
			}
		}
		return topMeta{"job", job.Name, job.Labels, job.Annotations}, true

	case "ReplicationController": // legacy
		var rc corev1.ReplicationController
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &rc); err != nil || string(rc.UID) != string(ref.UID) {
			return topMeta{}, false
		}
		return topMeta{"replicationcontroller", rc.Name, rc.Labels, rc.Annotations}, true

	default:
		// Unknown kind (e.g., argoproj.io Rollout) → generic follow
		return followGeneric(ctx, cl, ns, ref)
	}
}

func followGeneric(ctx context.Context, cl client.Client, ns string, ref metav1.OwnerReference) (top topMeta, ok bool) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, u); err != nil || string(u.GetUID()) != string(ref.UID) {
		return topMeta{}, false
	}
	// If it itself has a controller, keep walking
	if parent := controllerOf(u); parent != nil {
		if tm, ok := followGeneric(ctx, cl, ns, *parent); ok {
			return tm, true
		}
	}
	// This object is the top
	k := strings.ToLower(ref.Kind)
	return topMeta{Kind: k, Name: u.GetName(), Labels: u.GetLabels(), Annotations: u.GetAnnotations()}, true
}
