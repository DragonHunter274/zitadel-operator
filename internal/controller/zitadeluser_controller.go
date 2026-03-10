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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/dragonhunter274/zitadel-operator/api/v1alpha1"
	"github.com/dragonhunter274/zitadel-operator/internal/zitadel"
)

// ZitadelUserReconciler reconciles a ZitadelUser object.
type ZitadelUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelusers/finalizers,verbs=update

func (r *ZitadelUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	user := &zitadelv1alpha1.ZitadelUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !user.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, user)
	}

	if !controllerutil.ContainsFinalizer(user, zitadelv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(user, zitadelv1alpha1.Finalizer)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	zClient, err := getZitadelClient(ctx, r.Client, user.Namespace, user.Spec.ZitadelRef)
	if err != nil {
		log.Info("Zitadel not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	orgID, err := resolveOrgID(ctx, r.Client, user.Namespace, user.Spec.OrganizationRef)
	if err != nil {
		log.Info("Organization not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if user.Status.UserID == "" {
		userID, err := r.createUser(ctx, zClient, orgID, user)
		if err != nil {
			if zitadel.IsConflict(err) {
				log.Info("User already exists in Zitadel")
			} else {
				setCondition(&user.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
					metav1.ConditionFalse, "CreateFailed", err.Error())
				_ = r.Status().Update(ctx, user)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			user.Status.UserID = userID
		}

		// Generate PAT for machine users if requested
		if user.Status.UserID != "" && user.Spec.Type == "Machine" &&
			user.Spec.Machine != nil && user.Spec.Machine.GeneratePAT &&
			user.Spec.CredentialSecretRef != "" {
			if err := r.createAndStorePAT(ctx, zClient, orgID, user); err != nil {
				log.Error(err, "Failed to create PAT")
			}
		}
	}

	user.Status.Ready = true
	user.Status.ObservedGeneration = user.Generation
	setCondition(&user.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
		metav1.ConditionTrue, "Synced", "User synced successfully")
	if err := r.Status().Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZitadelUserReconciler) createUser(ctx context.Context, zClient *zitadel.Client, orgID string, user *zitadelv1alpha1.ZitadelUser) (string, error) {
	switch user.Spec.Type {
	case "Human":
		if user.Spec.Human == nil {
			return "", fmt.Errorf("human config required for Human user type")
		}
		req := zitadel.CreateHumanUserRequest{
			UserName: user.Spec.UserName,
			Profile: zitadel.UserProfile{
				FirstName: user.Spec.Human.FirstName,
				LastName:  user.Spec.Human.LastName,
			},
			Email: zitadel.UserEmail{
				Email:           user.Spec.Human.Email,
				IsEmailVerified: true,
			},
		}
		if user.Spec.Human.PasswordRef != nil {
			password, err := readSecretValue(ctx, r.Client, user.Namespace, *user.Spec.Human.PasswordRef)
			if err != nil {
				return "", fmt.Errorf("read password: %w", err)
			}
			req.Password = &zitadel.UserPassword{Password: password}
		}
		resp, err := zClient.CreateHumanUser(ctx, orgID, req)
		if err != nil {
			return "", err
		}
		return resp.UserID, nil

	case "Machine":
		if user.Spec.Machine == nil {
			return "", fmt.Errorf("machine config required for Machine user type")
		}
		resp, err := zClient.CreateMachineUser(ctx, orgID, zitadel.CreateMachineUserRequest{
			UserName:    user.Spec.UserName,
			Name:        user.Spec.Machine.Name,
			Description: user.Spec.Machine.Description,
		})
		if err != nil {
			return "", err
		}
		return resp.UserID, nil

	default:
		return "", fmt.Errorf("unknown user type: %s", user.Spec.Type)
	}
}

func (r *ZitadelUserReconciler) createAndStorePAT(ctx context.Context, zClient *zitadel.Client, orgID string, user *zitadelv1alpha1.ZitadelUser) error {
	patResp, err := zClient.CreatePAT(ctx, orgID, user.Status.UserID)
	if err != nil {
		return fmt.Errorf("create PAT: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      user.Spec.CredentialSecretRef,
			Namespace: user.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetOwnerReference(user, secret, r.Scheme); err != nil {
			return err
		}
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			"token":  []byte(patResp.Token),
			"userID": []byte(user.Status.UserID),
		}
		return nil
	})
	return err
}

func (r *ZitadelUserReconciler) handleDeletion(ctx context.Context, user *zitadelv1alpha1.ZitadelUser) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if user.Status.UserID != "" {
		zClient, err := getZitadelClient(ctx, r.Client, user.Namespace, user.Spec.ZitadelRef)
		if err != nil {
			log.Info("Zitadel not available for cleanup, removing finalizer")
		} else {
			orgID, _ := resolveOrgID(ctx, r.Client, user.Namespace, user.Spec.OrganizationRef)
			if err := zClient.DeleteUser(ctx, orgID, user.Status.UserID); err != nil {
				if !zitadel.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}
	}

	controllerutil.RemoveFinalizer(user, zitadelv1alpha1.Finalizer)
	return ctrl.Result{}, r.Update(ctx, user)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZitadelUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ZitadelUser{}).
		Named("zitadeluser").
		Complete(r)
}
