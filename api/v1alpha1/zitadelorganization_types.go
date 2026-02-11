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

// ZitadelOrganizationSpec defines the desired state of a Zitadel organization.
type ZitadelOrganizationSpec struct {
	// ZitadelRef names the Zitadel instance in the same namespace.
	ZitadelRef string `json:"zitadelRef"`
	// Name of the organization in Zitadel.
	Name string `json:"name"`
}

// ZitadelOrganizationStatus defines the observed state of a Zitadel organization.
type ZitadelOrganizationStatus struct {
	// ObservedGeneration is the last .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// OrgID is the Zitadel-assigned organization ID.
	// +optional
	OrgID string `json:"orgID,omitempty"`
	// Ready indicates the organization exists and is active.
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
// +kubebuilder:printcolumn:name="OrgID",type=string,JSONPath=`.status.orgID`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZitadelOrganization is the Schema for the zitadelorganizations API.
type ZitadelOrganization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelOrganizationSpec   `json:"spec,omitempty"`
	Status ZitadelOrganizationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelOrganizationList contains a list of ZitadelOrganization.
type ZitadelOrganizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZitadelOrganization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZitadelOrganization{}, &ZitadelOrganizationList{})
}
