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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ZitadelSpec defines the desired state of a Zitadel instance.
type ZitadelSpec struct {
	// Version is the Zitadel container image tag (e.g. "v2.67.2").
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Replicas for the main Zitadel Deployment.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Database configures the PostgreSQL connection.
	Database DatabaseSpec `json:"database"`

	// Network configures external access and TLS.
	Network NetworkSpec `json:"network"`

	// Masterkey references the Secret containing the 32-char encryption key.
	// The Secret must have a key named "masterkey".
	Masterkey SecretKeyRef `json:"masterkey"`

	// Configuration holds freeform Zitadel YAML config (non-sensitive).
	// Rendered into a ConfigMap and mounted at /config/zitadel-config.yaml.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	Configuration *runtime.RawExtension `json:"configuration,omitempty"`

	// SecretConfiguration references a Secret containing sensitive YAML
	// config overrides mounted after the ConfigMap.
	// +optional
	SecretConfiguration *SecretKeyRef `json:"secretConfiguration,omitempty"`

	// FirstInstance configures the initial instance bootstrapped by "zitadel setup".
	// +optional
	FirstInstance *FirstInstanceSpec `json:"firstInstance,omitempty"`

	// ServiceAccount configures the operator-managed service account
	// used for API-resource controllers to authenticate to Zitadel.
	// +optional
	ServiceAccount *OperatorServiceAccountSpec `json:"serviceAccount,omitempty"`

	// Ingress configures optional Ingress resource creation.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Resources defines compute resources for the Zitadel containers.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// DatabaseSpec configures the PostgreSQL connection.
type DatabaseSpec struct {
	// Host is the PostgreSQL server hostname.
	Host string `json:"host"`

	// Port is the PostgreSQL server port.
	// +kubebuilder:default=5432
	// +optional
	Port int32 `json:"port,omitempty"`

	// Database is the database name.
	// +kubebuilder:default="zitadel"
	// +optional
	Database string `json:"database,omitempty"`

	// AdminCredentials references a Secret with keys "username" and "password"
	// for the admin database user (used by init and setup jobs).
	AdminCredentials SecretKeyRef `json:"adminCredentials"`

	// UserCredentials references a Secret with keys "username" and "password"
	// for the application database user (used at runtime).
	UserCredentials SecretKeyRef `json:"userCredentials"`

	// SSLMode for PostgreSQL connections.
	// +kubebuilder:default="disable"
	// +kubebuilder:validation:Enum=disable;require;verify-ca;verify-full
	// +optional
	SSLMode string `json:"sslMode,omitempty"`

	// SSLRootCert references a Secret containing the CA certificate.
	// +optional
	SSLRootCert *SecretKeyRef `json:"sslRootCert,omitempty"`
}

// NetworkSpec configures external access and TLS.
type NetworkSpec struct {
	// ExternalDomain is the user-facing domain (e.g. "auth.example.com").
	ExternalDomain string `json:"externalDomain"`

	// ExternalPort is the user-facing port.
	// +kubebuilder:default=443
	// +optional
	ExternalPort int32 `json:"externalPort,omitempty"`

	// ExternalSecure indicates whether TLS is terminated externally.
	// +kubebuilder:default=true
	// +optional
	ExternalSecure *bool `json:"externalSecure,omitempty"`

	// TLS configures TLS termination on the Zitadel pod itself.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`
}

// TLSSpec configures TLS on the Zitadel pod.
type TLSSpec struct {
	// Enabled toggles TLS on the Zitadel pod.
	Enabled bool `json:"enabled"`

	// SecretName references a TLS Secret (tls.crt, tls.key).
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// IngressSpec configures Ingress resource creation.
type IngressSpec struct {
	// Enabled toggles Ingress creation.
	Enabled bool `json:"enabled"`

	// IngressClassName specifies the Ingress controller class.
	// +optional
	IngressClassName *string `json:"ingressClassName,omitempty"`

	// Annotations for the Ingress resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// TLS configuration for the Ingress.
	// +optional
	TLS []networkingv1.IngressTLS `json:"tls,omitempty"`
}

// FirstInstanceSpec configures the initial Zitadel instance.
type FirstInstanceSpec struct {
	// Org configures the default organization.
	Org FirstInstanceOrg `json:"org"`
}

// FirstInstanceOrg configures the default organization for first instance setup.
type FirstInstanceOrg struct {
	// Name of the default organization.
	Name string `json:"name"`

	// Human configures the initial admin user.
	Human FirstInstanceHuman `json:"human"`
}

// FirstInstanceHuman configures the initial admin user.
type FirstInstanceHuman struct {
	// UserName for the admin.
	UserName string `json:"userName"`
	// FirstName of the admin.
	FirstName string `json:"firstName"`
	// LastName of the admin.
	LastName string `json:"lastName"`
	// Email address.
	Email string `json:"email"`
	// PasswordRef references a Secret containing key "password".
	PasswordRef SecretKeyRef `json:"passwordRef"`
}

// OperatorServiceAccountSpec configures the operator service account.
type OperatorServiceAccountSpec struct {
	// UserName for the machine user the operator uses to call Zitadel APIs.
	// +kubebuilder:default="zitadel-operator"
	// +optional
	UserName string `json:"userName,omitempty"`

	// SecretName where the operator writes the PAT after creating it.
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// ZitadelStatus defines the observed state of a Zitadel instance.
type ZitadelStatus struct {
	// Phase is a high-level summary: Pending, Initializing, Running, Error.
	// +kubebuilder:validation:Enum=Pending;Initializing;Running;Error
	// +optional
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the last .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Version is the currently deployed Zitadel version.
	// +optional
	Version string `json:"version,omitempty"`

	// Ready is the number of ready replicas.
	// +optional
	Ready int32 `json:"ready,omitempty"`

	// ServiceAccountReady indicates whether the operator service account
	// has been created and its PAT is available.
	// +optional
	ServiceAccountReady bool `json:"serviceAccountReady,omitempty"`

	// Conditions follow the standard Kubernetes conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Zitadel is the Schema for the zitadels API.
type Zitadel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelSpec   `json:"spec,omitempty"`
	Status ZitadelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelList contains a list of Zitadel.
type ZitadelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Zitadel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Zitadel{}, &ZitadelList{})
}
