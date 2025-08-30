package server

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/codespace-operator/codespace-operator/internal/common"
)

const cmPrefixName = "codespace-server-id"

func ensureInstallationID(ctx context.Context, cl client.Client, cfg *ServerConfig) (common.AnchorMeta, string, error) {
	clusterUID := common.GetClusterUID(ctx, cl)
	anchor, rbacLimited, isKubernetes := common.GetSelfAnchorMeta(ctx, cl)
	if rbacLimited {
		logger.Warn("ServiceAccount likely unable to resolve fully", "rbacLimited", rbacLimited)
	}

	id := common.InstanceIDv1(clusterUID, anchor)
	logger.Info("Determined instance ID", "id", id, "anchor", anchor.String(), "clusterUID", clusterUID, "namespace", anchor.Namespace)

	if !isKubernetes {
		logger.Info("Running in non-Kubernetes environment, creating ConfigMap in k8s default context creation", "isKubernetes", isKubernetes)
	}
	// ConfigMap name derives from the (stable) anchor, like before
	cmName := fmt.Sprintf("%s-%s", cmPrefixName, common.K8sHexHash(anchor.String(), 10))
	key := client.ObjectKey{Namespace: anchor.Namespace, Name: cmName}

	// If CM exists, trust it (but if it has a legacy random id, we don't change it)
	var cm corev1.ConfigMap
	if err := cl.Get(ctx, key, &cm); err == nil {
		if v := cm.Data["id"]; v != "" {
			return anchor, v, nil
		}
		// backfill if present but missing 'id'
	} else if !apierrors.IsNotFound(err) {
		return anchor, "", err
	}

	// (Re)create index CM with deterministic content
	cm = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: anchor.Namespace,
			Labels:    common.BuildConfigMapSafeLabels(ctx, cl, anchor.Namespace, cfg.APP_NAME),
		},
		Data: map[string]string{
			"id":         id,
			"anchor":     anchor.String(),
			"clusterUID": clusterUID,
			"version":    "1",
		},
	}
	if err := cl.Create(ctx, &cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return anchor, "", err
	}
	return anchor, id, nil
}
