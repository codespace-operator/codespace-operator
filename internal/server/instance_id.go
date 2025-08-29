package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const InstanceIDLabel = "codespace.dev/instance-id"
const cmPrefixName = "codespace-server-id"

// ManagerMeta describes how this codespace-server is managed.
type ManagerMeta struct {
	Kind      string // argo|helm|deployment|statefulset|daemonset|cronjob|job|pod|namespace|unresolved
	Name      string // release/app/deployment name (sanitized when used as label)
	Namespace string // namespace where the manager runs
}

type InstanceIDConfigMapData struct {
	id         string
	anchor     string
	clusterUID string
	version    string
}

type AnchorMeta struct {
	Kind, Namespace, Name string // e.g. Kind="helm", Name="my-release"
}

func (a AnchorMeta) String() string {
	// stable, parseable
	return fmt.Sprintf("%s:%s:%s", a.Kind, a.Namespace, a.Name)
}

func getClusterUID(ctx context.Context, cl client.Client) string {
	var ns corev1.Namespace
	if err := cl.Get(ctx, types.NamespacedName{Name: "kube-system"}, &ns); err == nil {
		return string(ns.UID)
	}
	if err := cl.Get(ctx, types.NamespacedName{Name: "default"}, &ns); err == nil {
		return string(ns.UID)
	}
	return "unknown"
}

func instanceIDv1(clusterUID string, anchor AnchorMeta) string {
	s := clusterUID + "|" + anchor.String()
	sum := sha256.Sum256([]byte(s))
	// 20 bytes -> 40 hex chars
	return "i1-" + hex.EncodeToString(sum[:20])
}

func ensureInstallationID(ctx context.Context, cl client.Client) (string, error) {
	ns := inClusterNamespace()
	anchor := detectAnchor(ctx, cl, ns)
	clusterUID := getClusterUID(ctx, cl)
	id := instanceIDv1(clusterUID, anchor)

	// ConfigMap name derives from the (stable) anchor, like before
	cmName := fmt.Sprintf("%s-%s", cmPrefixName, k8sHexHash(anchor.String(), 10))
	key := client.ObjectKey{Namespace: ns, Name: cmName}

	// If CM exists, trust it (but if it has a legacy random id, we don't change it)
	var cm corev1.ConfigMap
	if err := cl.Get(ctx, key, &cm); err == nil {
		if v := cm.Data["id"]; v != "" {
			return v, nil
		}
		// backfill if present but missing 'id'
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}

	// (Re)create index CM with deterministic content
	cm = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: ns,
			Labels:    buildConfigMapSafeLabels(ctx, cl, ns),
		},
		Data: map[string]string{
			"id":         id,
			"anchor":     anchor.String(),
			"clusterUID": clusterUID,
			"version":    "1",
		},
	}
	if err := cl.Create(ctx, &cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", err
	}
	return id, nil
}

/* ============================
   Helpers: current pod & top owner
   ============================ */

type topMeta struct {
	Kind        string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

func getCurrentPod(ctx context.Context, cl client.Client, ns string) (*corev1.Pod, error) {
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

// resolveTopController walks ownerReferences to a stable, top-level controller.
// Handles: Deployment (via ReplicaSet), StatefulSet, DaemonSet, Job->CronJob, Job.
func resolveTopController(ctx context.Context, cl client.Client, ns string, pod *corev1.Pod) (top topMeta, ok bool) {
	for _, owner := range pod.OwnerReferences {
		switch owner.Kind {
		case "ReplicaSet":
			var rs appsv1.ReplicaSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &rs); err == nil {
				// Prefer Deployment above the RS
				for _, ro := range rs.OwnerReferences {
					if ro.Kind == "Deployment" {
						var dep appsv1.Deployment
						if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: ro.Name}, &dep); err == nil {
							return topMeta{
								Kind:        "deployment",
								Name:        dep.Name,
								Labels:      dep.Labels,
								Annotations: dep.Annotations,
							}, true
						}
					}
				}
				// If no Deployment owner, consider RS as controller (rare but possible)
				return topMeta{
					Kind:        "replicaset",
					Name:        rs.Name,
					Labels:      rs.Labels,
					Annotations: rs.Annotations,
				}, true
			}
		case "Deployment":
			var dep appsv1.Deployment
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &dep); err == nil {
				return topMeta{"deployment", dep.Name, dep.Labels, dep.Annotations}, true
			}
		case "StatefulSet":
			var ss appsv1.StatefulSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &ss); err == nil {
				return topMeta{"statefulset", ss.Name, ss.Labels, ss.Annotations}, true
			}
		case "DaemonSet":
			var ds appsv1.DaemonSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &ds); err == nil {
				return topMeta{"daemonset", ds.Name, ds.Labels, ds.Annotations}, true
			}
		case "Job":
			var job batchv1.Job
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &job); err == nil {
				// If CronJob owns the Job, promote to CronJob
				for _, jo := range job.OwnerReferences {
					if jo.Kind == "CronJob" {
						var cj batchv1.CronJob
						if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: jo.Name}, &cj); err == nil {
							return topMeta{"cronjob", cj.Name, cj.Labels, cj.Annotations}, true
						}
					}
				}
				return topMeta{"job", job.Name, job.Labels, job.Annotations}, true
			}
		}
	}
	// No controller; treat pod itself as the "top"
	return topMeta{"pod", pod.Name, pod.Labels, pod.Annotations}, false
}

/* ============================
   Helpers: Argo & Helm extraction (top-first, then pod)
   ============================ */

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

/* ============================
   Manager identity (top-first)
   ============================ */

// getSelfManagerMeta inspects the current Pod & owners to determine manager identity.
func managerFromLabels(ns string, labels, ann map[string]string) (ManagerMeta, bool) {
	if app := argoAppName(labels, ann); app != "" {
		return ManagerMeta{Kind: "argo", Name: sanitizeLabelValue(app), Namespace: ns}, true
	}
	if rel := helmReleaseName(labels, ann); rel != "" {
		return ManagerMeta{Kind: "helm", Name: sanitizeLabelValue(rel), Namespace: ns}, true
	}
	return ManagerMeta{}, false
}

func getSelfManagerMeta(ctx context.Context, cl client.Client) ManagerMeta {
	ns := inClusterNamespace()

	pod, err := getCurrentPod(ctx, cl, ns)
	if err != nil || pod == nil {
		// We can’t read labels if we don’t know the pod; report unresolved.
		logger.Info("getSelfManagerMeta: current pod not found", "namespace", ns, "err", err)
		return ManagerMeta{Kind: "unresolved", Name: "unresolved", Namespace: ns}
	}

	top, hasTop := resolveTopController(ctx, cl, ns, pod)

	// 1) Prefer ARGO/HELM from the TOP controller’s labels/annotations.
	if hasTop {
		if mm, ok := managerFromLabels(ns, top.Labels, top.Annotations); ok {
			return mm // Argo wins over Helm; early return
		}
		// 2) Otherwise, fall back to the top controller’s identity.
		if top.Kind != "" && top.Name != "" {
			return ManagerMeta{Kind: top.Kind, Name: sanitizeLabelValue(top.Name), Namespace: ns}
		}
	}

	// 3) No top controller (ownerless pod) or nothing on top: try the POD’s own labels.
	if mm, ok := managerFromLabels(ns, pod.Labels, pod.Annotations); ok {
		return mm
	}

	// 4) Final fallback: the pod itself.
	return ManagerMeta{Kind: "pod", Name: sanitizeLabelValue(pod.Name), Namespace: ns}
}

/* ============================
   Stable anchor (top-first; Argo > Helm > controller > namespace)
   ============================ */

// determineStableAnchor creates a stable identity based on deployment context
func detectAnchor(ctx context.Context, cl client.Client, ns string) AnchorMeta {
	pod, err := getCurrentPod(ctx, cl, ns)
	if err != nil || pod == nil {
		return AnchorMeta{Kind: "namespace", Namespace: ns, Name: "server"}
	}
	top, hasTop := resolveTopController(ctx, cl, ns, pod)

	// Top-first Argo/Helm
	if hasTop {
		if mm, ok := managerFromLabels(ns, top.Labels, top.Annotations); ok {
			return AnchorMeta{Kind: mm.Kind, Namespace: ns, Name: mm.Name}
		}
	}
	// Pod labels if nothing on top
	if mm, ok := managerFromLabels(ns, pod.Labels, pod.Annotations); ok {
		return AnchorMeta{Kind: mm.Kind, Namespace: ns, Name: mm.Name}
	}
	// Then controller identity (stable), finally namespace (most stable)
	if hasTop {
		return AnchorMeta{Kind: top.Kind, Namespace: ns, Name: sanitizeLabelValue(top.Name)}
	}
	return AnchorMeta{Kind: "namespace", Namespace: ns, Name: "server"}
}

/* ============================
   Labels
   ============================ */

// buildSafeLabels creates Kubernetes-safe labels for
func buildConfigMapSafeLabels(ctx context.Context, cl client.Client, ns string) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/part-of":    "codespace-operator",
		"app.kubernetes.io/managed-by": APP_NAME,
		"app.kubernetes.io/component":  "server",
	}

	pod, err := getCurrentPod(ctx, cl, ns)
	if err != nil || pod == nil {
		labels["codespace.dev/method"] = "kubectl"
		return labels
	}
	top, hasTop := resolveTopController(ctx, cl, ns, pod)

	if hasTop {
		if mm, ok := managerFromLabels(ns, top.Labels, top.Annotations); ok {
			if mm.Kind == "argo" {
				labels["codespace.dev/method"] = "argo"
				labels["codespace.dev/argo-app"] = mm.Name
				return labels
			}
			if mm.Kind == "helm" {
				labels["codespace.dev/method"] = "helm"
				labels["codespace.dev/release"] = mm.Name
				return labels
			}
		}
	}

	if mm, ok := managerFromLabels(ns, pod.Labels, pod.Annotations); ok {
		if mm.Kind == "argo" {
			labels["codespace.dev/method"] = "argo"
			labels["codespace.dev/argo-app"] = mm.Name
			return labels
		}
		if mm.Kind == "helm" {
			labels["codespace.dev/method"] = "helm"
			labels["codespace.dev/release"] = mm.Name
			return labels
		}
	}

	// Heuristic fallback
	if _, ok := pod.Labels["helm.sh/chart"]; ok {
		labels["codespace.dev/method"] = "helm"
	} else {
		labels["codespace.dev/method"] = "kubectl"
	}
	return labels
}

/* ============================
   Sanitization & index
   ============================ */

// sanitizeLabelValue ensures label values are Kubernetes-compliant
func sanitizeLabelValue(value string) string {
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
		safe = safe[:60] + k8sHexHash(value, 1) // Add hash suffix for uniqueness
	}

	return safe
}

// buildInstanceMetaIndex scans per-instance ConfigMaps and returns instanceID -> ManagerMeta.
func (h *handlers) buildInstanceMetaIndex(r *http.Request) map[string]ManagerMeta {
	out := map[string]ManagerMeta{}

	var cms corev1.ConfigMapList
	sel := client.MatchingLabels{
		"app.kubernetes.io/part-of":   APP_NAME,
		"app.kubernetes.io/component": "server",
	}
	if err := h.deps.client.List(r.Context(), &cms, sel); err != nil {
		logger.Debug("buildInstanceMetaIndex: list configmaps failed", "err", err)
		return out
	}

	for _, cm := range cms.Items {
		id := cm.Data["id"]
		if id == "" {
			continue
		}

		var meta ManagerMeta

		// 1) Prefer the stable anchor: "<kind>:<ns>:<name>"
		if a := cm.Data["anchor"]; a != "" {
			parts := strings.Split(a, ":")
			if len(parts) >= 3 {
				kind := parts[0]
				ns := parts[1]
				name := sanitizeLabelValue(parts[2])

				// recognize expanded set of kinds
				switch kind {
				case "helm", "argo", "deployment", "statefulset", "daemonset", "cronjob", "job", "pod", "namespace":
					// ok
				default:
					kind = "unresolved"
					if name == "" {
						name = "unresolved"
					}
				}
				meta = ManagerMeta{Kind: kind, Namespace: ns, Name: name}
			}
		}

		// 2) Fallback for very old CMs that predate 'anchor'
		if meta.Kind == "" {
			method := cm.Labels["codespace.dev/method"] // helm|argo|kubectl
			switch method {
			case "helm":
				meta = ManagerMeta{Kind: "helm", Namespace: cm.Namespace, Name: sanitizeLabelValue(cm.Labels["codespace.dev/release"])}
				if meta.Name == "" {
					meta.Name = "release"
				}
			case "argo":
				meta = ManagerMeta{Kind: "argo", Namespace: cm.Namespace, Name: sanitizeLabelValue(cm.Labels["codespace.dev/argo-app"])}
				if meta.Name == "" {
					meta.Name = "app"
				}
			default:
				meta = ManagerMeta{Kind: "unresolved", Namespace: cm.Namespace, Name: "unresolved"}
			}
		}

		out[id] = meta
	}

	return out
}
