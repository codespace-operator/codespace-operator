package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagerMeta describes how this codespace-server is managed.
type ManagerMeta struct {
	Kind      string // helm|argo|deployment|statefulset|namespace
	Name      string // release/app/deployment name (sanitized when used as label)
	Namespace string // namespace where the manager runs
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
			Labels:    buildSafeLabels(ctx, cl, ns),
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

// getSelfManagerMeta inspects the current Pod & owners to determine manager identity.
func getSelfManagerMeta(ctx context.Context, cl client.Client) ManagerMeta {
	ns := inClusterNamespace()
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return ManagerMeta{Kind: "namespace", Name: "server", Namespace: ns}
	}

	// Prefer Helm
	if release, ok := pod.Labels["app.kubernetes.io/instance"]; ok {
		return ManagerMeta{Kind: "helm", Name: sanitizeLabelValue(release), Namespace: ns}
	}
	// Then ArgoCD
	if app, ok := pod.Labels["argocd.argoproj.io/instance"]; ok {
		return ManagerMeta{Kind: "argo", Name: sanitizeLabelValue(app), Namespace: ns}
	}

	// Walk owners for Deployment/StatefulSet
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			var rs appsv1.ReplicaSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &rs); err == nil {
				for _, o := range rs.OwnerReferences {
					if o.Kind == "Deployment" {
						return ManagerMeta{Kind: "deployment", Name: sanitizeLabelValue(o.Name), Namespace: ns}
					}
				}
			}
		}
		if owner.Kind == "StatefulSet" {
			return ManagerMeta{Kind: "statefulset", Name: sanitizeLabelValue(owner.Name), Namespace: ns}
		}
	}

	// Fallback
	return ManagerMeta{Kind: "namespace", Name: "server", Namespace: ns}
}

// determineStableAnchor creates a stable identity based on deployment context
func detectAnchor(ctx context.Context, cl client.Client, ns string) AnchorMeta {
	if s := getHelmAnchor(ctx, cl, ns); s != "" {
		// s looks like "helm:<ns>:<release>" or "helm:<ns>:<release>:<chart>"
		parts := strings.Split(s, ":")
		return AnchorMeta{Kind: "helm", Namespace: parts[1], Name: parts[2]}
	}
	if s := getArgoAnchor(ctx, cl, ns); s != "" {
		parts := strings.Split(s, ":")
		return AnchorMeta{Kind: "argo", Namespace: parts[1], Name: parts[2]}
	}
	if s := getDeploymentAnchor(ctx, cl, ns); s != "" {
		parts := strings.Split(s, ":")
		return AnchorMeta{Kind: "deployment", Namespace: parts[1], Name: parts[2]}
	}
	if s := getStatefulSetAnchor(ctx, cl, ns); s != "" {
		parts := strings.Split(s, ":")
		return AnchorMeta{Kind: "statefulset", Namespace: parts[1], Name: parts[2]}
	}
	return AnchorMeta{Kind: "namespace", Namespace: ns, Name: "server"}
}

// getHelmAnchor looks for Helm release information
func getHelmAnchor(ctx context.Context, cl client.Client, ns string) string {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return ""
	}

	if release, ok := pod.Labels["app.kubernetes.io/instance"]; ok {
		if chart, ok := pod.Labels["helm.sh/chart"]; ok {
			return fmt.Sprintf("helm:%s:%s:%s", ns, release, chart) // <-- versioned, unstable
		}
		return fmt.Sprintf("helm:%s:%s", ns, release)
	}

	// Check owner references for Helm-managed resources
	return getHelmFromOwners(ctx, cl, ns, pod.OwnerReferences)
}

// getHelmFromOwners walks up the owner chain looking for Helm metadata
func getHelmFromOwners(ctx context.Context, cl client.Client, ns string, owners []metav1.OwnerReference) string {
	for _, owner := range owners {
		if owner.Kind == "ReplicaSet" {
			var rs appsv1.ReplicaSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &rs); err == nil {
				if release, ok := rs.Labels["app.kubernetes.io/instance"]; ok {
					return fmt.Sprintf("helm:%s:%s", ns, release)
				}
				// Check Deployment parent
				return getHelmFromOwners(ctx, cl, ns, rs.OwnerReferences)
			}
		}
		if owner.Kind == "StatefulSet" {
			var dep appsv1.StatefulSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &dep); err == nil {
				if release, ok := dep.Labels["app.kubernetes.io/instance"]; ok {
					return fmt.Sprintf("helm:%s:%s", ns, release)
				}
			}
		}
		if owner.Kind == "Deployment" {
			var dep appsv1.Deployment
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &dep); err == nil {
				if release, ok := dep.Labels["app.kubernetes.io/instance"]; ok {
					return fmt.Sprintf("helm:%s:%s", ns, release)
				}
			}
		}
	}
	return ""
}

// getArgoAnchor looks for ArgoCD application information
func getArgoAnchor(ctx context.Context, cl client.Client, ns string) string {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return ""
	}

	// Check for ArgoCD annotations/labels
	if app, ok := pod.Labels["argocd.argoproj.io/instance"]; ok {
		return fmt.Sprintf("argo:%s:%s", ns, app)
	}

	if app, ok := pod.Annotations["argocd.argoproj.io/instance"]; ok {
		return fmt.Sprintf("argo:%s:%s", ns, app)
	}

	return getArgoFromOwners(ctx, cl, ns, pod.OwnerReferences)
}

// getArgoFromOwners walks up the owner chain looking for ArgoCD metadata
func getArgoFromOwners(ctx context.Context, cl client.Client, ns string, owners []metav1.OwnerReference) string {
	for _, owner := range owners {
		if owner.Kind == "ReplicaSet" {
			var rs appsv1.ReplicaSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &rs); err == nil {
				if app, ok := rs.Labels["argocd.argoproj.io/instance"]; ok {
					return fmt.Sprintf("argo:%s:%s", ns, app)
				}
				return getArgoFromOwners(ctx, cl, ns, rs.OwnerReferences)
			}
		}
		if owner.Kind == "Deployment" {
			var dep appsv1.Deployment
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &dep); err == nil {
				if app, ok := dep.Labels["argocd.argoproj.io/instance"]; ok {
					return fmt.Sprintf("argo:%s:%s", ns, app)
				}
			}
		}
	}
	return ""
}

// getDeploymentAnchor finds the stable Deployment name
func getDeploymentAnchor(ctx context.Context, cl client.Client, ns string) string {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return ""
	}

	// Walk up to find Deployment
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			var rs appsv1.ReplicaSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: owner.Name}, &rs); err == nil {
				for _, rsOwner := range rs.OwnerReferences {
					if rsOwner.Kind == "Deployment" {
						return fmt.Sprintf("deployment:%s:%s", ns, rsOwner.Name)
					}
				}
			}
		}
	}

	return ""
}

// getStatefulSetAnchor finds StatefulSet (inherently stable)
func getStatefulSetAnchor(ctx context.Context, cl client.Client, ns string) string {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return ""
	}

	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "StatefulSet" {
			return fmt.Sprintf("statefulset:%s:%s", ns, owner.Name)
		}
	}

	return ""
}

// buildSafeLabels creates Kubernetes-safe labels
func buildSafeLabels(ctx context.Context, cl client.Client, ns string) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/part-of":    APP_NAME,
		"app.kubernetes.io/managed-by": APP_NAME,
		"app.kubernetes.io/component":  "server",
	}

	// Add safe deployment context labels
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err == nil {
		// Add Helm release if available
		if release, ok := pod.Labels["app.kubernetes.io/instance"]; ok {
			labels["codespace.dev/release"] = sanitizeLabelValue(release)
		}

		// Add deployment method
		if _, ok := pod.Labels["helm.sh/chart"]; ok {
			labels["codespace.dev/method"] = "helm"
		} else if app, ok := pod.Labels["argocd.argoproj.io/instance"]; ok {
			labels["codespace.dev/method"] = "argo"
			labels["codespace.dev/argo-app"] = sanitizeLabelValue(app)
		} else {
			labels["codespace.dev/method"] = "kubectl"
		}
	}

	return labels
}

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

				// normalize anything unknown back to "namespace"
				switch kind {
				case "helm", "argo", "deployment", "statefulset", "namespace":
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
