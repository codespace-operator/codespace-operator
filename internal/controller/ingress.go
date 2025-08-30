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

	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
	netv1apply "k8s.io/client-go/applyconfigurations/networking/v1"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SessionReconciler) reconcileIngress(ctx context.Context, sess *codespacev1.Session, name, svcName string) error {
	if sess.Spec.Networking == nil || sess.Spec.Networking.Host == "" {
		return nil
	}
	ns := sess.Namespace
	host := sess.Spec.Networking.Host

	pt := netv1.PathTypePrefix
	path := netv1apply.HTTPIngressPath().
		WithPath("/").
		WithPathType(pt).
		WithBackend(
			netv1apply.IngressBackend().
				WithService(
					netv1apply.IngressServiceBackend().
						WithName(svcName).
						WithPort(netv1apply.ServiceBackendPort().WithNumber(80)),
				),
		)

	ing := netv1apply.Ingress(name, ns).
		WithAnnotations(sess.Spec.Networking.Annotations).
		WithSpec(
			netv1apply.IngressSpec().
				WithRules(
					netv1apply.IngressRule().
						WithHost(host).
						WithHTTP(netv1apply.HTTPIngressRuleValue().WithPaths(path)),
				),
		)

	if tls := sess.Spec.Networking.TLSSecretName; tls != "" {
		ing.Spec.WithTLS(netv1apply.IngressTLS().
			WithHosts(host).
			WithSecretName(tls))
	}

	owner := metav1apply.OwnerReference().
		WithAPIVersion(codespacev1.GroupVersion.String()).
		WithKind("Session").
		WithName(sess.Name).
		WithUID(sess.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
	ing.WithOwnerReferences(owner)

	data, err := json.Marshal(ing)
	if err != nil {
		return err
	}
	return r.Patch(ctx,
		&netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		client.RawPatch(types.ApplyPatchType, data),
		client.FieldOwner(ssaFieldOwner),
	)
}
