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

// ZitadelRoleSpec defines the desired state of Zitadel project roles.
type ZitadelRoleSpec struct {
	// ZitadelRef names the Zitadel instance in the same namespace.
	ZitadelRef string `json:"zitadelRef"`
	// ProjectRef names the ZitadelProject in the same namespace.
	ProjectRef string `json:"projectRef"`
	// Roles to create in the project.
	Roles []RoleEntry `json:"roles"`
}

// RoleEntry defines a single role within a project.
type RoleEntry struct {
	// Key is the unique role key within the project.
	Key string `json:"key"`
	// DisplayName is the human-readable name.
	DisplayName string `json:"displayName"`
	// Group is an optional grouping for roles.
	// +optional
	Group string `json:"group,omitempty"`
}

// ZitadelRoleStatus defines the observed state of Zitadel project roles.
type ZitadelRoleStatus struct {
	// ObservedGeneration is the last .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// SyncedRoles lists the role keys that have been synced.
	// +optional
	SyncedRoles []string `json:"syncedRoles,omitempty"`
	// Ready indicates all roles are synced.
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
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Synced",type=integer,JSONPath=`.status.syncedRoles`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZitadelRole is the Schema for the zitadelroles API.
type ZitadelRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelRoleSpec   `json:"spec,omitempty"`
	Status ZitadelRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelRoleList contains a list of ZitadelRole.
type ZitadelRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZitadelRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZitadelRole{}, &ZitadelRoleList{})
}
