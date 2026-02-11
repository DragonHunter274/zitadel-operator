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

// SecretKeyRef references a key within a Kubernetes Secret.
type SecretKeyRef struct {
	// Name of the Secret.
	Name string `json:"name"`
	// Key within the Secret.
	// +optional
	Key string `json:"key,omitempty"`
}

// Condition type constants for the Zitadel CRD.
const (
	ConditionTypeInitialized      = "Initialized"
	ConditionTypeSetupCompleted   = "SetupCompleted"
	ConditionTypeAvailable        = "Available"
	ConditionTypeServiceAccount   = "ServiceAccountReady"
	ConditionTypeDegraded         = "Degraded"
	ConditionTypeReady            = "Ready"
	ConditionTypeSynced           = "Synced"
	ConditionTypeZitadelAvailable = "ZitadelAvailable"
)

// Phase constants for the Zitadel CRD.
const (
	PhasePending      = "Pending"
	PhaseInitializing = "Initializing"
	PhaseRunning      = "Running"
	PhaseError        = "Error"
)

// Finalizer name for API resource cleanup.
const (
	Finalizer = "zitadel.dragonhunter274.github.com/finalizer"
)
