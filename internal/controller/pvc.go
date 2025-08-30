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
package controller

import (
	"context"

	"encoding/json"

	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SessionReconciler) reconcilePVC(ctx context.Context, sess *codespacev1.Session, name, suffix string, spec *codespacev1.PVCSpec) error {
	if spec == nil {
		return nil
	}
	pvcName := name + "-" + suffix

	reqs := map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceStorage: resource.MustParse(spec.Size),
	}

	pvcSpec := corev1apply.PersistentVolumeClaimSpec().
		WithAccessModes(corev1.ReadWriteOnce).
		WithResources(corev1apply.VolumeResourceRequirements().WithRequests(reqs))

	if spec.StorageClassName != "" {
		pvcSpec = pvcSpec.WithStorageClassName(spec.StorageClassName)
	}

	cfg := corev1apply.PersistentVolumeClaim(pvcName, sess.Namespace).
		WithSpec(pvcSpec)

	owner := metav1apply.OwnerReference().
		WithAPIVersion(codespacev1.GroupVersion.String()).
		WithKind("Session").
		WithName(sess.Name).
		WithUID(sess.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
	cfg.WithOwnerReferences(owner)

	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return r.Patch(ctx,
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: sess.Namespace}},
		client.RawPatch(types.ApplyPatchType, data),
		client.FieldOwner(ssaFieldOwner),
	)
}
