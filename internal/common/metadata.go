package common

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InstanceIDLabel        = "codespace.dev/instance-id"
	LabelCreatedBy         = "codespace.dev/created-by"     // hashed, label-safe
	AnnotationCreatedBy    = "codespace.dev/created-by"     // raw subject
	AnnotationCreatedBySig = "codespace.dev/created-by.sig" // optional: HMAC of raw subject
)

/*
	We backfill for existing sessions (cluster-scope) during list/stream:
	the server builds a map of instance-id -> manager meta by scanning the per-instance ConfigMaps it already creates (the ones named like codespace-server-instance-*)
	the server enriches each returned session with the same labels in-memory (no persistence needed).
*/
// Manager identity labels stamped on Session objects
const (
	LabelManagerType      = "codespace.dev/manager-type" // helm|argo|deployment|statefulset|namespace
	LabelManagerName      = "codespace.dev/manager-name" // release/app/deployment name (sanitized)
	LabelManagerNamespace = "codespace.dev/manager-ns"   // namespace the manager runs in
)

type topMeta struct {
	Kind        string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

// AnchorMeta describes how an instance is managed.
type AnchorMeta struct {
	Type      string // argo|helm|deployment|statefulset|daemonset|cronjob|job|pod|namespace|unresolved
	Name      string // release/app/deployment name (sanitized when used as label)
	Namespace string // namespace where the manager runs
}

func (a AnchorMeta) String() string {
	// stable, parseable
	return fmt.Sprintf("%s:%s:%s", a.Type, a.Namespace, a.Name)
}

// returns the single controller owner (if any)
func controllerOf(obj metav1.Object) *metav1.OwnerReference {
	for i := range obj.GetOwnerReferences() {
		or := obj.GetOwnerReferences()[i]
		if or.Controller != nil && *or.Controller {
			return &or
		}
	}
	return nil
}

func ManagerFromLabels(ns string, labels, ann map[string]string) (AnchorMeta, bool) {
	if app := argoAppName(labels, ann); app != "" {
		return AnchorMeta{Type: "argo", Name: SanitizeLabelValue(app), Namespace: ns}, true
	}
	if rel := helmReleaseName(labels, ann); rel != "" {
		return AnchorMeta{Type: "helm", Name: SanitizeLabelValue(rel), Namespace: ns}, true
	}
	return AnchorMeta{}, false
}

// ResolveTopController walks up controller refs to the top controller it can see.
// It never errors out: it returns the best it can and a signal if RBAC likely limited us.
// - top: the best top object we could determine (labels/annos only present if fetched)
// - ok:  true if the pod had a controller (false => top is the pod)
// - rbacLimited: true if we hit Forbidden/Unauthorized anywhere during the walk
func ResolveTopController(ctx context.Context, cl client.Client, ns string, pod *corev1.Pod) (top topMeta, ok bool, rbacLimited bool) {
	if pod == nil {
		return topMeta{"pod", "", nil, nil}, false, false
	}
	ref := controllerOf(pod)
	if ref == nil {
		return topMeta{"pod", pod.Name, pod.Labels, pod.Annotations}, false, false
	}
	return followController(ctx, cl, ns, *ref)
}

func followController(ctx context.Context, cl client.Client, ns string, ref metav1.OwnerReference) (top topMeta, ok bool, rbacLimited bool) {
	switch ref.Kind {

	case "ReplicaSet":
		var rs appsv1.ReplicaSet
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &rs); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return topMeta{Kind: lowerKind(ref.Kind), Name: ref.Name}, true, true
			}
			// best-effort even on NotFound or other errors
			return topMeta{Kind: lowerKind(ref.Kind), Name: ref.Name}, true, false
		}
		// promote to its controller if present (Deployment, Rollout, etc.)
		if parent := controllerOf(&rs); parent != nil {
			// known fast-path: Deployment
			if parent.Kind == "Deployment" {
				var dep appsv1.Deployment
				err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: parent.Name}, &dep)
				if err == nil {
					return topMeta{"deployment", dep.Name, dep.Labels, dep.Annotations}, true, false
				}
				if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
					return topMeta{Kind: "deployment", Name: parent.Name}, true, true
				}
				return topMeta{Kind: "deployment", Name: parent.Name}, true, false
			}
			// generic for CRDs (e.g., argoproj.io Rollout)
			return followGeneric(ctx, cl, ns, *parent)
		}
		return topMeta{"replicaset", rs.Name, rs.Labels, rs.Annotations}, true, false

	case "StatefulSet":
		var ss appsv1.StatefulSet
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &ss); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return topMeta{Kind: "statefulset", Name: ref.Name}, true, true
			}
			return topMeta{Kind: "statefulset", Name: ref.Name}, true, false
		}
		return topMeta{"statefulset", ss.Name, ss.Labels, ss.Annotations}, true, false

	case "DaemonSet":
		var ds appsv1.DaemonSet
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &ds); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return topMeta{Kind: "daemonset", Name: ref.Name}, true, true
			}
			return topMeta{Kind: "daemonset", Name: ref.Name}, true, false
		}
		return topMeta{"daemonset", ds.Name, ds.Labels, ds.Annotations}, true, false

	case "Job":
		var job batchv1.Job
		if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &job); err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return topMeta{Kind: "job", Name: ref.Name}, true, true
			}
			return topMeta{Kind: "job", Name: ref.Name}, true, false
		}
		// promote to CronJob when applicable
		if parent := controllerOf(&job); parent != nil && parent.Kind == "CronJob" {
			var cj batchv1.CronJob
			err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: parent.Name}, &cj)
			if err == nil {
				return topMeta{"cronjob", cj.Name, cj.Labels, cj.Annotations}, true, false
			}
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return topMeta{Kind: "cronjob", Name: parent.Name}, true, true
			}
			return topMeta{Kind: "cronjob", Name: parent.Name}, true, false
		}
		return topMeta{"job", job.Name, job.Labels, job.Annotations}, true, false

	case "ReplicationController":
		var rc corev1.ReplicationController
		err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, &rc)
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return topMeta{Kind: "replicationcontroller", Name: ref.Name}, true, true
			}
			return topMeta{Kind: "replicationcontroller", Name: ref.Name}, true, false
		}
		return topMeta{"replicationcontroller", rc.Name, rc.Labels, rc.Annotations}, true, false

	default:
		// unknown kind (CRD) → best-effort generic
		return followGeneric(ctx, cl, ns, ref)
	}
}

func followGeneric(ctx context.Context, cl client.Client, ns string, ref metav1.OwnerReference) (top topMeta, ok bool, rbacLimited bool) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ref.Name}, u); err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return topMeta{Kind: lowerKind(ref.Kind), Name: ref.Name}, true, true
		}
		return topMeta{Kind: lowerKind(ref.Kind), Name: ref.Name}, true, false
	}
	// If it has a controller, keep walking
	if parent := controllerOf(u); parent != nil {
		return followGeneric(ctx, cl, ns, *parent)
	}
	return topMeta{Kind: lowerKind(ref.Kind), Name: u.GetName(), Labels: u.GetLabels(), Annotations: u.GetAnnotations()}, true, false
}

func GetCurrentPod(ctx context.Context, cl client.Client, ns string) (*corev1.Pod, error) {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		var err error
		podName, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}
	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

// Helm: prefer meta.helm.sh/release-name annotation.
// If absent, accept app.kubernetes.io/instance **only if** we see strong Helm signals.
func helmReleaseName(labels, ann map[string]string) string {
	if v := ann["meta.helm.sh/release-name"]; v != "" {
		return v
	}
	rel := labels["app.kubernetes.io/instance"]
	if rel == "" {
		return ""
	}
	// Guard against Argo collision by requiring Helm markers.
	if labels["app.kubernetes.io/managed-by"] == "Helm" || labels["helm.sh/chart"] != "" || ann["helm.sh/chart"] != "" {
		return rel
	}
	return ""
}

// We treat only argocd.argoproj.io/instance as Argo to avoid confusion with Helm's use of app.kubernetes.io/instance.
func argoAppName(labels, ann map[string]string) string {
	if v := labels["argocd.argoproj.io/instance"]; v != "" {
		return v
	}
	if v := ann["argocd.argoproj.io/instance"]; v != "" {
		return v
	}
	return ""
}
func SanitizeLabelValue(value string) string {
	// K8s label values must be <= 63 chars, start/end with alphanumeric
	// and contain only alphanumeric, '-', '_', '.'

	if len(value) == 0 {
		return "unknown"
	}

	// Replace invalid characters
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, value)

	// Ensure it starts and ends with alphanumeric
	safe = strings.Trim(safe, "-_.")
	if len(safe) == 0 {
		return "unknown"
	}

	// Truncate if too long
	if len(safe) > 63 {
		safe = safe[:60] + K8sHexHash(value, 1) // Add hash suffix for uniqueness
	}

	return safe
}
func GetInClusterNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	if b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return "ns-unresolved"
}

// GetSelfAnchorMeta returns the resolved manager meta and whether RBAC likely limited resolution.
func GetSelfAnchorMeta(ctx context.Context, cl client.Client) (AnchorMeta, bool /*rbacLimited*/) {
	ns := GetInClusterNamespace()

	pod, err := GetCurrentPod(ctx, cl, ns)
	if err != nil || pod == nil {
		logger.Warn("GetSelfAnchorMeta: current pod not found", "namespace", ns, "err", err)
		return AnchorMeta{Type: "unresolved", Name: "unresolved", Namespace: ns}, false
	}

	top, hasTop, rbacLimited := ResolveTopController(ctx, cl, ns, pod)

	// Prefer Argo/Helm from TOP labels (if we fetched labels)
	if hasTop && top.Labels != nil {
		if app := argoAppName(top.Labels, top.Annotations); app != "" {
			return AnchorMeta{Type: "argo", Name: SanitizeLabelValue(app), Namespace: ns}, rbacLimited
		}
		if rel := helmReleaseName(top.Labels, top.Annotations); rel != "" {
			return AnchorMeta{Type: "helm", Name: SanitizeLabelValue(rel), Namespace: ns}, rbacLimited
		}
	}

	// Try pod labels only when top didn't yield a manager (or labels unavailable)
	if app := argoAppName(pod.Labels, pod.Annotations); app != "" {
		return AnchorMeta{Type: "argo", Name: SanitizeLabelValue(app), Namespace: ns}, rbacLimited
	}
	if rel := helmReleaseName(pod.Labels, pod.Annotations); rel != "" {
		return AnchorMeta{Type: "helm", Name: SanitizeLabelValue(rel), Namespace: ns}, rbacLimited
	}

	// Fall back to top identity (even if it came from OwnerRef only)
	if hasTop && top.Kind != "" && top.Name != "" {
		return AnchorMeta{Type: top.Kind, Name: SanitizeLabelValue(top.Name), Namespace: ns}, rbacLimited
	}

	// Finally: pod identity
	return AnchorMeta{Type: "pod", Name: SanitizeLabelValue(pod.Name), Namespace: ns}, rbacLimited
}

func GetClusterUID(ctx context.Context, cl client.Client) string {
	var ns corev1.Namespace
	if err := cl.Get(ctx, types.NamespacedName{Name: "kube-system"}, &ns); err == nil {
		return string(ns.UID)
	}
	if err := cl.Get(ctx, types.NamespacedName{Name: "default"}, &ns); err == nil {
		return string(ns.UID)
	}
	return "unknown"
}

// CLUSTER_UID=$(kubectl get ns kube-system -o jsonpath='{.metadata.uid}' 2>/dev/null || kubectl get ns default -o jsonpath='{.metadata.uid}'); \
// ANCHOR="helm:my-ns:my-release"; \
// printf 'i1-%s\n' "$(printf '%s' "$CLUSTER_UID|$ANCHOR" | sha256sum | awk '{print $1}')"
func InstanceIDv1(clusterUID string, anchor AnchorMeta) string {
	s := clusterUID + "|" + anchor.String()
	sum := sha256.Sum256([]byte(s))
	// 20 bytes -> 40 hex chars   (label-safe: 3 + 40 = 43)
	return "i1-" + hex.EncodeToString(sum[:20])
}

// detectAnchor creates a stable identity based on deployment context
func DetectAnchor(ctx context.Context, cl client.Client, ns string) AnchorMeta {
	pod, err := GetCurrentPod(ctx, cl, ns)
	if err != nil || pod == nil {
		return AnchorMeta{Type: "kind-unresolved", Namespace: ns, Name: "owner-unresolved"}
	}
	top, hasTop, _ := ResolveTopController(ctx, cl, ns, pod)

	// Top-first Argo/Helm
	if hasTop {
		if mm, ok := ManagerFromLabels(ns, top.Labels, top.Annotations); ok {
			return AnchorMeta{Type: mm.Type, Namespace: ns, Name: mm.Name}
		}
	}
	// Pod labels if nothing on top
	if mm, ok := ManagerFromLabels(ns, pod.Labels, pod.Annotations); ok {
		return AnchorMeta{Type: mm.Type, Namespace: ns, Name: mm.Name}
	}
	// Then controller identity (stable), finally namespace (most stable)
	if hasTop {
		return AnchorMeta{Type: top.Kind, Namespace: ns, Name: SanitizeLabelValue(top.Name)}
	}
	return AnchorMeta{Type: "kind-unresolved", Namespace: ns, Name: "owner-unresolved"}
}
