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
	"encoding/json"
	"fmt"
	"maps"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	zitadelv1alpha1 "github.com/dragonhunter274/zitadel-operator/api/v1alpha1"
	"github.com/dragonhunter274/zitadel-operator/internal/zitadel"
)

const zitadelImage = "ghcr.io/zitadel/zitadel"

// ZitadelReconciler reconciles a Zitadel object.
type ZitadelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=zitadel.dragonhunter274.github.com,resources=zitadels/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *ZitadelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	z := &zitadelv1alpha1.Zitadel{}
	if err := r.Get(ctx, req.NamespacedName, z); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Step 1: Reconcile ConfigMap
	configHash, err := r.reconcileConfigMap(ctx, z)
	if err != nil {
		return r.setDegraded(ctx, z, "ConfigMap", err)
	}

	// Step 2: Reconcile database config secret
	dbHash, err := r.reconcileDBSecret(ctx, z)
	if err != nil {
		return r.setDegraded(ctx, z, "DBSecret", err)
	}

	// Step 3: Reconcile init Job
	initDone, err := r.reconcileInitJob(ctx, z)
	if err != nil {
		return r.setDegraded(ctx, z, "InitJob", err)
	}
	if !initDone {
		z.Status.Phase = zitadelv1alpha1.PhaseInitializing
		setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeInitialized,
			metav1.ConditionFalse, "InitJobRunning", "Init job is still running")
		if err := r.Status().Update(ctx, z); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeInitialized,
		metav1.ConditionTrue, "InitJobSucceeded", "Database initialized successfully")

	// Step 4: Reconcile setup Job
	setupDone, err := r.reconcileSetupJob(ctx, z)
	if err != nil {
		return r.setDegraded(ctx, z, "SetupJob", err)
	}
	if !setupDone {
		z.Status.Phase = zitadelv1alpha1.PhaseInitializing
		setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeSetupCompleted,
			metav1.ConditionFalse, "SetupJobRunning", "Setup job is still running")
		if err := r.Status().Update(ctx, z); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeSetupCompleted,
		metav1.ConditionTrue, "SetupJobSucceeded", "Instance setup completed successfully")

	// Step 5: Reconcile Deployment
	if err := r.reconcileDeployment(ctx, z, configHash, dbHash); err != nil {
		return r.setDegraded(ctx, z, "Deployment", err)
	}

	// Step 6: Reconcile Service
	if err := r.reconcileService(ctx, z); err != nil {
		return r.setDegraded(ctx, z, "Service", err)
	}

	// Step 7: Check deployment readiness
	ready, replicas, err := r.getDeploymentStatus(ctx, z)
	if err != nil {
		return r.setDegraded(ctx, z, "DeploymentStatus", err)
	}
	z.Status.Ready = replicas
	if !ready {
		z.Status.Phase = zitadelv1alpha1.PhaseInitializing
		setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeAvailable,
			metav1.ConditionFalse, "DeploymentNotReady", "Waiting for deployment to become ready")
		if err := r.Status().Update(ctx, z); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeAvailable,
		metav1.ConditionTrue, "DeploymentReady", "Zitadel deployment is ready")

	// Step 8: Reconcile Ingress (optional)
	if z.Spec.Ingress != nil && z.Spec.Ingress.Enabled {
		if err := r.reconcileIngress(ctx, z); err != nil {
			return r.setDegraded(ctx, z, "Ingress", err)
		}
	}

	// Step 9: Bootstrap operator service account
	if z.Spec.ServiceAccount != nil {
		if err := r.reconcileServiceAccount(ctx, z); err != nil {
			log.Error(err, "Failed to bootstrap operator service account, will retry")
			setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeServiceAccount,
				metav1.ConditionFalse, "ServiceAccountError", err.Error())
			z.Status.Phase = zitadelv1alpha1.PhaseRunning
			z.Status.Version = z.Spec.Version
			z.Status.ObservedGeneration = z.Generation
			if statusErr := r.Status().Update(ctx, z); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		z.Status.ServiceAccountReady = true
		setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeServiceAccount,
			metav1.ConditionTrue, "ServiceAccountReady", "Operator service account is ready")
	}

	// All good
	z.Status.Phase = zitadelv1alpha1.PhaseRunning
	z.Status.Version = z.Spec.Version
	z.Status.ObservedGeneration = z.Generation
	if err := r.Status().Update(ctx, z); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZitadelReconciler) setDegraded(ctx context.Context, z *zitadelv1alpha1.Zitadel, component string, err error) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "Reconciliation failed", "component", component)

	z.Status.Phase = zitadelv1alpha1.PhaseError
	setCondition(&z.Status.Conditions, zitadelv1alpha1.ConditionTypeDegraded,
		metav1.ConditionTrue, component+"Error", err.Error())
	if statusErr := r.Status().Update(ctx, z); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// reconcileConfigMap creates or updates the Zitadel config ConfigMap.
func (r *ZitadelReconciler) reconcileConfigMap(ctx context.Context, z *zitadelv1alpha1.Zitadel) (string, error) {
	configYAML := r.buildConfigYAML(z)
	cmName := z.Name + "-config"

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: z.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := controllerutil.SetControllerReference(z, cm, r.Scheme); err != nil {
			return err
		}
		cm.Data = map[string]string{
			"zitadel-config.yaml": string(configYAML),
		}
		if z.Spec.FirstInstance != nil {
			cm.Data["steps.yaml"] = r.buildStepsYAML(z)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("create/update configmap %s: %w", cmName, err)
	}

	log := logf.FromContext(ctx)
	if op != controllerutil.OperationResultNone {
		log.Info("ConfigMap reconciled", "operation", op)
	}

	return hashData(configYAML), nil
}

// buildConfigYAML builds the Zitadel YAML configuration merging spec fields with freeform config.
func (r *ZitadelReconciler) buildConfigYAML(z *zitadelv1alpha1.Zitadel) []byte {
	config := map[string]any{
		"Port":           8080,
		"ExternalDomain": z.Spec.Network.ExternalDomain,
		"ExternalPort":   z.Spec.Network.ExternalPort,
		"ExternalSecure": z.Spec.Network.ExternalSecure == nil || *z.Spec.Network.ExternalSecure,
		"Database": map[string]any{
			"Postgres": map[string]any{
				"Host":            z.Spec.Database.Host,
				"Port":            z.Spec.Database.Port,
				"Database":        z.Spec.Database.Database,
				"MaxOpenConns":    10,
				"MaxIdleConns":    5,
				"MaxConnLifetime": "30m",
				"User": map[string]any{
					"SSL": map[string]any{
						"Mode": z.Spec.Database.SSLMode,
					},
				},
				"Admin": map[string]any{
					"SSL": map[string]any{
						"Mode": z.Spec.Database.SSLMode,
					},
				},
			},
		},
	}

	if z.Spec.Network.TLS != nil && z.Spec.Network.TLS.Enabled {
		config["TLS"] = map[string]any{
			"Enabled":  true,
			"KeyPath":  "/etc/zitadel/tls/tls.key",
			"CertPath": "/etc/zitadel/tls/tls.crt",
		}
	}

	// Merge freeform configuration
	if z.Spec.Configuration != nil && z.Spec.Configuration.Raw != nil {
		var userConfig map[string]any
		if err := json.Unmarshal(z.Spec.Configuration.Raw, &userConfig); err == nil {
			maps.Copy(config, userConfig)
		}
	}

	data, _ := json.Marshal(config)
	return data
}

// buildStepsYAML builds the steps.yaml for zitadel setup --steps.
func (r *ZitadelReconciler) buildStepsYAML(z *zitadelv1alpha1.Zitadel) string {
	if z.Spec.FirstInstance == nil {
		return "{}"
	}
	fi := z.Spec.FirstInstance
	steps := map[string]any{
		"FirstInstance": map[string]any{
			"Org": map[string]any{
				"Name": fi.Org.Name,
				"Human": map[string]any{
					"UserName":  fi.Org.Human.UserName,
					"FirstName": fi.Org.Human.FirstName,
					"LastName":  fi.Org.Human.LastName,
					"Email": map[string]any{
						"Address":    fi.Org.Human.Email,
						"IsVerified": true,
					},
					"PasswordChangeRequired": false,
				},
			},
		},
	}
	data, _ := json.Marshal(steps)
	return string(data)
}

// reconcileDBSecret creates or updates the database credential secret as a YAML overlay.
func (r *ZitadelReconciler) reconcileDBSecret(ctx context.Context, z *zitadelv1alpha1.Zitadel) (string, error) {
	adminUser, err := readSecretValue(ctx, r.Client, z.Namespace, z.Spec.Database.AdminCredentials)
	if err != nil {
		return "", fmt.Errorf("read admin credentials: %w", err)
	}

	appUser, err := readSecretValue(ctx, r.Client, z.Namespace, z.Spec.Database.UserCredentials)
	if err != nil {
		return "", fmt.Errorf("read user credentials: %w", err)
	}

	// Build the database YAML overlay with actual credentials
	dbConfig := map[string]any{
		"Database": map[string]any{
			"Postgres": map[string]any{
				"User": map[string]any{
					"Password": appUser,
				},
				"Admin": map[string]any{
					"Password": adminUser,
				},
			},
		},
	}
	dbConfigData, _ := json.Marshal(dbConfig)

	secretName := z.Name + "-db-config"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: z.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(z, secret, r.Scheme); err != nil {
			return err
		}
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			"db-config.yaml": dbConfigData,
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("create/update db secret %s: %w", secretName, err)
	}

	return hashData(dbConfigData), nil
}

// reconcileInitJob creates the init Job if it doesn't exist, and checks if it's done.
func (r *ZitadelReconciler) reconcileInitJob(ctx context.Context, z *zitadelv1alpha1.Zitadel) (bool, error) {
	jobName := z.Name + "-init"
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: z.Namespace}, job)

	if errors.IsNotFound(err) {
		job = r.buildJob(z, jobName, []string{"init"})
		if err := controllerutil.SetControllerReference(z, job, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, job); err != nil {
			return false, fmt.Errorf("create init job: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get init job: %w", err)
	}

	return isJobComplete(job)
}

// reconcileSetupJob creates or recreates the setup Job.
func (r *ZitadelReconciler) reconcileSetupJob(ctx context.Context, z *zitadelv1alpha1.Zitadel) (bool, error) {
	jobName := z.Name + "-setup"
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: z.Namespace}, job)

	if errors.IsNotFound(err) {
		args := []string{"setup", "--init-projections=true"}
		if z.Spec.FirstInstance != nil {
			args = append(args, "--steps", "/config/steps.yaml")
		}
		job = r.buildJob(z, jobName, args)

		// Add admin password env if firstInstance is configured
		if z.Spec.FirstInstance != nil {
			job.Spec.Template.Spec.Containers[0].Env = append(
				job.Spec.Template.Spec.Containers[0].Env,
				corev1.EnvVar{
					Name: "ZITADEL_FIRSTINSTANCE_ORG_HUMAN_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: z.Spec.FirstInstance.Org.Human.PasswordRef.Name,
							},
							Key: secretKey(z.Spec.FirstInstance.Org.Human.PasswordRef),
						},
					},
				},
			)
		}

		if err := controllerutil.SetControllerReference(z, job, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, job); err != nil {
			return false, fmt.Errorf("create setup job: %w", err)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get setup job: %w", err)
	}

	// If version changed, delete old job and let it be recreated
	if z.Status.Version != "" && z.Status.Version != z.Spec.Version {
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			if !errors.IsNotFound(err) {
				return false, fmt.Errorf("delete old setup job: %w", err)
			}
		}
		return false, nil
	}

	return isJobComplete(job)
}

// buildJob creates a Job spec for zitadel init or setup commands.
func (r *ZitadelReconciler) buildJob(z *zitadelv1alpha1.Zitadel, name string, command []string) *batchv1.Job {
	args := append(command,
		"--config", "/config/zitadel-config.yaml",
		"--config", "/secrets/db-config.yaml",
	)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: z.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "zitadel",
				"app.kubernetes.io/instance":   z.Name,
				"app.kubernetes.io/managed-by": "zitadel-operator",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "zitadel",
							Image:   fmt.Sprintf("%s:%s", zitadelImage, z.Spec.Version),
							Command: []string{"zitadel"},
							Args:    args,
							Env: []corev1.EnvVar{
								{
									Name: "ZITADEL_MASTERKEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: z.Spec.Masterkey.Name,
											},
											Key: secretKey(z.Spec.Masterkey),
										},
									},
								},
							},
							VolumeMounts: r.volumeMounts(z),
						},
					},
					Volumes: r.volumes(z),
				},
			},
		},
	}
}

// reconcileDeployment creates or updates the main Zitadel Deployment.
func (r *ZitadelReconciler) reconcileDeployment(ctx context.Context, z *zitadelv1alpha1.Zitadel, configHash, dbHash string) error {
	replicas := int32(1)
	if z.Spec.Replicas != nil {
		replicas = *z.Spec.Replicas
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      z.Name,
			Namespace: z.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(z, dep, r.Scheme); err != nil {
			return err
		}

		labels := map[string]string{
			"app.kubernetes.io/name":       "zitadel",
			"app.kubernetes.io/instance":   z.Name,
			"app.kubernetes.io/managed-by": "zitadel-operator",
		}

		dep.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"zitadel.dragonhunter274.github.com/config-hash": configHash + "-" + dbHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "zitadel",
							Image:   fmt.Sprintf("%s:%s", zitadelImage, z.Spec.Version),
							Command: []string{"zitadel"},
							Args: []string{
								"start",
								"--config", "/config/zitadel-config.yaml",
								"--config", "/secrets/db-config.yaml",
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "grpc",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "ZITADEL_MASTERKEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: z.Spec.Masterkey.Name,
											},
											Key: secretKey(z.Spec.Masterkey),
										},
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/debug/ready",
										Port:   intstr.FromInt32(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/debug/healthz",
										Port:   intstr.FromInt32(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       15,
								TimeoutSeconds:      5,
							},
							VolumeMounts: r.volumeMounts(z),
						},
					},
					Volumes: r.volumes(z),
				},
			},
		}

		if z.Spec.Resources != nil {
			dep.Spec.Template.Spec.Containers[0].Resources = *z.Spec.Resources
		}

		return nil
	})
	return err
}

// reconcileService creates or updates the Zitadel Service.
func (r *ZitadelReconciler) reconcileService(ctx context.Context, z *zitadelv1alpha1.Zitadel) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      z.Name,
			Namespace: z.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(z, svc, r.Scheme); err != nil {
			return err
		}
		appProtocol := "h2c"
		svc.Spec = corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app.kubernetes.io/name":     "zitadel",
				"app.kubernetes.io/instance": z.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:        "grpc",
					Port:        8080,
					TargetPort:  intstr.FromInt32(8080),
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: &appProtocol,
				},
			},
		}
		return nil
	})
	return err
}

// reconcileIngress creates or updates the Ingress.
func (r *ZitadelReconciler) reconcileIngress(ctx context.Context, z *zitadelv1alpha1.Zitadel) error {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      z.Name,
			Namespace: z.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ing, func() error {
		if err := controllerutil.SetControllerReference(z, ing, r.Scheme); err != nil {
			return err
		}

		pathType := networkingv1.PathTypePrefix
		ing.Annotations = z.Spec.Ingress.Annotations
		ing.Spec = networkingv1.IngressSpec{
			IngressClassName: z.Spec.Ingress.IngressClassName,
			TLS:              z.Spec.Ingress.TLS,
			Rules: []networkingv1.IngressRule{
				{
					Host: z.Spec.Network.ExternalDomain,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: z.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		return nil
	})
	return err
}

// reconcileServiceAccount bootstraps the operator service account in Zitadel.
func (r *ZitadelReconciler) reconcileServiceAccount(ctx context.Context, z *zitadelv1alpha1.Zitadel) error {
	sa := z.Spec.ServiceAccount
	secretName := sa.SecretName
	if secretName == "" {
		secretName = z.Name + "-operator-sa"
	}

	// Check if the secret already exists with a token
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: z.Namespace}, existingSecret)
	if err == nil && len(existingSecret.Data["token"]) > 0 {
		return nil // Already bootstrapped
	}

	// Need to bootstrap: authenticate with admin user and create machine user + PAT
	// First, get admin credentials to authenticate
	if z.Spec.FirstInstance == nil {
		return fmt.Errorf("cannot bootstrap service account without firstInstance configuration")
	}

	adminPassword, err := readSecretValue(ctx, r.Client, z.Namespace, z.Spec.FirstInstance.Org.Human.PasswordRef)
	if err != nil {
		return fmt.Errorf("read admin password: %w", err)
	}

	baseURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", z.Name, z.Namespace)

	// Get admin token via password grant
	token, err := zitadel.GetPasswordToken(ctx, baseURL,
		z.Spec.FirstInstance.Org.Human.UserName,
		adminPassword)
	if err != nil {
		return fmt.Errorf("get admin token: %w", err)
	}

	zClient := zitadel.NewClient(baseURL, token)

	// Create machine user
	userName := sa.UserName
	if userName == "" {
		userName = "zitadel-operator"
	}

	userResp, err := zClient.CreateMachineUser(ctx, "", zitadel.CreateMachineUserRequest{
		UserName:    userName,
		Name:        "Zitadel Operator Service Account",
		Description: "Managed by zitadel-operator",
	})
	if err != nil {
		if !zitadel.IsConflict(err) {
			return fmt.Errorf("create machine user: %w", err)
		}
		// User already exists - we need to find their ID
		// For now, the PAT secret should have been created during a previous attempt
		return fmt.Errorf("machine user already exists but PAT secret not found; manual intervention required")
	}

	// Create PAT for the machine user
	patResp, err := zClient.CreatePAT(ctx, "", userResp.UserID)
	if err != nil {
		return fmt.Errorf("create PAT: %w", err)
	}

	// Write PAT to Kubernetes Secret
	patSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: z.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, patSecret, func() error {
		if err := controllerutil.SetControllerReference(z, patSecret, r.Scheme); err != nil {
			return err
		}
		patSecret.Type = corev1.SecretTypeOpaque
		patSecret.Data = map[string][]byte{
			"token":  []byte(patResp.Token),
			"userID": []byte(userResp.UserID),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create PAT secret: %w", err)
	}

	return nil
}

// getDeploymentStatus returns whether the deployment is ready and the number of ready replicas.
func (r *ZitadelReconciler) getDeploymentStatus(ctx context.Context, z *zitadelv1alpha1.Zitadel) (bool, int32, error) {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: z.Name, Namespace: z.Namespace}, dep)
	if err != nil {
		return false, 0, err
	}
	ready := dep.Status.ReadyReplicas >= 1
	return ready, dep.Status.ReadyReplicas, nil
}

// volumes returns the volume list for Zitadel containers.
func (r *ZitadelReconciler) volumes(z *zitadelv1alpha1.Zitadel) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: z.Name + "-config",
					},
				},
			},
		},
		{
			Name: "db-secrets",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: z.Name + "-db-config",
				},
			},
		},
	}

	if z.Spec.Network.TLS != nil && z.Spec.Network.TLS.Enabled && z.Spec.Network.TLS.SecretName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: z.Spec.Network.TLS.SecretName,
				},
			},
		})
	}

	return volumes
}

// volumeMounts returns the volume mount list for Zitadel containers.
func (r *ZitadelReconciler) volumeMounts(z *zitadelv1alpha1.Zitadel) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/config",
			ReadOnly:  true,
		},
		{
			Name:      "db-secrets",
			MountPath: "/secrets",
			ReadOnly:  true,
		},
	}

	if z.Spec.Network.TLS != nil && z.Spec.Network.TLS.Enabled {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "tls",
			MountPath: "/etc/zitadel/tls",
			ReadOnly:  true,
		})
	}

	return mounts
}

// isJobComplete checks if a Job has succeeded or failed beyond retries.
func isJobComplete(job *batchv1.Job) (bool, error) {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true, nil
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return false, fmt.Errorf("job %s failed: %s", job.Name, c.Message)
		}
	}
	return false, nil
}

// secretKey returns the key from a SecretKeyRef, defaulting to "password" if empty.
func secretKey(ref zitadelv1alpha1.SecretKeyRef) string {
	if ref.Key != "" {
		return ref.Key
	}
	return "password"
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZitadelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zitadelv1alpha1.Zitadel{}).
		Owns(&batchv1.Job{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.Ingress{}).
		Named("zitadel").
		Complete(r)
}
