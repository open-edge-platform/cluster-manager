// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controlplaneprovider

import (
	"context"
	"encoding/json"
	"fmt"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmcpv1beta1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubeadmintel struct{}

// AlterClusterClass implements Provider.
func (k kubeadmintel) AlterClusterClass(cc *v1beta1.ClusterClass) {
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.Kind = KubeadmControlPlaneTemplate

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
						Kind:       KubeadmControlPlaneTemplate,
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

// CreatePrerequisites ensures that any prerequisites required by the kubeadmintel provider are created.
// For this provider, no prerequisites are required, so this method does nothing.
func (k kubeadmintel) CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

// CreateControlPlaneTemplate creates a new KubeadmControlPlaneTemplate.
func (k kubeadmintel) CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error {
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

// CreateControlPlaneMachineTemplate creates a new IntelMachineTemplate with the name <name>-controlplane.
func (k kubeadmintel) CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
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

// CreateClusterTemplate creates IntelClusterTemplate for the kubeadmintel provider.
func (k kubeadmintel) CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
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

// DeletePrerequisites deletes the prerequisites for the kubeadmintel provider.
// For this provider, no prerequisites are required, so this method does nothing.
func (k kubeadmintel) DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

// GetPrerequisites gets the prerequisites for the kubeadmintel provider.
// For this provider, no prerequisites are required, so this method does nothing.
func (k kubeadmintel) GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

// GetControlPlaneTemplate gets KubeadmControlPlaneTemplate for the kubeadmintel provider.
func (k kubeadmintel) GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &kubeadmcpv1beta1.KubeadmControlPlaneTemplate{})
}

// GetControlPlaneMachineTemplate gets IntelMachineTemplate for the kubeadmintel provider.
func (k kubeadmintel) GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, types.NamespacedName{Name: name.Name + "-controlplane", Namespace: name.Namespace}, &intelv1alpha1.IntelMachineTemplate{})
}

// GetClusterTemplate gets IntelClusterTemplate for the kubeadmintel provider.
func (k kubeadmintel) GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &intelv1alpha1.IntelClusterTemplate{})
}
