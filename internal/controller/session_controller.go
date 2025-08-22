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
	"time"

	"os"
	"sync"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	cfgOnce       sync.Once
	namePrefix    = "cs-"                // default prefix for child names
	ssaFieldOwner = "codespace-operator" // default SSA field manager
)

func loadControllerConfig() {
	cfgOnce.Do(func() {
		if v := os.Getenv("SESSION_NAME_PREFIX"); v != "" {
			namePrefix = v
		}
		if v := os.Getenv("FIELD_OWNER"); v != "" {
			ssaFieldOwner = v
		}
	})
}

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
	loadControllerConfig()
	logger := log.FromContext(ctx)

	var sess codespacev1.Session
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

func (r *SessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&codespacev1.Session{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&netv1.Ingress{}).
		Complete(r)
}
