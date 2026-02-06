/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArenaDevSessionSpec defines the desired state of ArenaDevSession.
// A dev session creates an ephemeral dev console for interactive agent testing.
type ArenaDevSessionSpec struct {
	// projectID is the Arena project being tested.
	// +kubebuilder:validation:Required
	ProjectID string `json:"projectId"`

	// workspace is the workspace name (for reference/labeling).
	// +kubebuilder:validation:Required
	Workspace string `json:"workspace"`

	// idleTimeout specifies how long the session can be idle before cleanup.
	// Default is 30 minutes.
	// +kubebuilder:default="30m"
	// +optional
	IdleTimeout string `json:"idleTimeout,omitempty"`

	// image overrides the default dev console image.
	// +optional
	Image string `json:"image,omitempty"`

	// resources overrides the default resource requests/limits.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

// ResourceRequirements describes compute resource requirements.
type ResourceRequirements struct {
	// requests describes minimum resources required.
	// +optional
	Requests map[string]string `json:"requests,omitempty"`
	// limits describes maximum resources allowed.
	// +optional
	Limits map[string]string `json:"limits,omitempty"`
}

// ArenaDevSessionPhase represents the current phase of a dev session.
// +kubebuilder:validation:Enum=Pending;Starting;Ready;Stopping;Stopped;Failed
type ArenaDevSessionPhase string

const (
	// ArenaDevSessionPhasePending indicates the session is waiting to be processed.
	ArenaDevSessionPhasePending ArenaDevSessionPhase = "Pending"
	// ArenaDevSessionPhaseStarting indicates the dev console is being created.
	ArenaDevSessionPhaseStarting ArenaDevSessionPhase = "Starting"
	// ArenaDevSessionPhaseReady indicates the dev console is ready for connections.
	ArenaDevSessionPhaseReady ArenaDevSessionPhase = "Ready"
	// ArenaDevSessionPhaseStopping indicates the session is being cleaned up.
	ArenaDevSessionPhaseStopping ArenaDevSessionPhase = "Stopping"
	// ArenaDevSessionPhaseStopped indicates the session has been cleaned up.
	ArenaDevSessionPhaseStopped ArenaDevSessionPhase = "Stopped"
	// ArenaDevSessionPhaseFailed indicates the session failed to start.
	ArenaDevSessionPhaseFailed ArenaDevSessionPhase = "Failed"
)

// ArenaDevSessionStatus defines the observed state of ArenaDevSession.
type ArenaDevSessionStatus struct {
	// phase represents the current lifecycle phase.
	// +optional
	Phase ArenaDevSessionPhase `json:"phase,omitempty"`

	// endpoint is the WebSocket URL to connect to the dev console.
	// Format: ws://arena-dev-console-{name}.{namespace}.svc:8080/ws
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// serviceName is the name of the created service.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// lastActivityAt is the timestamp of the last client activity.
	// Used for idle timeout cleanup.
	// +optional
	LastActivityAt *metav1.Time `json:"lastActivityAt,omitempty"`

	// startedAt is when the dev console became ready.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// message provides additional status information.
	// +optional
	Message string `json:"message,omitempty"`

	// conditions represent the current state of the dev session.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ads
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.projectId`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ArenaDevSession is the Schema for the arenadevsessions API.
// It represents an ephemeral interactive testing session for an Arena project.
type ArenaDevSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ArenaDevSessionSpec   `json:"spec,omitempty"`
	Status ArenaDevSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ArenaDevSessionList contains a list of ArenaDevSession.
type ArenaDevSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArenaDevSession `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ArenaDevSession{}, &ArenaDevSessionList{})
}
