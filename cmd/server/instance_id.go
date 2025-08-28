package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ensureInstallationID creates/gets a per-instance ConfigMap and returns a stable id.
func ensureInstallationID(ctx context.Context, cl client.Client) (string, error) {
	ns := inClusterNamespace()

	// Try deployment-aware anchoring strategies in order of preference
	anchor := determineStableAnchor(ctx, cl, ns)

	// Generate k8s-safe ConfigMap name
	cmName := fmt.Sprintf("%s-%s", cmPrefixName, k8sHexHash(anchor, 10))

	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Namespace: ns, Name: cmName}

	// Try to get existing ConfigMap
	if err := cl.Get(ctx, key, cm); err == nil {
		if id := cm.Data["id"]; id != "" {
			return id, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}

	// Create new ConfigMap with stable ID
	id := randB64(18)
	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: ns,
			Labels:    buildSafeLabels(ctx, cl, ns),
		},
		Data: map[string]string{"id": id},
	}

	if err := cl.Create(ctx, cm); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Race condition - try to get the existing one
			if err := cl.Get(ctx, key, cm); err == nil && cm.Data["id"] != "" {
				return cm.Data["id"], nil
			}
		}
		return "", err
	}

	return id, nil
}

// determineStableAnchor creates a stable identity based on deployment context
func determineStableAnchor(ctx context.Context, cl client.Client, ns string) string {
	// Strategy 1: Helm release (most stable for Helm deployments)
	if helmAnchor := getHelmAnchor(ctx, cl, ns); helmAnchor != "" {
		return helmAnchor
	}

	// Strategy 2: ArgoCD application
	if argoAnchor := getArgoAnchor(ctx, cl, ns); argoAnchor != "" {
		return argoAnchor
	}

	// Strategy 3: Deployment name (stable across redeploys)
	if deployAnchor := getDeploymentAnchor(ctx, cl, ns); deployAnchor != "" {
		return deployAnchor
	}

	// Strategy 4: StatefulSet name
	if stsAnchor := getStatefulSetAnchor(ctx, cl, ns); stsAnchor != "" {
		return stsAnchor
	}

	// Fallback: namespace-based
	return fmt.Sprintf("namespace:%s:server", ns)
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

	// Check for Helm annotations/labels
	if release, ok := pod.Labels["app.kubernetes.io/instance"]; ok {
		if chart, ok := pod.Labels["helm.sh/chart"]; ok {
			return fmt.Sprintf("helm:%s:%s:%s", ns, release, chart)
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
		"app.kubernetes.io/part-of":    "codespace-operator",
		"app.kubernetes.io/component":  "server",
		"app.kubernetes.io/managed-by": "codespace-operator",
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
		} else if _, ok := pod.Labels["argocd.argoproj.io/instance"]; ok {
			labels["codespace.dev/method"] = "argo"
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
