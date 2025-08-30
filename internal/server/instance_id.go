package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/codespace-operator/codespace-operator/internal/common"
)

const cmPrefixName = "codespace-server-id"

func ensureInstallationID(ctx context.Context, cl client.Client) (string, error) {
	ns := common.GetInClusterNamespace()
	clusterUID := common.GetClusterUID(ctx, cl)
	anchor := common.DetectAnchor(ctx, cl, ns)
	id := common.InstanceIDv1(clusterUID, anchor)
	logger.Info("Determined instance ID", "id", id, "anchor", anchor.String(), "clusterUID", clusterUID, "namespace", ns)

	// ConfigMap name derives from the (stable) anchor, like before
	cmName := fmt.Sprintf("%s-%s", cmPrefixName, common.K8sHexHash(anchor.String(), 10))
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
   Labels
   ============================ */

// buildSafeLabels creates Kubernetes-safe labels for
func buildConfigMapSafeLabels(ctx context.Context, cl client.Client, ns string) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/part-of":    "codespace-operator",
		"app.kubernetes.io/managed-by": APP_NAME,
		"app.kubernetes.io/component":  "server",
	}

	pod, err := common.GetCurrentPod(ctx, cl, ns)
	if err != nil || pod == nil {
		labels["codespace.dev/method"] = "kubectl"
		return labels
	}
	top, hasTop, _ := common.ResolveTopController(ctx, cl, ns, pod)

	if hasTop {
		if mm, ok := common.ManagerFromLabels(ns, top.Labels, top.Annotations); ok {
			if mm.Type == "argo" {
				labels["codespace.dev/method"] = "argo"
				labels["codespace.dev/argo-app"] = mm.Name
				return labels
			}
			if mm.Type == "helm" {
				labels["codespace.dev/method"] = "helm"
				labels["codespace.dev/release"] = mm.Name
				return labels
			}
		}
	}

	if mm, ok := common.ManagerFromLabels(ns, pod.Labels, pod.Annotations); ok {
		if mm.Type == "argo" {
			labels["codespace.dev/method"] = "argo"
			labels["codespace.dev/argo-app"] = mm.Name
			return labels
		}
		if mm.Type == "helm" {
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

// SanitizeLabelValue ensures label values are Kubernetes-compliant

// buildInstanceMetaIndex scans per-instance ConfigMaps and returns instanceID -> common.AnchorMeta.
func (h *handlers) buildInstanceMetaIndex(r *http.Request) map[string]common.AnchorMeta {
	out := map[string]common.AnchorMeta{}

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

		var meta common.AnchorMeta

		// 1) Prefer the stable anchor: "<kind>:<ns>:<name>"
		if a := cm.Data["anchor"]; a != "" {
			parts := strings.Split(a, ":")
			if len(parts) >= 3 {
				kind := parts[0]
				ns := parts[1]
				name := common.SanitizeLabelValue(parts[2])

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
				meta = common.AnchorMeta{Type: kind, Namespace: ns, Name: name}
			}
		}

		// 2) Fallback for very old CMs that predate 'anchor'
		if meta.Type == "" {
			method := cm.Labels["codespace.dev/method"] // helm|argo|kubectl
			switch method {
			case "helm":
				meta = common.AnchorMeta{Type: "helm", Namespace: cm.Namespace, Name: common.SanitizeLabelValue(cm.Labels["codespace.dev/release"])}
				if meta.Name == "" {
					meta.Name = "release"
				}
			case "argo":
				meta = common.AnchorMeta{Type: "argo", Namespace: cm.Namespace, Name: common.SanitizeLabelValue(cm.Labels["codespace.dev/argo-app"])}
				if meta.Name == "" {
					meta.Name = "app"
				}
			default:
				meta = common.AnchorMeta{Type: "unresolved", Namespace: cm.Namespace, Name: "unresolved"}
			}
		}

		out[id] = meta
	}

	return out
}
