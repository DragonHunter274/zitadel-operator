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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ZitadelUserSpec defines the desired state of a Zitadel user.
type ZitadelUserSpec struct {
	// ZitadelRef names the Zitadel instance in the same namespace.
	ZitadelRef string `json:"zitadelRef"`
	// OrganizationRef names the ZitadelOrganization in the same namespace.
	// +optional
	OrganizationRef string `json:"organizationRef,omitempty"`
	// Type: Human or Machine.
	// +kubebuilder:validation:Enum=Human;Machine
	Type string `json:"type"`
	// UserName is the login name.
	UserName string `json:"userName"`

	// Human profile (required when type is Human).
	// +optional
	Human *HumanUserSpec `json:"human,omitempty"`
	// Machine profile (required when type is Machine).
	// +optional
	Machine *MachineUserSpec `json:"machine,omitempty"`

	// CredentialSecretRef optionally names a Secret where credentials
	// (PAT or generated password) are written back.
	// +optional
	CredentialSecretRef string `json:"credentialSecretRef,omitempty"`
}

// HumanUserSpec defines a human user profile.
type HumanUserSpec struct {
	// FirstName of the user.
	FirstName string `json:"firstName"`
	// LastName of the user.
	LastName string `json:"lastName"`
	// Email address.
	Email string `json:"email"`
	// PasswordRef references a Secret containing the initial password.
	// +optional
	PasswordRef *SecretKeyRef `json:"passwordRef,omitempty"`
}

// MachineUserSpec defines a machine/service account user.
type MachineUserSpec struct {
	// Name is the display name.
	Name string `json:"name"`
	// Description of the machine user.
	// +optional
	Description string `json:"description,omitempty"`
	// GeneratePAT tells the controller to create a Personal Access Token.
	// +kubebuilder:default=false
	// +optional
	GeneratePAT bool `json:"generatePAT,omitempty"`
}

// ZitadelUserStatus defines the observed state of a Zitadel user.
type ZitadelUserStatus struct {
	// ObservedGeneration is the last .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// UserID is the Zitadel-assigned user ID.
	// +optional
	UserID string `json:"userID,omitempty"`
	// Ready indicates the user exists and is active.
	// +optional
	Ready bool `json:"ready,omitempty"`
	// Conditions follow the standard Kubernetes conditions pattern.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="UserID",type=string,JSONPath=`.status.userID`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZitadelUser is the Schema for the zitadelusers API.
type ZitadelUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelUserSpec   `json:"spec,omitempty"`
	Status ZitadelUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelUserList contains a list of ZitadelUser.
type ZitadelUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZitadelUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZitadelUser{}, &ZitadelUserList{})
}
