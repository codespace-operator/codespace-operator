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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	codespacev1alpha1 "github.com/codespace-operator/codespace-operator/api/v1alpha1"
)

// RBAC markers (operator-sdk reads these)
//+kubebuilder:rbac:groups=codespace.codespace.dev,resources=sessions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=codespace.codespace.dev,resources=sessions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets;configmaps;services;persistentvolumeclaims;serviceaccounts,verbs=create;update;patch;get;list;watch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=create;update;patch;get;list;watch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=create;update;patch;get;list;watch;delete

const finalizer = "codespace.dev/finalizer"

type SessionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile creates/updates child resources for a Session.
func (r *SessionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var sess codespacev1alpha1.Session
	if err := r.Get(ctx, req.NamespacedName, &sess); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Runtime defaults (dev/test convenience; validation still recommended on CRD)
	r.applyDefaults(&sess)

	// Finalizer / deletion flow
	if !sess.DeletionTimestamp.IsZero() {
		return r.handleDelete(ctx, &sess)
	}
	if err := r.ensureFinalizer(ctx, &sess); err != nil {
		return ctrl.Result{}, err
	}

	name, labels := r.desiredNamesLabels(&sess)

	// Child resources
	if err := r.reconcileServiceAccount(ctx, &sess, name); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcilePVC(ctx, &sess, name, "home", sess.Spec.Home); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcilePVC(ctx, &sess, name, "scratch", sess.Spec.Scratch); err != nil {
		return ctrl.Result{}, err
	}

	dep, err := r.reconcileDeployment(ctx, &sess, name, labels)
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := r.reconcileService(ctx, &sess, name, labels)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileIngress(ctx, &sess, name, svc.Name); err != nil {
		return ctrl.Result{}, err
	}

	// Status
	if err := r.updateStatus(ctx, &sess, dep); err != nil && !errors.IsConflict(err) {
		logger.Error(err, "status update failed")
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *SessionReconciler) applyDefaults(sess *codespacev1alpha1.Session) {
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
}

func (r *SessionReconciler) handleDelete(ctx context.Context, sess *codespacev1alpha1.Session) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(sess, finalizer)
	return ctrl.Result{}, r.Update(ctx, sess)
}

func (r *SessionReconciler) ensureFinalizer(ctx context.Context, sess *codespacev1alpha1.Session) error {
	if controllerutil.AddFinalizer(sess, finalizer) {
		return r.Update(ctx, sess)
	}
	return nil
}

func (r *SessionReconciler) desiredNamesLabels(sess *codespacev1alpha1.Session) (string, map[string]string) {
	name := "cs-" + sess.Name
	return name, map[string]string{"app": name}
}
func (r *SessionReconciler) reconcileServiceAccount(ctx context.Context, sess *codespacev1alpha1.Session, name string) error {
	sa := &corev1.ServiceAccount{}
	sa.Name, sa.Namespace = name, sess.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		return controllerutil.SetControllerReference(sess, sa, r.Scheme)
	})
	return err
}

func (r *SessionReconciler) reconcilePVC(ctx context.Context, sess *codespacev1alpha1.Session, name, suffix string, spec *codespacev1alpha1.PVCSpec) error {
	if spec == nil {
		return nil
	}
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name, pvc.Namespace = name+"-"+suffix, sess.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: resource.MustParse(spec.Size),
		}
		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		if spec.StorageClassName != "" {
			sc := spec.StorageClassName
			pvc.Spec.StorageClassName = &sc
		}
		return controllerutil.SetControllerReference(sess, pvc, r.Scheme)
	})
	return err
}

func (r *SessionReconciler) reconcileDeployment(ctx context.Context, sess *codespacev1alpha1.Session, name string, labels map[string]string) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{}
	dep.Name, dep.Namespace = name, sess.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		port := r.determinePort(sess)

		vols, mounts := r.buildVolumesAndMounts(sess, name)
		mainC := corev1.Container{
			Name:         "ide",
			Image:        sess.Spec.Profile.Image,
			Args:         sess.Spec.Profile.Cmd,
			Ports:        []corev1.ContainerPort{{ContainerPort: port}},
			VolumeMounts: mounts,
		}
		containers := r.buildContainers(sess, port, mainC)

		dep.Labels = labels
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		dep.Spec.Replicas = pointer.Int32(1)
		dep.Spec.Template.Labels = labels
		dep.Spec.Template.Spec.ServiceAccountName = name // SA uses same base name
		dep.Spec.Template.Spec.Volumes = vols
		dep.Spec.Template.Spec.Containers = containers
		return controllerutil.SetControllerReference(sess, dep, r.Scheme)
	})
	return dep, err
}

func (r *SessionReconciler) reconcileService(ctx context.Context, sess *codespacev1alpha1.Session, name string, labels map[string]string) (*corev1.Service, error) {
	svc := &corev1.Service{}
	svc.Name, svc.Namespace = name, sess.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Spec.Selector = labels
		target := 4180
		if !(sess.Spec.Auth.Mode == "oauth2proxy" && sess.Spec.Networking != nil && sess.Spec.Networking.Host != "") {
			if sess.Spec.Profile.IDE == "vscode" {
				target = 8080
			} else {
				target = 8888
			}
		}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name: "http", Port: 80, TargetPort: intstr.FromInt(target),
		}}
		return controllerutil.SetControllerReference(sess, svc, r.Scheme)
	})
	return svc, err
}

func (r *SessionReconciler) reconcileIngress(ctx context.Context, sess *codespacev1alpha1.Session, name, svcName string) error {
	if sess.Spec.Networking == nil || sess.Spec.Networking.Host == "" {
		return nil
	}
	ing := &netv1.Ingress{}
	ing.Name, ing.Namespace = name, sess.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ing, func() error {
		if ing.Annotations == nil {
			ing.Annotations = map[string]string{}
		}
		for k, v := range sess.Spec.Networking.Annotations {
			ing.Annotations[k] = v
		}
		pt := netv1.PathTypePrefix
		ing.Spec.Rules = []netv1.IngressRule{{
			Host: sess.Spec.Networking.Host,
			IngressRuleValue: netv1.IngressRuleValue{HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{{
					Path: "/", PathType: &pt,
					Backend: netv1.IngressBackend{
						Service: &netv1.IngressServiceBackend{
							Name: svcName, Port: netv1.ServiceBackendPort{Number: 80},
						},
					},
				}},
			}},
		}}
		if tls := sess.Spec.Networking.TLSSecretName; tls != "" {
			ing.Spec.TLS = []netv1.IngressTLS{{Hosts: []string{sess.Spec.Networking.Host}, SecretName: tls}}
		} else {
			ing.Spec.TLS = nil
		}
		return controllerutil.SetControllerReference(sess, ing, r.Scheme)
	})
	return err
}

func (r *SessionReconciler) updateStatus(ctx context.Context, sess *codespacev1alpha1.Session, dep *appsv1.Deployment) error {
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
func (r *SessionReconciler) determinePort(sess *codespacev1alpha1.Session) int32 {
	if sess.Spec.Profile.IDE == "vscode" {
		return 8080
	}
	return 8888
}

func (r *SessionReconciler) buildVolumesAndMounts(sess *codespacev1alpha1.Session, name string) ([]corev1.Volume, []corev1.VolumeMount) {
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

func (r *SessionReconciler) buildContainers(sess *codespacev1alpha1.Session, port int32, mainC corev1.Container) []corev1.Container {
	containers := []corev1.Container{mainC}
	if sess.Spec.Auth.Mode == "oauth2proxy" && sess.Spec.Networking != nil && sess.Spec.Networking.Host != "" {
		sidecar := corev1.Container{
			Name:  "oauth2-proxy",
			Image: "quay.io/oauth2-proxy/oauth2-proxy:v7.6.0",
			Args: []string{
				"--provider=oidc",
				"--oidc-issuer-url=$(OIDC_ISSUER)",
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
			sidecar.Env = append(sidecar.Env, corev1.EnvVar{Name: "OIDC_ISSUER", Value: sess.Spec.Auth.OIDC.IssuerURL})
		}
		containers = append(containers, sidecar)
	}
	return containers
}

func (r *SessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&codespacev1alpha1.Session{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&netv1.Ingress{}).
		Complete(r)
}
