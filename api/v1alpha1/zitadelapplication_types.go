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

// ZitadelApplicationSpec defines the desired state of a Zitadel application.
type ZitadelApplicationSpec struct {
	// ZitadelRef names the Zitadel instance in the same namespace.
	ZitadelRef string `json:"zitadelRef"`
	// ProjectRef names the ZitadelProject in the same namespace.
	ProjectRef string `json:"projectRef"`
	// Name of the application.
	Name string `json:"name"`
	// Type is the application type.
	// +kubebuilder:validation:Enum=OIDC;SAML;API
	Type string `json:"type"`

	// OIDC settings (required when type is OIDC).
	// +optional
	OIDC *OIDCAppConfig `json:"oidc,omitempty"`
	// API settings (required when type is API).
	// +optional
	API *APIAppConfig `json:"api,omitempty"`
	// SAML settings (required when type is SAML).
	// +optional
	SAML *SAMLAppConfig `json:"saml,omitempty"`

	// ClientSecretRef optionally names a Secret where generated client
	// credentials (client_id, client_secret) are written.
	// +optional
	ClientSecretRef string `json:"clientSecretRef,omitempty"`
}

// OIDCAppConfig configures an OIDC application.
type OIDCAppConfig struct {
	// RedirectURIs for OIDC redirect.
	RedirectURIs []string `json:"redirectUris"`
	// PostLogoutRedirectURIs for post-logout.
	// +optional
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris,omitempty"`
	// ResponseTypes: CODE, ID_TOKEN, ID_TOKEN_TOKEN.
	// +kubebuilder:default={"CODE"}
	// +optional
	ResponseTypes []string `json:"responseTypes,omitempty"`
	// GrantTypes: AUTHORIZATION_CODE, IMPLICIT, REFRESH_TOKEN, DEVICE_CODE.
	// +kubebuilder:default={"AUTHORIZATION_CODE"}
	// +optional
	GrantTypes []string `json:"grantTypes,omitempty"`
	// AppType: WEB, USER_AGENT, NATIVE.
	// +kubebuilder:validation:Enum=WEB;USER_AGENT;NATIVE
	// +kubebuilder:default="WEB"
	// +optional
	AppType string `json:"appType,omitempty"`
	// AuthMethodType: BASIC, POST, NONE, PRIVATE_KEY_JWT.
	// +kubebuilder:validation:Enum=BASIC;POST;NONE;PRIVATE_KEY_JWT
	// +kubebuilder:default="BASIC"
	// +optional
	AuthMethodType string `json:"authMethodType,omitempty"`
	// AccessTokenType: BEARER, JWT.
	// +kubebuilder:default="BEARER"
	// +optional
	AccessTokenType string `json:"accessTokenType,omitempty"`
	// DevMode enables development mode (allows http redirect URIs).
	// +kubebuilder:default=false
	// +optional
	DevMode bool `json:"devMode,omitempty"`
}

// APIAppConfig configures an API application.
type APIAppConfig struct {
	// AuthMethodType: BASIC, PRIVATE_KEY_JWT.
	// +kubebuilder:validation:Enum=BASIC;PRIVATE_KEY_JWT
	// +kubebuilder:default="BASIC"
	// +optional
	AuthMethodType string `json:"authMethodType,omitempty"`
}

// SAMLAppConfig configures a SAML application.
type SAMLAppConfig struct {
	// MetadataXML is the raw SAML metadata.
	// +optional
	MetadataXML string `json:"metadataXml,omitempty"`
	// MetadataURL is the URL to fetch SAML metadata.
	// +optional
	MetadataURL string `json:"metadataUrl,omitempty"`
}

// ZitadelApplicationStatus defines the observed state of a Zitadel application.
type ZitadelApplicationStatus struct {
	// ObservedGeneration is the last .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// AppID is the Zitadel-assigned application ID.
	// +optional
	AppID string `json:"appID,omitempty"`
	// ClientID is the OIDC/API client ID.
	// +optional
	ClientID string `json:"clientID,omitempty"`
	// Ready indicates the application exists and is active.
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
// +kubebuilder:printcolumn:name="ClientID",type=string,JSONPath=`.status.clientID`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ZitadelApplication is the Schema for the zitadelapplications API.
type ZitadelApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZitadelApplicationSpec   `json:"spec,omitempty"`
	Status ZitadelApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ZitadelApplicationList contains a list of ZitadelApplication.
type ZitadelApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZitadelApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZitadelApplication{}, &ZitadelApplicationList{})
}
