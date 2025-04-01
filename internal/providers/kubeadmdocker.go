// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controlplaneprovider

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmcpv1beta1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	dockerv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubeadmdocker struct {
}

func (kubeadmdocker) CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (kubeadmdocker) CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error {
	var cpt kubeadmcpv1beta1.KubeadmControlPlaneTemplate
	if err := json.Unmarshal([]byte(config), &cpt); err != nil {
		return fmt.Errorf("failed to unmarshal control plane template: %w", err)
	}

	cpt.ObjectMeta = metav1.ObjectMeta{
		Name:      name.Name,
		Namespace: name.Namespace,
	}

	return c.Create(ctx, &cpt)
}

func (kd kubeadmdocker) CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	cpmt := dockerv1beta1.DockerMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-controlplane", name.Name),
			Namespace: name.Namespace,
		},
		Spec: dockerv1beta1.DockerMachineTemplateSpec{
			Template: dockerv1beta1.DockerMachineTemplateResource{
				Spec: dockerv1beta1.DockerMachineSpec{
					ExtraMounts: []dockerv1beta1.Mount{
						{ContainerPath: "/var/run/docker.sock", HostPath: "/var/run/docker.sock"},
					},
				},
			},
		},
	}
	return c.Create(ctx, &cpmt)
}

func (kd kubeadmdocker) CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	ct := dockerv1beta1.DockerClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: dockerv1beta1.DockerClusterTemplateSpec{
			Template: dockerv1beta1.DockerClusterTemplateResource{
				Spec: dockerv1beta1.DockerClusterSpec{},
			},
		},
	}
	return c.Create(ctx, &ct)
}

func (kubeadmdocker) DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (kubeadmdocker) GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (kubeadmdocker) GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &kubeadmcpv1beta1.KubeadmControlPlaneTemplate{})
}

func (kubeadmdocker) GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	name.Name = fmt.Sprintf("%s-controlplane", name.Name)
	return c.Get(ctx, name, &dockerv1beta1.DockerMachineTemplate{})
}

func (kubeadmdocker) GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &dockerv1beta1.DockerClusterTemplate{})
}

func (kubeadmdocker) AlterClusterClass(cc *capiv1beta1.ClusterClass) {
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.Kind = KubeadmControlPlaneTemplate
	cc.Spec.ControlPlane.MachineInfrastructure.Ref.Kind = DockerMachineTemplate
	cc.Spec.Infrastructure.Ref.Kind = DockerClusterTemplate

	imageVariable := "dockerKindImage"

	cc.Spec.Variables = []capiv1beta1.ClusterClassVariable{
		{
			Name:     imageVariable,
			Required: false,
			Schema: capiv1beta1.VariableSchema{
				OpenAPIV3Schema: capiv1beta1.JSONSchemaProps{
					Type: "string",
				},
			},
		},
	}

	cc.Spec.Patches = []capiv1beta1.ClusterClassPatch{
		{
			Definitions: []capiv1beta1.PatchDefinition{
				{
					JSONPatches: []capiv1beta1.JSONPatch{
						{
							Op:   "add",
							Path: "/spec/template/spec/customImage",
							ValueFrom: &capiv1beta1.JSONPatchValue{
								Variable: &imageVariable,
							},
						},
					},
					Selector: capiv1beta1.PatchSelector{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Kind:       "DockerMachineTemplate",
						MatchResources: capiv1beta1.PatchSelectorMatch{
							ControlPlane: true,
						},
					},
				},
			},
			Description: "Sets the container image that is used for running dockerMachines for the controlPlane.",
			Name:        imageVariable,
		},
	}
}
