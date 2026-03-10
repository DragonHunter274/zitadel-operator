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

// ZitadelOrganizationReconciler reconciles a ZitadelOrganization object.
type ZitadelOrganizationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelorganizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelorganizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelorganizations/finalizers,verbs=update

func (r *ZitadelOrganizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	org := &zitadelv1alpha1.ZitadelOrganization{}
	if err := r.Get(ctx, req.NamespacedName, org); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !org.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, org)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(org, zitadelv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(org, zitadelv1alpha1.Finalizer)
		if err := r.Update(ctx, org); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Get Zitadel API client
	zClient, err := getZitadelClient(ctx, r.Client, org.Namespace, org.Spec.ZitadelRef)
	if err != nil {
		log.Info("Zitadel not ready, requeuing", "error", err.Error())
		setCondition(&org.Status.Conditions, zitadelv1alpha1.ConditionTypeZitadelAvailable,
			metav1.ConditionFalse, "ZitadelNotReady", err.Error())
		_ = r.Status().Update(ctx, org)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Create or update organization
	if org.Status.OrgID == "" {
		resp, err := zClient.CreateOrganization(ctx, org.Spec.Name)
		if err != nil {
			if zitadel.IsConflict(err) {
				log.Info("Organization already exists in Zitadel")
			} else {
				setCondition(&org.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
					metav1.ConditionFalse, "CreateFailed", err.Error())
				_ = r.Status().Update(ctx, org)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			org.Status.OrgID = resp.Org.ID
		}
	}

	org.Status.Ready = true
	org.Status.ObservedGeneration = org.Generation
	setCondition(&org.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
		metav1.ConditionTrue, "Synced", "Organization synced successfully")
	if err := r.Status().Update(ctx, org); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZitadelOrganizationReconciler) handleDeletion(ctx context.Context, org *zitadelv1alpha1.ZitadelOrganization) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if org.Status.OrgID != "" {
		zClient, err := getZitadelClient(ctx, r.Client, org.Namespace, org.Spec.ZitadelRef)
		if err != nil {
			log.Info("Zitadel not available for cleanup, removing finalizer")
		} else {
			if err := zClient.DeleteOrganization(ctx, org.Status.OrgID); err != nil {
				if !zitadel.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}
	}

	controllerutil.RemoveFinalizer(org, zitadelv1alpha1.Finalizer)
	return ctrl.Result{}, r.Update(ctx, org)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZitadelOrganizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ZitadelOrganization{}).
		Named("zitadelorganization").
		Complete(r)
}
