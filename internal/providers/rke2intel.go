// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controlplaneprovider

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	rke2cpv1beta1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
)

var (
	connectAgentManifest = "connectAgentManifest"
	enabledIf            = "{{ if .connectAgentManifest.path }}true{{ end }}"
)

type rke2intel struct {
}

func (rke2intel) AlterClusterClass(cc *capiv1beta1.ClusterClass) {
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.Kind = RKE2ControlPlaneTemplate

	cc.Spec.ControlPlane.MachineInfrastructure.Ref.APIVersion = "infrastructure.cluster.x-k8s.io/v1alpha1"
	cc.Spec.ControlPlane.MachineInfrastructure.Ref.Kind = IntelMachineTemplate

	cc.Spec.Infrastructure.Ref.APIVersion = "infrastructure.cluster.x-k8s.io/v1alpha1"
	cc.Spec.Infrastructure.Ref.Kind = IntelClusterTemplate

	cc.Spec.Variables = []capiv1beta1.ClusterClassVariable{
		{
			Name: connectAgentManifest,
			Schema: capiv1beta1.VariableSchema{
				OpenAPIV3Schema: capiv1beta1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]capiv1beta1.JSONSchemaProps{
						"path": {
							Type: "string",
						},
						"content": {
							Type: "string",
						},
						"owner": {
							Type: "string",
						},
					},
				},
			},
		},
	}

	cc.Spec.Patches = []capiv1beta1.ClusterClassPatch{
		{
			Name: "connect-agent-manifest",
			Description: "This patch will add connect-agent manifest " +
				"injected by Cluster Connect Gateway.",
			EnabledIf: &enabledIf,
			Definitions: []capiv1beta1.PatchDefinition{
				{
					Selector: capiv1beta1.PatchSelector{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
						Kind:       RKE2ControlPlaneTemplate,
						MatchResources: capiv1beta1.PatchSelectorMatch{
							ControlPlane: true,
						},
					},
					JSONPatches: []capiv1beta1.JSONPatch{
						{
							// This patch assumes something is already at .Files array.
							// If not (like in vanilla baseline template), we'll need a different patch
							Op:   "add",
							Path: "/spec/template/spec/files/-",
							ValueFrom: &capiv1beta1.JSONPatchValue{
								Variable: &connectAgentManifest,
							},
						},
					},
				},
			},
		},
	}
}

func (rke2intel) CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (rke2intel) CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error {
	var cpt rke2cpv1beta1.RKE2ControlPlaneTemplate
	if err := json.Unmarshal([]byte(config), &cpt); err != nil {
		return fmt.Errorf("failed to unmarshal control plane template: %w", err)
	}

	cpt.ObjectMeta = metav1.ObjectMeta{
		Name:      name.Name,
		Namespace: name.Namespace,
	}

	cpt.Spec.Template.Spec.InfrastructureRef = corev1.ObjectReference{
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
		Kind:       IntelMachineTemplate,
		Name:       fmt.Sprintf("%s-controlplane", name.Name),
	}

	return c.Create(ctx, &cpt)
}

func (rke2intel) CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	cpmt := intelv1alpha1.IntelMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-controlplane", name.Name),
			Namespace: name.Namespace,
		},
		Spec: intelv1alpha1.IntelMachineTemplateSpec{
			Template: intelv1alpha1.IntelMachineTemplateSpecTemplate{},
		},
	}
	return c.Create(ctx, &cpmt)
}

func (rke2intel) CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	uct := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": intelv1alpha1.GroupVersion.String(),
			"kind":       "IntelClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      name.Name,
				"namespace": name.Namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{},
					"spec":     map[string]interface{}{},
				},
			},
		},
	}
	return c.Create(ctx, &uct)
}

func (rke2intel) DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (rke2intel) GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (rke2intel) GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &rke2cpv1beta1.RKE2ControlPlaneTemplate{})
}

func (rke2intel) GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, types.NamespacedName{Name: name.Name + "-controlplane", Namespace: name.Namespace}, &intelv1alpha1.IntelMachineTemplate{})
}

func (rke2intel) GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &intelv1alpha1.IntelClusterTemplate{})
}
