// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controlplaneprovider

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DockerMachineTemplate = "DockerMachineTemplate"
	DockerClusterTemplate = "DockerClusterTemplate"

	IntelMachineTemplate = "IntelMachineTemplate"
	IntelClusterTemplate = "IntelClusterTemplate"

	KubeadmControlPlaneTemplate = "KubeadmControlPlaneTemplate"
	RKE2ControlPlaneTemplate    = "RKE2ControlPlaneTemplate"
	KThreesControlPlaneTemplate = "KThreesControlPlaneTemplate"

	DefaultProvider = "k3s"
)

type Provider interface {
	AlterClusterClass(cc *capiv1beta1.ClusterClass)

	CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error
	CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error
	CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error
	CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error

	DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error

	GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error
	GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error
	GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error
	GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error
}

var (
	connectAgentManifest  = "connectAgentManifest"
	connectAgentEnabledIf = "{{ if .connectAgentManifest.path }}true{{ end }}"
	readOnlyEnabledIf     = "{{ if .readOnly }}true{{ end }}"

	ReadOnly = "readOnly"
)
var providerRegistry = map[string]Provider{
	"kubeadm:docker": kubeadmdocker{},
	"rke2:docker":    rke2docker{},
	"rke2:intel":     rke2intel{},
	"k3s:intel":      k3sintel{},
	"k3s:docker":     k3sdocker{},
}

func GetCapiProvider(controlPlaneProvider, infraProvider string) Provider {
	key := controlPlaneProvider + ":" + infraProvider
	return providerRegistry[key]
}
