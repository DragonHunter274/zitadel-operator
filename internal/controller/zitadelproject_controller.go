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

// ZitadelProjectReconciler reconciles a ZitadelProject object.
type ZitadelProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelprojects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelprojects/finalizers,verbs=update

func (r *ZitadelProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	proj := &zitadelv1alpha1.ZitadelProject{}
	if err := r.Get(ctx, req.NamespacedName, proj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !proj.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, proj)
	}

	if !controllerutil.ContainsFinalizer(proj, zitadelv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(proj, zitadelv1alpha1.Finalizer)
		if err := r.Update(ctx, proj); err != nil {
			return ctrl.Result{}, err
		}
	}

	zClient, _, err := getZitadelClient(ctx, r.Client, proj.Namespace, proj.Spec.ZitadelRef)
	if err != nil {
		log.Info("Zitadel not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	orgID, err := resolveOrgID(ctx, r.Client, proj.Namespace, proj.Spec.OrganizationRef)
	if err != nil {
		log.Info("Organization not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if proj.Status.ProjectID == "" {
		resp, err := zClient.CreateProject(ctx, orgID, zitadel.CreateProjectRequest{
			Name:                 proj.Spec.Name,
			ProjectRoleAssertion: proj.Spec.ProjectRoleAssertion,
			ProjectRoleCheck:     proj.Spec.ProjectRoleCheck,
			HasProjectCheck:      proj.Spec.HasProjectCheck,
		})
		if err != nil {
			setCondition(&proj.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
				metav1.ConditionFalse, "CreateFailed", err.Error())
			_ = r.Status().Update(ctx, proj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		proj.Status.ProjectID = resp.ID
	} else {
		if err := zClient.UpdateProject(ctx, orgID, proj.Status.ProjectID, zitadel.UpdateProjectRequest{
			Name:                 proj.Spec.Name,
			ProjectRoleAssertion: proj.Spec.ProjectRoleAssertion,
			ProjectRoleCheck:     proj.Spec.ProjectRoleCheck,
			HasProjectCheck:      proj.Spec.HasProjectCheck,
		}); err != nil {
			setCondition(&proj.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
				metav1.ConditionFalse, "UpdateFailed", err.Error())
			_ = r.Status().Update(ctx, proj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	proj.Status.Ready = true
	proj.Status.ObservedGeneration = proj.Generation
	setCondition(&proj.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
		metav1.ConditionTrue, "Synced", "Project synced successfully")
	if err := r.Status().Update(ctx, proj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZitadelProjectReconciler) handleDeletion(ctx context.Context, proj *zitadelv1alpha1.ZitadelProject) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if proj.Status.ProjectID != "" {
		zClient, _, err := getZitadelClient(ctx, r.Client, proj.Namespace, proj.Spec.ZitadelRef)
		if err != nil {
			log.Info("Zitadel not available for cleanup, removing finalizer")
		} else {
			orgID, _ := resolveOrgID(ctx, r.Client, proj.Namespace, proj.Spec.OrganizationRef)
			if err := zClient.DeleteProject(ctx, orgID, proj.Status.ProjectID); err != nil {
				if !zitadel.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}
	}

	controllerutil.RemoveFinalizer(proj, zitadelv1alpha1.Finalizer)
	return ctrl.Result{}, r.Update(ctx, proj)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZitadelProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ZitadelProject{}).
		Named("zitadelproject").
		Complete(r)
}
