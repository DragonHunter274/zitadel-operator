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
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zitadelv1alpha1 "github.com/dragonhunter274/zitadel-operator/api/v1alpha1"
	"github.com/dragonhunter274/zitadel-operator/internal/zitadel"
)

// getZitadelClient resolves a Zitadel CR reference and returns an API client.
func getZitadelClient(ctx context.Context, c client.Client, namespace, zitadelRef string) (*zitadel.Client, *zitadelv1alpha1.Zitadel, error) {
	z := &zitadelv1alpha1.Zitadel{}
	if err := c.Get(ctx, types.NamespacedName{Name: zitadelRef, Namespace: namespace}, z); err != nil {
		return nil, nil, fmt.Errorf("get Zitadel %s: %w", zitadelRef, err)
	}

	if z.Status.Phase != zitadelv1alpha1.PhaseRunning {
		return nil, z, fmt.Errorf("Zitadel %s is not running (phase: %s)", zitadelRef, z.Status.Phase)
	}

	if !z.Status.ServiceAccountReady {
		return nil, z, fmt.Errorf("Zitadel %s service account not ready", zitadelRef)
	}

	if z.Spec.ServiceAccount == nil {
		return nil, z, fmt.Errorf("Zitadel %s has no serviceAccount configured", zitadelRef)
	}

	secretName := z.Spec.ServiceAccount.SecretName
	if secretName == "" {
		secretName = z.Name + "-operator-sa"
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return nil, z, fmt.Errorf("get operator SA secret %s: %w", secretName, err)
	}

	token := string(secret.Data["token"])
	if token == "" {
		return nil, z, fmt.Errorf("operator SA secret %s has no 'token' key", secretName)
	}

	baseURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", z.Name, namespace)
	return zitadel.NewClient(baseURL, token), z, nil
}

// resolveOrgID looks up a ZitadelOrganization by name and returns its OrgID.
func resolveOrgID(ctx context.Context, c client.Client, namespace, orgRef string) (string, error) {
	if orgRef == "" {
		return "", nil
	}
	org := &zitadelv1alpha1.ZitadelOrganization{}
	if err := c.Get(ctx, types.NamespacedName{Name: orgRef, Namespace: namespace}, org); err != nil {
		return "", fmt.Errorf("get ZitadelOrganization %s: %w", orgRef, err)
	}
	if org.Status.OrgID == "" {
		return "", fmt.Errorf("ZitadelOrganization %s has no orgID yet", orgRef)
	}
	return org.Status.OrgID, nil
}

// resolveProjectID looks up a ZitadelProject by name and returns its ProjectID and OrgID.
func resolveProjectID(ctx context.Context, c client.Client, namespace, projectRef string) (string, string, error) {
	proj := &zitadelv1alpha1.ZitadelProject{}
	if err := c.Get(ctx, types.NamespacedName{Name: projectRef, Namespace: namespace}, proj); err != nil {
		return "", "", fmt.Errorf("get ZitadelProject %s: %w", projectRef, err)
	}
	if proj.Status.ProjectID == "" {
		return "", "", fmt.Errorf("ZitadelProject %s has no projectID yet", projectRef)
	}
	orgID, _ := resolveOrgID(ctx, c, namespace, proj.Spec.OrganizationRef)
	return proj.Status.ProjectID, orgID, nil
}

// readSecretValue reads a single value from a Kubernetes Secret.
func readSecretValue(ctx context.Context, c client.Client, namespace string, ref zitadelv1alpha1.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("get secret %s: %w", ref.Name, err)
	}
	key := ref.Key
	if key == "" {
		for k := range secret.Data {
			key = k
			break
		}
	}
	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret %s has no key %s", ref.Name, key)
	}
	return string(val), nil
}

// hashData computes a SHA-256 hash of multiple byte slices.
func hashData(data ...[]byte) string {
	h := sha256.New()
	for _, d := range data {
		h.Write(d)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// setCondition is a helper to set a metav1.Condition on a condition list.
func setCondition(conditions *[]metav1.Condition, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}
