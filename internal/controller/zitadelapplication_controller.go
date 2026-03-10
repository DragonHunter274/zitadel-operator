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

// ZitadelApplicationReconciler reconciles a ZitadelApplication object.
type ZitadelApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadelapplications/finalizers,verbs=update

func (r *ZitadelApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	app := &zitadelv1alpha1.ZitadelApplication{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !app.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, app)
	}

	if !controllerutil.ContainsFinalizer(app, zitadelv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(app, zitadelv1alpha1.Finalizer)
		if err := r.Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
	}

	zClient, err := getZitadelClient(ctx, r.Client, app.Namespace, app.Spec.ZitadelRef)
	if err != nil {
		log.Info("Zitadel not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	projectID, orgID, err := resolveProjectID(ctx, r.Client, app.Namespace, app.Spec.ProjectRef)
	if err != nil {
		log.Info("Project not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	if app.Status.AppID == "" {
		appResp, err := r.createApp(ctx, zClient, orgID, projectID, app)
		if err != nil {
			setCondition(&app.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
				metav1.ConditionFalse, "CreateFailed", err.Error())
			_ = r.Status().Update(ctx, app)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		app.Status.AppID = appResp.AppID
		app.Status.ClientID = appResp.ClientID

		// Write client credentials to Secret if requested
		if app.Spec.ClientSecretRef != "" && appResp.ClientSecret != "" {
			if err := r.writeCredentialSecret(ctx, app, appResp); err != nil {
				log.Error(err, "Failed to write client credential secret")
			}
		}
	}

	app.Status.Ready = true
	app.Status.ObservedGeneration = app.Generation
	setCondition(&app.Status.Conditions, zitadelv1alpha1.ConditionTypeReady,
		metav1.ConditionTrue, "Synced", "Application synced successfully")
	if err := r.Status().Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZitadelApplicationReconciler) createApp(ctx context.Context, zClient *zitadel.Client, orgID, projectID string, app *zitadelv1alpha1.ZitadelApplication) (*zitadel.AppResponse, error) {
	switch app.Spec.Type {
	case "OIDC":
		if app.Spec.OIDC == nil {
			return nil, fmt.Errorf("OIDC config required for OIDC application type")
		}
		return zClient.CreateOIDCApp(ctx, orgID, projectID, zitadel.CreateOIDCAppRequest{
			Name:                   app.Spec.Name,
			RedirectUris:           app.Spec.OIDC.RedirectURIs,
			PostLogoutRedirectUris: app.Spec.OIDC.PostLogoutRedirectURIs,
			ResponseTypes:          mapResponseTypes(app.Spec.OIDC.ResponseTypes),
			GrantTypes:             mapGrantTypes(app.Spec.OIDC.GrantTypes),
			AppType:                mapAppType(app.Spec.OIDC.AppType),
			AuthMethodType:         mapOIDCAuthMethod(app.Spec.OIDC.AuthMethodType),
			AccessTokenType:        mapAccessTokenType(app.Spec.OIDC.AccessTokenType),
			DevMode:                app.Spec.OIDC.DevMode,
		})
	case "API":
		if app.Spec.API == nil {
			return nil, fmt.Errorf("API config required for API application type")
		}
		return zClient.CreateAPIApp(ctx, orgID, projectID, zitadel.CreateAPIAppRequest{
			Name:           app.Spec.Name,
			AuthMethodType: mapAPIAuthMethod(app.Spec.API.AuthMethodType),
		})
	case "SAML":
		if app.Spec.SAML == nil {
			return nil, fmt.Errorf("SAML config required for SAML application type")
		}
		return zClient.CreateSAMLApp(ctx, orgID, projectID, zitadel.CreateSAMLAppRequest{
			Name:        app.Spec.Name,
			MetadataXml: app.Spec.SAML.MetadataXML,
			MetadataUrl: app.Spec.SAML.MetadataURL,
		})
	default:
		return nil, fmt.Errorf("unknown application type: %s", app.Spec.Type)
	}
}

func (r *ZitadelApplicationReconciler) writeCredentialSecret(ctx context.Context, app *zitadelv1alpha1.ZitadelApplication, appResp *zitadel.AppResponse) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Spec.ClientSecretRef,
			Namespace: app.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetOwnerReference(app, secret, r.Scheme); err != nil {
			return err
		}
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			"clientId":     []byte(appResp.ClientID),
			"clientSecret": []byte(appResp.ClientSecret),
		}
		return nil
	})
	return err
}

func (r *ZitadelApplicationReconciler) handleDeletion(ctx context.Context, app *zitadelv1alpha1.ZitadelApplication) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if app.Status.AppID != "" {
		zClient, err := getZitadelClient(ctx, r.Client, app.Namespace, app.Spec.ZitadelRef)
		if err != nil {
			log.Info("Zitadel not available for cleanup, removing finalizer")
		} else {
			projectID, orgID, _ := resolveProjectID(ctx, r.Client, app.Namespace, app.Spec.ProjectRef)
			if projectID != "" {
				if err := zClient.DeleteApp(ctx, orgID, projectID, app.Status.AppID); err != nil {
					if !zitadel.IsNotFound(err) {
						return ctrl.Result{}, err
					}
				}
			}
		}
	}

	controllerutil.RemoveFinalizer(app, zitadelv1alpha1.Finalizer)
	return ctrl.Result{}, r.Update(ctx, app)
}

// Mapping helpers for Zitadel API enum values.

func mapResponseTypes(types []string) []string {
	m := map[string]string{
		"CODE":           "OIDC_RESPONSE_TYPE_CODE",
		"ID_TOKEN":       "OIDC_RESPONSE_TYPE_ID_TOKEN",
		"ID_TOKEN_TOKEN": "OIDC_RESPONSE_TYPE_ID_TOKEN_TOKEN",
	}
	return mapEnums(types, m)
}

func mapGrantTypes(types []string) []string {
	m := map[string]string{
		"AUTHORIZATION_CODE": "OIDC_GRANT_TYPE_AUTHORIZATION_CODE",
		"IMPLICIT":           "OIDC_GRANT_TYPE_IMPLICIT",
		"REFRESH_TOKEN":      "OIDC_GRANT_TYPE_REFRESH_TOKEN",
		"DEVICE_CODE":        "OIDC_GRANT_TYPE_DEVICE_CODE",
	}
	return mapEnums(types, m)
}

func mapAppType(t string) string {
	m := map[string]string{
		"WEB":        "OIDC_APP_TYPE_WEB",
		"USER_AGENT": "OIDC_APP_TYPE_USER_AGENT",
		"NATIVE":     "OIDC_APP_TYPE_NATIVE",
	}
	if v, ok := m[t]; ok {
		return v
	}
	return "OIDC_APP_TYPE_WEB"
}

func mapOIDCAuthMethod(t string) string {
	m := map[string]string{
		"BASIC":           "OIDC_AUTH_METHOD_TYPE_BASIC",
		"POST":            "OIDC_AUTH_METHOD_TYPE_POST",
		"NONE":            "OIDC_AUTH_METHOD_TYPE_NONE",
		"PRIVATE_KEY_JWT": "OIDC_AUTH_METHOD_TYPE_PRIVATE_KEY_JWT",
	}
	if v, ok := m[t]; ok {
		return v
	}
	return "OIDC_AUTH_METHOD_TYPE_BASIC"
}

func mapAPIAuthMethod(t string) string {
	m := map[string]string{
		"BASIC":           "API_AUTH_METHOD_TYPE_BASIC",
		"PRIVATE_KEY_JWT": "API_AUTH_METHOD_TYPE_PRIVATE_KEY_JWT",
	}
	if v, ok := m[t]; ok {
		return v
	}
	return "API_AUTH_METHOD_TYPE_BASIC"
}

func mapAccessTokenType(t string) string {
	m := map[string]string{
		"BEARER": "OIDC_TOKEN_TYPE_BEARER",
		"JWT":    "OIDC_TOKEN_TYPE_JWT",
	}
	if v, ok := m[t]; ok {
		return v
	}
	return "OIDC_TOKEN_TYPE_BEARER"
}

func mapEnums(input []string, m map[string]string) []string {
	result := make([]string, 0, len(input))
	for _, v := range input {
		if mapped, ok := m[v]; ok {
			result = append(result, mapped)
		} else {
			result = append(result, v)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZitadelApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.ZitadelApplication{}).
		Named("zitadelapplication").
		Complete(r)
}
