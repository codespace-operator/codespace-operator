/*
Copyright 2025 Dennis Marcus Goh.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package controllers

import (
	"context"

	"encoding/json"

	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"

	codespacev1alpha1 "github.com/codespace-operator/codespace-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SessionReconciler) reconcileServiceAccount(ctx context.Context, sess *codespacev1alpha1.Session, name string) error {
	// Build apply configuration
	sa := corev1apply.ServiceAccount(name, sess.Namespace)
	// OwnerRef via apply config
	owner := metav1apply.OwnerReference().
		WithAPIVersion(codespacev1alpha1.GroupVersion.String()).
		WithKind("Session").
		WithName(sess.Name).
		WithUID(sess.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
	sa.WithOwnerReferences(owner)

	// Marshal and Apply
	data, err := json.Marshal(sa)
	if err != nil {
		return err
	}
	return r.Patch(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: sess.Namespace}},
		client.RawPatch(types.ApplyPatchType, data),
		client.FieldOwner(ssaFieldOwner),
	)
}