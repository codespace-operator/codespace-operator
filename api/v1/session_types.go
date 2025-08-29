/*
Copyright 2025 Dennis Marcus Goh.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProfileSpec struct {
	// +kubebuilder:validation:Enum=jupyterlab;vscode;rstudio;custom
	IDE string `json:"ide"`
	// +kubebuilder:validation:MinLength=1
	Image string   `json:"image"`
	Cmd   []string `json:"cmd,omitempty"`
}

type OIDCRef struct {
	// +kubebuilder:validation:Pattern=`^https?://`
	IssuerURL       string `json:"issuerURL"`
	ClientIDSecret  string `json:"clientIDSecret,omitempty"`
	ClientSecretRef string `json:"clientSecretRef,omitempty"`
}

type AuthSpec struct {
	// +kubebuilder:validation:Enum=oauth2proxy;none
	// +kubebuilder:default=none
	Mode string   `json:"mode,omitempty"`
	OIDC *OIDCRef `json:"oidc,omitempty"`
}

type PVCSpec struct {
	// +kubebuilder:validation:Pattern=`^\d+(Gi|Mi)$`
	Size             string `json:"size"`
	StorageClassName string `json:"storageClassName,omitempty"`
	// +kubebuilder:validation:MinLength=1
	MountPath string `json:"mountPath"`
}

type NetSpec struct {
	Host          string            `json:"host,omitempty"`
	TLSSecretName string            `json:"tlsSecretName,omitempty"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

type SessionSpec struct {
	Profile    ProfileSpec `json:"profile"`
	Auth       AuthSpec    `json:"auth,omitempty"`
	Home       *PVCSpec    `json:"home,omitempty"`
	Scratch    *PVCSpec    `json:"scratch,omitempty"`
	Networking *NetSpec    `json:"networking,omitempty"`
	Replicas   *int32      `json:"replicas,omitempty"`
}

type SessionStatus struct {
	Phase  string `json:"phase,omitempty"` // Pending | Ready | Error
	URL    string `json:"url,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Session struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SessionSpec   `json:"spec,omitempty"`
	Status SessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Session `json:"items"`
}

// Register the API types with the SchemeBuilder.
func init() {
	SchemeBuilder.Register(&Session{}, &SessionList{})
}
