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

// ZitadelProjectSpec defines the desired state of a Zitadel project.
type ZitadelProjectSpec struct {
	// ZitadelRef names the Zitadel instance in the same namespace.
	ZitadelRef string `json:"zitadelRef"`
	// OrganizationRef names the ZitadelOrganization in the same namespace.
	// If omitted, uses the default org of the Zitadel instance.
	// +optional
	OrganizationRef string `json:"organizationRef,omitempty"`
	// Name of the project.
	Name string `json:"name"`
	// ProjectRoleAssertion enables role assertion in tokens.
	// +kubebuilder:default=false
	// +optional
	ProjectRoleAssertion bool `json:"projectRoleAssertion,omitempty"`
	// ProjectRoleCheck enables role check on authentication.
	// +kubebuilder:default=false
	// +optional
	ProjectRoleCheck bool `json:"projectRoleCheck,omitempty"`
	// HasProjectCheck verifies project existence on auth.
	// +kubebuilder:default=false
	// +optional
	HasProjectCheck bool `json:"hasProjectCheck,omitempty"`
}

// ZitadelProjectStatus defines the observed state of a Zitadel project.
type ZitadelProjectStatus struct {
	// ObservedGeneration is the last .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ProjectID is the Zitadel-assigned project ID.
	// +optional
	ProjectID string `json:"projectID,omitempty"`
	// Ready indicates the project exists and is active.
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
// +kubebuilder:printcolumn:name="ProjectID",type=string,JSONPath=`.status.projectID`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZitadelProject is the Schema for the zitadelprojects API.
type ZitadelProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelProjectSpec   `json:"spec,omitempty"`
	Status ZitadelProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelProjectList contains a list of ZitadelProject.
type ZitadelProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZitadelProject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZitadelProject{}, &ZitadelProjectList{})
}
