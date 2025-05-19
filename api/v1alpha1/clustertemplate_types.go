// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	ClusterTemplateFinalizer = "clustertemplates.edge-orchestrator.intel.com/finalizer"

	// PrerequisitesCondition documents the status of the CAPI Resources prerequisites installation.
	PrerequisitesCondition clusterv1.ConditionType = "PrerequisitesInstalled"

	// ControlPlaneTemplateCondition documents the status of the ControlPlaneTemplate creation.
	ControlPlaneTemplateCondition clusterv1.ConditionType = "ControlPlaneTemplateCreated"

	// ControlPlaneMachineTemplateCondition documents the status of the ControlPlaneMachineTemplate creation.
	ControlPlaneMachineTemplateCondition clusterv1.ConditionType = "ControlPlaneMachineTemplateCreated"

	// ProviderClusterTemplateCondition documents the status of the <infrastructure provider>ClusterTemplate creation.
	InfraProviderClusterTemplateCondition clusterv1.ConditionType = "InfraProviderClusterTemplateCreated"

	// ClusterClassCondition documents the status of the ClusterClassCondition creation.
	ClusterClassCondition clusterv1.ConditionType = "ClusterClassCreated"
)

// ClusterTemplateSpec defines the desired state of ClusterTemplate.
type ClusterTemplateSpec struct {
	// +optional
	// +kubebuilder:validation:Enum=kubeadm;rke2;k3s
	// +kubebuilder:default=rke2
	ControlPlaneProviderType string `json:"controlPlaneProviderType,omitempty" yaml:"controlPlaneProviderType"`

	// +optional
	// +kubebuilder:validation:Enum=intel;docker
	InfraProviderType string `json:"infraProviderType,omitempty" yaml:"infraProviderType,omitempty"`

	// +required
	KubernetesVersion string `json:"kubernetesVersion,omitempty" yaml:"kubernetesVersion"`

	// +optional
	ClusterConfiguration string `json:"clusterConfiguration,omitempty" yaml:"clusterConfiguration,omitempty"`

	// +optional
	ClusterNetwork ClusterNetwork `json:"clusterNetwork,omitempty" yaml:"clusterNetwork,omitempty"`

	// +optional
	ClusterLabels map[string]string `json:"clusterLabels,omitempty" yaml:"clusterLabels,omitempty"`
}

// ClusterNetwork specifies the different networking
// parameters for a cluster.
type ClusterNetwork struct {
	// The network ranges from which service VIPs are allocated.
	// +optional
	Services *NetworkRanges `json:"services,omitempty" yaml:"services,omitempty"`

	// The network ranges from which Pod networks are allocated.
	// +optional
	Pods *NetworkRanges `json:"pods,omitempty" yaml:"pods,omitempty"`
}

// NetworkRanges represents ranges of network addresses.
type NetworkRanges struct {
	CIDRBlocks []string `json:"cidrBlocks" yaml:"cidrBlocks"`
}

func (n NetworkRanges) String() string {
	if len(n.CIDRBlocks) == 0 {
		return ""
	}
	return strings.Join(n.CIDRBlocks, ",")
}

// ClusterTemplateStatus defines the observed state of ClusterTemplate.
type ClusterTemplateStatus struct {
	// +optional
	Ready           bool                    `json:"ready" yaml:"ready"`
	ClusterClassRef *corev1.ObjectReference `json:"clusterClassRef,omitempty" yaml:"clusterClassRef,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in ClusterTemplate's status with the V1Beta2 version.
	// +optional
	V1Beta2 *ClusterTemplateV1Beta2Status `json:"v1beta2,omitempty" yaml:"v1beta2,omitempty"`

	Conditions clusterv1.Conditions `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// ClusterTemplateV1Beta2Status groups all the fields that will be added or modified in ClusterTemplate with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type ClusterTemplateV1Beta2Status struct {
	// conditions represents the observations of an ClusterTemplate's current state.
	// Known condition types are Ready, Provisioned, BootstrapExecSucceeded, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty" yaml:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean, JSONPath=".status.ready", description="ClusterTemplate readiness status such as True/False"

// ClusterTemplate is the Schema for the clustertemplates API.
type ClusterTemplate struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   ClusterTemplateSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status ClusterTemplateStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *ClusterTemplate) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *ClusterTemplate) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (c *ClusterTemplate) GetV1Beta2Conditions() []metav1.Condition {
	if c.Status.V1Beta2 == nil {
		return nil
	}
	return c.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (c *ClusterTemplate) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if c.Status.V1Beta2 == nil {
		c.Status.V1Beta2 = &ClusterTemplateV1Beta2Status{}
	}
	c.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// ClusterTemplateList contains a list of ClusterTemplate.
type ClusterTemplateList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items           []ClusterTemplate `json:"items" yaml:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterTemplate{}, &ClusterTemplateList{})
}
