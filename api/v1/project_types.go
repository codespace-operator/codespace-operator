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

// ProjectMember defines a member of a CodespaceProject
type ProjectMember struct {
	// Subject is the user ID or group name (email, username, etc)
	// +kubebuilder:validation:MinLength=1
	Subject string `json:"subject"`

	// Role defines the access level
	// +kubebuilder:validation:Enum=owner;admin;developer;viewer
	// +kubebuilder:default=viewer
	Role string `json:"role"`

	// AddedAt timestamp when the member was added
	AddedAt *metav1.Time `json:"addedAt,omitempty"`

	// AddedBy who added this member
	AddedBy string `json:"addedBy,omitempty"`
}

// ResourceQuotas defines resource limits for a project
type ResourceQuotas struct {
	// MaxSessions limits the number of active sessions
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	MaxSessions *int32 `json:"maxSessions,omitempty"`

	// MaxReplicas limits replicas per session
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// MaxCpuPerSession limits CPU per session
	// +kubebuilder:validation:Pattern=`^\d+(\.\d+)?(m|[KMGTPE]i?)?$`
	// +kubebuilder:default="2"
	MaxCpuPerSession string `json:"maxCpuPerSession,omitempty"`

	// MaxMemoryPerSession limits memory per session
	// +kubebuilder:validation:Pattern=`^\d+(\.\d+)?(Ki|Mi|Gi|Ti|Pi|Ei|K|M|G|T|P|E)?$`
	// +kubebuilder:default="4Gi"
	MaxMemoryPerSession string `json:"maxMemoryPerSession,omitempty"`
}

// ProjectSpec defines the desired state of Project
type ProjectSpec struct {
	// DisplayName is a human-readable name for the project
	// +kubebuilder:validation:MaxLength=100
	DisplayName string `json:"displayName,omitempty"`

	// Description provides additional context about the project
	// +kubebuilder:validation:MaxLength=500
	Description string `json:"description,omitempty"`

	// Members list of users/groups with access to this project
	// +kubebuilder:validation:MaxItems=100
	Members []ProjectMember `json:"members,omitempty"`

	// Namespaces restricts sessions to specific namespaces (empty = all allowed)
	// +kubebuilder:validation:MaxItems=50
	Namespaces []string `json:"namespaces,omitempty"`

	// ResourceQuotas defines limits for sessions in this project
	ResourceQuotas *ResourceQuotas `json:"resourceQuotas,omitempty"`

	// DefaultSessionProfile provides default values for new sessions
	DefaultSessionProfile *ProfileSpec `json:"defaultSessionProfile,omitempty"`

	// ImageAllowlist restricts allowed container images (empty = all allowed)
	// +kubebuilder:validation:MaxItems=100
	ImageAllowlist []string `json:"imageAllowlist,omitempty"`

	// ImageDenylist blocks specific container images
	// +kubebuilder:validation:MaxItems=100
	ImageDenylist []string `json:"imageDenylist,omitempty"`

	// Suspended indicates if the project is temporarily disabled
	// +kubebuilder:default=false
	Suspended bool `json:"suspended,omitempty"`
}

// ProjectStatus defines the observed state of Project
type ProjectStatus struct {
	// Phase indicates the project state
	// +kubebuilder:validation:Enum=Active;Inactive;Error;Suspended
	// +kubebuilder:default=Active
	Phase string `json:"phase,omitempty"`

	// MemberCount tracks the number of active members
	MemberCount int32 `json:"memberCount,omitempty"`

	// SessionCount tracks active sessions in this project
	SessionCount int32 `json:"sessionCount,omitempty"`

	// Reason provides context for the current phase
	Reason string `json:"reason,omitempty"`

	// LastUpdated timestamp of last status update
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// Conditions represent the latest available observations of the project's state
	// +kubebuilder:validation:MaxItems=10
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed Project
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced,shortName=proj;project
//+kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
//+kubebuilder:printcolumn:name="Members",type=integer,JSONPath=`.status.memberCount`
//+kubebuilder:printcolumn:name="Sessions",type=integer,JSONPath=`.status.sessionCount`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Project is the Schema for the projects API
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProjectList contains a list of Project
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
