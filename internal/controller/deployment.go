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
	"fmt"

	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"

	codespacev1alpha1 "github.com/codespace-operator/codespace-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *SessionReconciler) reconcileDeployment(ctx context.Context, sess *codespacev1alpha1.Session, name string, labels map[string]string) (*appsv1.Deployment, error) {
	ns := sess.Namespace
	port := r.determinePort(sess)
	vols, mounts := r.buildVolumesAndMounts(sess, name)

	// Convert mounts/volumes to apply configurations
	var acMounts []*corev1apply.VolumeMountApplyConfiguration
	for _, m := range mounts {
		acMounts = append(acMounts, corev1apply.VolumeMount().WithName(m.Name).WithMountPath(m.MountPath))
	}
	var acVols []*corev1apply.VolumeApplyConfiguration
	for _, v := range vols {
		acVols = append(acVols, corev1apply.Volume().
			WithName(v.Name).
			WithPersistentVolumeClaim(corev1apply.PersistentVolumeClaimVolumeSource().
				WithClaimName(v.VolumeSource.PersistentVolumeClaim.ClaimName)))
	}

	mainC := corev1apply.Container().
		WithName("ide").
		WithImage(sess.Spec.Profile.Image).
		WithArgs(sess.Spec.Profile.Cmd...).
		WithPorts(corev1apply.ContainerPort().WithContainerPort(port)).
		WithVolumeMounts(acMounts...)

	containers := []*corev1apply.ContainerApplyConfiguration{mainC}
	if sess.Spec.Auth.Mode == "oauth2proxy" && sess.Spec.Networking != nil && sess.Spec.Networking.Host != "" {
		sidecar := corev1apply.Container().
			WithName("oauth2-proxy").
			WithImage("quay.io/oauth2-proxy/oauth2-proxy:v7.6.0").
			WithArgs(
				"--provider=oidc",
				"--oidc-issuer-url=$(OIDC_ISSUER)",
				"--client-id=$(OIDC_CLIENT_ID)",
				"--client-secret=$(OIDC_CLIENT_SECRET)",
				fmt.Sprintf("--upstream=http://127.0.0.1:%d", port),
				"--http-address=0.0.0.0:4180",
				"--reverse-proxy=true",
				"--email-domain=*",
			).
			WithPorts(corev1apply.ContainerPort().WithContainerPort(4180))
		if sess.Spec.Auth.OIDC != nil {
			sidecar = sidecar.WithEnv(corev1apply.EnvVar().WithName("OIDC_ISSUER").WithValue(sess.Spec.Auth.OIDC.IssuerURL))
		}
		containers = append(containers, sidecar)
	}

	dep := appsv1apply.Deployment(name, ns).
		WithLabels(labels).
		WithSpec(
			appsv1apply.DeploymentSpec().
				WithSelector(metav1apply.LabelSelector().WithMatchLabels(labels)).
				WithReplicas(*sess.Spec.Replicas).
				WithTemplate(
					corev1apply.PodTemplateSpec().
						WithLabels(labels).
						WithSpec(
							corev1apply.PodSpec().
								WithServiceAccountName(name).
								WithVolumes(acVols...).
								WithContainers(containers...),
						),
				),
		)

	owner := metav1apply.OwnerReference().
		WithAPIVersion(codespacev1alpha1.GroupVersion.String()).
		WithKind("Session").
		WithName(sess.Name).
		WithUID(sess.UID).
		WithController(true).
		WithBlockOwnerDeletion(true)
	dep.WithOwnerReferences(owner)

	data, err := json.Marshal(dep)
	if err != nil {
		return nil, err
	}
	if err := r.Patch(ctx,
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}},
		client.RawPatch(types.ApplyPatchType, data),
		client.FieldOwner(ssaFieldOwner),
	); err != nil {
		return nil, err
	}

	// Return the latest Deployment (for status)
	out := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, out); err != nil {
		return nil, err
	}
	return out, nil
}