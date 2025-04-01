// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
)

const (
	ClusterOrchResourceGroup   = "edge-orchestrator.intel.com"
	ClusterOrchResourceVersion = "v1alpha1"

	TemplateLabelKey = ClusterOrchResourceGroup + "/template"

	ActiveProjectIdHeaderKey             = "Activeprojectid"
	ActiveProjectIdContextKey ContextKey = ActiveProjectIdHeaderKey
	ClusterInstances                     = "spec.topology.classRef"
)

type ContextKey string

const (
	TemplateResourceGroup   = "edge-orchestrator.intel.com"
	TemplateResourceVersion = "v1alpha1"
	TemplateResourceKind    = "clustertemplates"
)

var (
	BindingsResourceSchema = schema.GroupVersionResource{
		Group:    intelv1alpha1.GroupVersion.Group,
		Version:  intelv1alpha1.GroupVersion.Version,
		Resource: "intelmachinebindings",
	}
	IntelMachineResourceSchema = schema.GroupVersionResource{
		Group:    intelv1alpha1.GroupVersion.Group,
		Version:  intelv1alpha1.GroupVersion.Version,
		Resource: "intelmachines",
	}
	ClusterResourceSchema = schema.GroupVersionResource{
		Group:    "cluster.x-k8s.io",
		Version:  "v1beta1",
		Resource: "clusters",
	}
	TemplateResourceSchema = schema.GroupVersionResource{
		Group:    TemplateResourceGroup,
		Version:  TemplateResourceVersion,
		Resource: TemplateResourceKind,
	}
	MachineResourceSchema = schema.GroupVersionResource{
		Group:    "cluster.x-k8s.io",
		Version:  "v1beta1",
		Resource: "machines",
	}
	SecretResourceSchema = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}
)
