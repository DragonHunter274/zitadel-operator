/*
Copyright 2026.

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/dragonhunter274/zitadel-operator/api/v1alpha1"
	"github.com/dragonhunter274/zitadel-operator/internal/zitadel"
)

// ZitadelRoleReconciler reconciles a ZitadelRole object.
type ZitadelRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelroles/finalizers,verbs=update

func (r *ZitadelRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	role := &zitadelv1alpha1.ZitadelRole{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !role.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, role)
	}

	if !controllerutil.ContainsFinalizer(role, zitadelv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(role, zitadelv1alpha1.Finalizer)
		if err := r.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
	}

	zClient, _, err := getZitadelClient(ctx, r.Client, role.Namespace, role.Spec.ZitadelRef)
	if err != nil {
		log.Info("Zitadel not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	projectID, orgID, err := resolveProjectID(ctx, r.Client, role.Namespace, role.Spec.ProjectRef)
	if err != nil {
		log.Info("Project not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// Get existing roles to compute diff
	existingRoles, err := zClient.ListProjectRoles(ctx, orgID, projectID)
	if err != nil {
		setCondition(&role.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
			metav1.ConditionFalse, "ListFailed", err.Error())
		_ = r.Status().Update(ctx, role)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	existingKeys := make(map[string]bool)
	for _, r := range existingRoles {
		existingKeys[r.Key] = true
	}

	// Add roles that don't exist yet
	var toAdd []zitadel.RoleEntry
	desiredKeys := make(map[string]bool)
	for _, r := range role.Spec.Roles {
		desiredKeys[r.Key] = true
		if !existingKeys[r.Key] {
			toAdd = append(toAdd, zitadel.RoleEntry{
				Key:         r.Key,
				DisplayName: r.DisplayName,
				Group:       r.Group,
			})
		}
	}

	if len(toAdd) > 0 {
		if err := zClient.BulkAddProjectRoles(ctx, orgID, projectID, toAdd); err != nil {
			setCondition(&role.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
				metav1.ConditionFalse, "AddFailed", err.Error())
			_ = r.Status().Update(ctx, role)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Remove roles that are no longer desired
	for _, existing := range existingRoles {
		if !desiredKeys[existing.Key] {
			if err := zClient.RemoveProjectRole(ctx, orgID, projectID, existing.Key); err != nil {
				if !zitadel.IsNotFound(err) {
					log.Error(err, "Failed to remove role", "key", existing.Key)
				}
			}
		}
	}

	// Update status
	syncedRoles := make([]string, 0, len(role.Spec.Roles))
	for _, r := range role.Spec.Roles {
		syncedRoles = append(syncedRoles, r.Key)
	}

	role.Status.SyncedRoles = syncedRoles
	role.Status.Ready = true
	role.Status.ObservedGeneration = role.Generation
	setCondition(&role.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
		metav1.ConditionTrue, "Synced", "Roles synced successfully")
	if err := r.Status().Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZitadelRoleReconciler) handleDeletion(ctx context.Context, role *zitadelv1alpha1.ZitadelRole) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if len(role.Status.SyncedRoles) > 0 {
		zClient, _, err := getZitadelClient(ctx, r.Client, role.Namespace, role.Spec.ZitadelRef)
		if err != nil {
			log.Info("Zitadel not available for cleanup, removing finalizer")
		} else {
			projectID, orgID, _ := resolveProjectID(ctx, r.Client, role.Namespace, role.Spec.ProjectRef)
			if projectID != "" {
				for _, key := range role.Status.SyncedRoles {
					if err := zClient.RemoveProjectRole(ctx, orgID, projectID, key); err != nil {
						if !zitadel.IsNotFound(err) {
							log.Error(err, "Failed to remove role during cleanup", "key", key)
						}
					}
				}
			}
		}
	}

	controllerutil.RemoveFinalizer(role, zitadelv1alpha1.Finalizer)
	return ctrl.Result{}, r.Update(ctx, role)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZitadelRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ZitadelRole{}).
		Named("zitadelrole").
		Complete(r)
}
