package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProfileSpec struct {
	IDE   string   `json:"ide"` // jupyterlab | vscode | rstudio | custom
	Image string   `json:"image"`
	Cmd   []string `json:"cmd,omitempty"`
}

type OIDCRef struct {
	IssuerURL       string `json:"issuerURL"`
	ClientIDSecret  string `json:"clientIDSecret,omitempty"`  // secret name; key: client_id
	ClientSecretRef string `json:"clientSecretRef,omitempty"` // secret name; key: client_secret
}

type AuthSpec struct {
	Mode string   `json:"mode,omitempty"` // oauth2proxy | none
	OIDC *OIDCRef `json:"oidc,omitempty"`
}

type PVCSpec struct {
	Size             string `json:"size"`
	StorageClassName string `json:"storageClassName,omitempty"`
	MountPath        string `json:"mountPath"`
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
