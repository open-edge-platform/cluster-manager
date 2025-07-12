// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controlplaneprovider

import (
	"context"
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	kthreescpv1beta2 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	dockerv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type k3sdocker struct {
}

func (k3sdocker) CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (k3sdocker) CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error {
	var cpt kthreescpv1beta2.KThreesControlPlaneTemplate
	if err := json.Unmarshal([]byte(config), &cpt); err != nil {
		return fmt.Errorf("failed to unmarshal control plane template: %w", err)
	}

	cpt.ObjectMeta = metav1.ObjectMeta{
		Name:      name.Name,
		Namespace: name.Namespace,
	}

	cpt.Spec.Template.Spec.MachineTemplate.InfrastructureRef = corev1.ObjectReference{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
		Kind:       DockerMachineTemplate,
		Name:       fmt.Sprintf("%s-controlplane", name.Name),
	}

	return c.Create(ctx, &cpt)
}

func (rd k3sdocker) CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	cpmt := dockerv1beta1.DockerMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-controlplane", name.Name),
			Namespace: name.Namespace,
		},
		Spec: dockerv1beta1.DockerMachineTemplateSpec{
			Template: dockerv1beta1.DockerMachineTemplateResource{
				Spec: dockerv1beta1.DockerMachineSpec{
					CustomImage: "kindest/node:v1.30.3-custom",
					ExtraMounts: []dockerv1beta1.Mount{
						{ContainerPath: "/var/run/docker.sock", HostPath: "/var/run/docker.sock"},
					},
				},
			},
		},
	}
	return c.Create(ctx, &cpmt)
}

func (rd k3sdocker) CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
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

func (k3sdocker) DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	cm := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-k3s-class-lb-config", name.Name), Namespace: name.Namespace}, cm)
	switch {
	// ignore error if ConfigMap is not found (already deleted)
	case err != nil && errors.IsNotFound(err):
		return nil
	case err != nil:
		return err
	default:
		return c.Delete(ctx, cm)
	}
}

func (k3sdocker) GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (rd k3sdocker) GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &kthreescpv1beta2.KThreesControlPlaneTemplate{})
}

func (kd k3sdocker) GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, types.NamespacedName{Name: name.Name + "-controlplane", Namespace: name.Namespace}, &dockerv1beta1.DockerMachineTemplate{})

}

func (kd k3sdocker) GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &dockerv1beta1.DockerClusterTemplate{})
}

func (rd k3sdocker) AlterClusterClass(cc *capiv1beta1.ClusterClass) {
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.APIVersion = "controlplane.cluster.x-k8s.io/v1beta2"
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.Kind = KThreesControlPlaneTemplate

	cc.Spec.ControlPlane.MachineInfrastructure.Ref.Kind = DockerMachineTemplate
	cc.Spec.Infrastructure.Ref.Kind = DockerClusterTemplate

	cc.Spec.Variables = []capiv1beta1.ClusterClassVariable{
		{
			Name: AirGapped,
			Schema: capiv1beta1.VariableSchema{
				OpenAPIV3Schema: capiv1beta1.JSONSchemaProps{
					Type: "boolean",
					Default: &apiextensionsv1.JSON{
						Raw: []byte("false"),
					},
				},
			},
		},
	}

	cc.Spec.Patches = []capiv1beta1.ClusterClassPatch{
		{
			Name:        "airGapped",
			Description: "This patch will disable air-gapped configuration ",
			Definitions: []capiv1beta1.PatchDefinition{
				{
					Selector: capiv1beta1.PatchSelector{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta2",
						Kind:       KThreesControlPlaneTemplate,
						MatchResources: capiv1beta1.PatchSelectorMatch{
							ControlPlane: true,
						},
					},
					JSONPatches: []capiv1beta1.JSONPatch{
						{
							Op:   "replace",
							Path: "/spec/template/spec/kthreesConfigSpec/agentConfig/airGapped",
							Value: &apiextensionsv1.JSON{
								Raw: []byte("false"),
							},
						},
					},
				},
			},
		},
	}
}
