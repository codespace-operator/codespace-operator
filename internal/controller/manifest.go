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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

func (r *SessionReconciler) applyDefaults(sess *codespacev1.Session) {
	if sess.Spec.Profile.IDE == "" {
		sess.Spec.Profile.IDE = "jupyterlab"
	}
	if sess.Spec.Profile.Image == "" {
		switch sess.Spec.Profile.IDE {
		case "jupyterlab":
			sess.Spec.Profile.Image = "jupyter/minimal-notebook:latest"
			if len(sess.Spec.Profile.Cmd) == 0 {
				sess.Spec.Profile.Cmd = []string{"start-notebook.sh", "--NotebookApp.token="}
			}
		case "vscode":
			sess.Spec.Profile.Image = "codercom/code-server:latest"
			if len(sess.Spec.Profile.Cmd) == 0 {
				sess.Spec.Profile.Cmd = []string{"--bind-addr", "0.0.0.0:8080", "--auth", "none"}
			}
		}
	}
	// default replicas to 1 if unset
	if sess.Spec.Replicas == nil {
		one := int32(1)
		sess.Spec.Replicas = &one
	}
}

func (r *SessionReconciler) handleDelete(ctx context.Context, sess *codespacev1.Session) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(sess, sessionFinalizer)
	return ctrl.Result{}, r.Update(ctx, sess)
}

func (r *SessionReconciler) ensureFinalizer(ctx context.Context, sess *codespacev1.Session) error {
	if controllerutil.AddFinalizer(sess, sessionFinalizer) {
		return r.Update(ctx, sess)
	}
	return nil
}
func (r *SessionReconciler) updateStatus(ctx context.Context, sess *codespacev1.Session, dep *appsv1.Deployment) error {
	url := ""
	if sess.Spec.Networking != nil && sess.Spec.Networking.Host != "" {
		url = "https://" + sess.Spec.Networking.Host
	}
	sess.Status.URL = url
	if dep != nil && dep.Status.ReadyReplicas > 0 {
		sess.Status.Phase = "Ready"
	} else {
		sess.Status.Phase = "Pending"
	}
	return r.Status().Update(ctx, sess)
}
func (r *SessionReconciler) desiredNamesLabels(sess *codespacev1.Session) (string, map[string]string) {
	name := namePrefix + sess.Name
	return name, map[string]string{"app": name}
}

func (r *SessionReconciler) determinePort(sess *codespacev1.Session) int32 {
	if sess.Spec.Profile.IDE == "vscode" {
		return 8080
	}
	return 8888
}

func (r *SessionReconciler) buildVolumesAndMounts(sess *codespacev1.Session, name string) ([]corev1.Volume, []corev1.VolumeMount) {
	var vols []corev1.Volume
	var mounts []corev1.VolumeMount
	if sess.Spec.Home != nil {
		vols = append(vols, corev1.Volume{
			Name: "home", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name + "-home"},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "home", MountPath: sess.Spec.Home.MountPath})
	}
	if sess.Spec.Scratch != nil {
		vols = append(vols, corev1.Volume{
			Name: "scratch", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name + "-scratch"},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "scratch", MountPath: sess.Spec.Scratch.MountPath})
	}
	return vols, mounts
}

func (r *SessionReconciler) buildContainers(sess *codespacev1.Session, port int32, mainC corev1.Container) []corev1.Container {
	containers := []corev1.Container{mainC}
	if sess.Spec.Auth.Mode == "oauth2proxy" && sess.Spec.Networking != nil && sess.Spec.Networking.Host != "" {
		sidecar := corev1.Container{
			Name:  "oauth2-proxy",
			Image: "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0",
			Args: []string{
				"--provider=oidc",
				"--oidc-issuer-url=$(OIDC_ISSUER_URL)",
				"--client-id=$(OIDC_CLIENT_ID)",
				"--client-secret=$(OIDC_CLIENT_SECRET)",
				"--upstream=http://127.0.0.1:" + fmt.Sprint(port),
				"--http-address=0.0.0.0:4180",
				"--reverse-proxy=true",
				"--email-domain=*",
			},
			Ports: []corev1.ContainerPort{{ContainerPort: 4180}},
		}
		// Only set issuer env if present to avoid nil deref
		if sess.Spec.Auth.OIDC != nil {
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{Name: "OIDC_ISSUER_URL", Value: sess.Spec.Auth.OIDC.IssuerURL})
		}
		containers = append(containers, sidecar)
	}
	return containers
}
