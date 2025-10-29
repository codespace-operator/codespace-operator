package controller

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

func mergeSettings(user map[string]string, session map[string]string) map[string]string {
  out := map[string]string{}
  for k, v := range user { out[k] = v }
  for k, v := range session { out[k] = v }
  return out
}

func (r *SessionReconciler) reconcileSessionConfig(ctx context.Context, sess *codespacev1.Session, name string, prof *UserProfile) (string, error) {
  if sess.Spec.Git == nil || sess.Spec.Git.URL == "" {
    return "", nil // nothing to create
  }

  resolved := map[string]string{
    "repo.url": sess.Spec.Git.URL,
    "repo.ref": defaultString(sess.Spec.Git.Ref, "main"),
  }
  if sp := sess.Spec.Git.SubPath; sp != "" { resolved["repo.subPath"] = sp }

  // merge settings
  merged := mergeSettings(prof.Data, map[string]string(sess.Spec.Settings))
  if len(merged) > 0 {
    b, _ := json.Marshal(merged)
    resolved["ide.settings.json"] = string(b)
  }

  // name with content hash to make it immutable-ish
  h := sha1.Sum([]byte(resolved["repo.url"] + "|" + resolved["repo.ref"] + "|" + resolved["ide.settings.json"]))
  cmName := fmt.Sprintf("%s-session-%s", name, hex.EncodeToString(h[:8]))

  cm := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
      Name:      cmName,
      Namespace: sess.Namespace,
      Labels:    map[string]string{"app": name},
      OwnerReferences: []metav1.OwnerReference{{
        APIVersion: codespacev1.GroupVersion.String(),
        Kind:       "Session", Name: sess.Name, UID: sess.UID, Controller: ptr(true), BlockOwnerDeletion: ptr(true),
      }},
      // immutable CM prevents churn; re-name on changes
      Annotations: map[string]string{"codespace.dev/immutable": "true"},
    },
    Immutable: ptr(true),
    Data:      resolved,
  }

  // best-effort create; ignore AlreadyExists (same hash)
  if err := r.Client.Create(ctx, cm); client.IgnoreAlreadyExists(err) != nil {
    return "", err
  }
  return cm.Name, nil
}

func ptr[T any](v T) *T { return &v }
func defaultString(s, d string) string { if s != "" { return s }; return d }
