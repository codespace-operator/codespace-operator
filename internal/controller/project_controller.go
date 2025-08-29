package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

// ProjectReconciler reconciles a Project object
type ProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Condition types for Project
const (
	TypeAvailable   = "Available"
	TypeProgressing = "Progressing"
	TypeDegraded    = "Degraded"
)

// Finalizer name for Project
const projectFinalizer = "codespace.codespace.dev/project-finalizer"

//+kubebuilder:rbac:groups=codespace.codespace.dev,resources=projects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=codespace.codespace.dev,resources=projects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=codespace.codespace.dev,resources=projects/finalizers,verbs=update
//+kubebuilder:rbac:groups=codespace.codespace.dev,resources=sessions,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Project instance
	project := &codespacev1.Project{}
	err := r.Get(ctx, req.NamespacedName, project)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Project resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Project")
		return ctrl.Result{}, err
	}

	// Check if the Project instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	if project.GetDeletionTimestamp() != nil {
		return r.finalizeProject(ctx, project)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(project, projectFinalizer) {
		log.Info("Adding Finalizer for Project")
		if ok := controllerutil.AddFinalizer(project, projectFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, project); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Update project status
	return r.updateProjectStatus(ctx, project)
}

// finalizeProject handles cleanup when Project is being deleted
func (r *ProjectReconciler) finalizeProject(ctx context.Context, project *codespacev1.Project) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(project, projectFinalizer) {
		log.Info("Performing Finalizer Operations for Project before delete CR")

		// TODO: Add cleanup logic here if needed
		// For example, you might want to:
		// 1. Update related Sessions to remove project labels
		// 2. Send notifications about project deletion
		// 3. Clean up external resources

		// Update status to indicate cleanup
		project.Status.Phase = "Terminating"
		project.Status.Reason = "Project is being deleted"
		if err := r.Status().Update(ctx, project); err != nil {
			log.Error(err, "Failed to update Project status during finalization")
			return ctrl.Result{}, err
		}

		log.Info("Removing Finalizer for Project after successfully performing the operations")
		if ok := controllerutil.RemoveFinalizer(project, projectFinalizer); !ok {
			log.Error(nil, "Failed to remove finalizer for Project")
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, project); err != nil {
			log.Error(err, "Failed to remove finalizer for Project")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// updateProjectStatus updates the Project status based on current state
func (r *ProjectReconciler) updateProjectStatus(ctx context.Context, project *codespacev1.Project) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Calculate member count
	memberCount := int32(len(project.Spec.Members))

	// Count active sessions associated with this project
	sessionCount, err := r.countProjectSessions(ctx, project)
	if err != nil {
		log.Error(err, "Failed to count project sessions")
		// Don't fail reconciliation for this - just log and continue
		sessionCount = 0
	}

	// Determine project phase
	phase := "Active"
	reason := "Project is active and ready"

	if project.Spec.Suspended {
		phase = "Suspended"
		reason = "Project has been suspended"
	}

	// Check if we need to update status
	needsUpdate := false
	if project.Status.Phase != phase ||
		project.Status.Reason != reason ||
		project.Status.MemberCount != memberCount ||
		project.Status.SessionCount != sessionCount {
		needsUpdate = true
	}

	if needsUpdate {
		// Update status fields
		project.Status.Phase = phase
		project.Status.Reason = reason
		project.Status.MemberCount = memberCount
		project.Status.SessionCount = sessionCount
		now := metav1.Now()
		project.Status.LastUpdated = &now
		project.Status.ObservedGeneration = project.Generation

		// Update conditions
		availableCondition := metav1.Condition{
			Type:   TypeAvailable,
			Status: metav1.ConditionTrue,
			Reason: "ProjectReady",
			Message: fmt.Sprintf("Project has %d members and %d active sessions",
				memberCount, sessionCount),
			LastTransitionTime: now,
		}

		if project.Spec.Suspended {
			availableCondition.Status = metav1.ConditionFalse
			availableCondition.Reason = "ProjectSuspended"
			availableCondition.Message = "Project is suspended"
		}

		// Update or add the condition
		r.updateCondition(&project.Status.Conditions, availableCondition)

		// Update the status
		if err := r.Status().Update(ctx, project); err != nil {
			log.Error(err, "Failed to update Project status")
			return ctrl.Result{}, err
		}

		log.Info("Updated Project status",
			"phase", phase,
			"members", memberCount,
			"sessions", sessionCount)
	}

	// Requeue after 30 seconds to refresh session counts
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// countProjectSessions counts active sessions associated with this project
func (r *ProjectReconciler) countProjectSessions(ctx context.Context, project *codespacev1.Project) (int32, error) {
	sessionList := &codespacev1.SessionList{}

	// List sessions with project label
	listOpts := []client.ListOption{
		client.MatchingLabels{
			"codespace.dev/project": project.Name,
		},
	}

	// If project specifies namespaces, search across them
	if len(project.Spec.Namespaces) > 0 {
		var count int32
		for _, ns := range project.Spec.Namespaces {
			nsSessionList := &codespacev1.SessionList{}
			nsOpts := append(listOpts, client.InNamespace(ns))
			if err := r.List(ctx, nsSessionList, nsOpts...); err != nil {
				return 0, err
			}
			count += int32(len(nsSessionList.Items))
		}
		return count, nil
	} else {
		// Search in project's own namespace
		listOpts = append(listOpts, client.InNamespace(project.Namespace))
		if err := r.List(ctx, sessionList, listOpts...); err != nil {
			return 0, err
		}
		return int32(len(sessionList.Items)), nil
	}
}

// updateCondition updates or adds a condition to the conditions slice
func (r *ProjectReconciler) updateCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	for i, condition := range *conditions {
		if condition.Type == newCondition.Type {
			// Update existing condition
			(*conditions)[i] = newCondition
			return
		}
	}
	// Add new condition
	*conditions = append(*conditions, newCondition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&codespacev1.Project{}).
		Complete(r)
}
