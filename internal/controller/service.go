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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SessionReconciler) reconcileService(ctx context.Context, sess *codespacev1.Session, name string, labels map[string]string) (*corev1.Service, error) {
	ns := sess.Namespace
	target := 4180
	if sess.Spec.Auth.Mode != "oauth2proxy" || sess.Spec.Networking == nil || sess.Spec.Networking.Host == "" {
		if sess.Spec.Profile.IDE == "vscode" {
			target = 8080
		} else {
			target = 8888
		}
	}

	svc := corev1apply.Service(name, ns).
		WithSpec(
			corev1apply.ServiceSpec().
				WithSelector(labels).
				WithPorts(
					corev1apply.ServicePort().
						WithName("http").
						WithPort(80).
						WithTargetPort(intstr.FromInt(target)),
				),
		)

	owner := metav1apply.OwnerReference().
		WithAPIVersion(codespacev1.GroupVersion.String()).
		WithKind("Session").
		WithName(sess.Name).
		WithUID(sess.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
	svc.WithOwnerReferences(owner)

	data, err := json.Marshal(svc)
	if err != nil {
		return nil, err
	}
	if err := r.Patch(ctx,
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		client.RawPatch(types.ApplyPatchType, data),
		client.FieldOwner(ssaFieldOwner),
	); err != nil {
		return nil, err
	}

	out := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, out); err != nil {
		return nil, err
	}
	return out, nil
}
