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
	kthreesbootstrapv1beta2 "github.com/k3s-io/cluster-api-k3s/bootstrap/api/v1beta2"
	kthreescpv1beta2 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type k3sintel struct {
}

func (k3sintel) AlterClusterClass(cc *capiv1beta1.ClusterClass) {
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.APIVersion = "controlplane.cluster.x-k8s.io/v1beta2"
	cc.Spec.ControlPlane.LocalObjectTemplate.Ref.Kind = KThreesControlPlaneTemplate

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
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta2",
						Kind:       KThreesControlPlaneTemplate,
						MatchResources: capiv1beta1.PatchSelectorMatch{
							ControlPlane: true,
						},
					},
					JSONPatches: []capiv1beta1.JSONPatch{
						{
							// This patch assumes something is already at .Files array.
							// If not (like in vanilla baseline template), we'll need a different patch
							Op:   "add",
							Path: "/spec/template/spec/kthreesConfigSpec/files/-",
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

func (k3sintel) CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (k3sintel) CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error {
	var cpt kthreescpv1beta2.KThreesControlPlaneTemplate
	if err := json.Unmarshal([]byte(config), &cpt); err != nil {
		return fmt.Errorf("failed to unmarshal control plane template: %w", err)
	}

	cpt.ObjectMeta = metav1.ObjectMeta{
		Name:      name.Name,
		Namespace: name.Namespace,
	}

	cpt.Spec.Template.Spec.KThreesConfigSpec = kthreesbootstrapv1beta2.KThreesConfigSpec{
		Version: "v1.32.4+k3s1", 									// TODO: make configurable from the template
		ServerConfig: kthreesbootstrapv1beta2.KThreesServerConfig{ 	// TODO: make configurable from the template
			TLSSan:                                 []string{"0.0.0.0"},
			ClusterDomain:                          "cluster.edge",
			DisableCloudController:                 func(b bool) *bool { return &b }(false),
		},
	}

	cpt.Spec.Template.Spec.MachineTemplate = kthreescpv1beta2.KThreesControlPlaneMachineTemplate{
		InfrastructureRef: corev1.ObjectReference{
			APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
			Kind:       "IntelMachineTemplate",
			Name:       fmt.Sprintf("%s-controlplane", name.Name),
			Namespace:  name.Namespace,
		},
	}

	if err := c.Create(ctx, &cpt); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create KThreesControlPlaneTemplate: %w", err)
	}
	return nil
}

func (k3sintel) CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
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

func (k3sintel) CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
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

func (k3sintel) DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (k3sintel) GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return nil
}

func (k3sintel) GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &kthreescpv1beta2.KThreesControlPlane{})
}

func (k3sintel) GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, types.NamespacedName{Name: name.Name + "-controlplane", Namespace: name.Namespace}, &intelv1alpha1.IntelMachineTemplate{})
}

func (k3sintel) GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return c.Get(ctx, name, &intelv1alpha1.IntelClusterTemplate{})
}
