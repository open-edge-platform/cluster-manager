// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"fmt"

	intelProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	dockerProvider "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

// mockClientAdapter wraps a MockInterface to implement k8s.Client interface for tests
type mockClientAdapter struct {
	*k8s.MockInterface
}

func (m *mockClientAdapter) Dynamic() dynamic.Interface {
	return m.MockInterface
}

// Implement the required Client interface methods with no-op or basic implementations for tests
func (m *mockClientAdapter) StartInformers(ctx context.Context, resources []schema.GroupVersionResource) error {
	return nil
}
func (m *mockClientAdapter) GetCached(ctx context.Context, resourceSchema schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	// Delegate to the dynamic client Resource() method for testing
	return m.MockInterface.Resource(resourceSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}
func (m *mockClientAdapter) ListCached(ctx context.Context, resourceSchema schema.GroupVersionResource, namespace string, listOptions metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	// Delegate to the dynamic client Resource() method for testing
	return m.MockInterface.Resource(resourceSchema).Namespace(namespace).List(ctx, listOptions)
}
func (m *mockClientAdapter) CreateNamespace(ctx context.Context, name string) error      { return nil }
func (m *mockClientAdapter) DeleteNamespace(ctx context.Context, namespace string) error { return nil }
func (m *mockClientAdapter) CreateTemplate(ctx context.Context, namespace string, template *v1alpha1.ClusterTemplate) error {
	return nil
}
func (m *mockClientAdapter) CreateCluster(ctx context.Context, namespace string, cluster capi.Cluster) (string, error) {
	// Convert cluster to unstructured and delegate to mock interface
	unstructuredCluster, err := convert.ToUnstructured(cluster)
	if err != nil {
		return "", err
	}
	_, err = m.MockInterface.Resource(core.ClusterResourceSchema).Namespace(namespace).Create(ctx, unstructuredCluster, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return cluster.Name, nil
}
func (m *mockClientAdapter) DeleteClusters(ctx context.Context, namespace string) error  { return nil }
func (m *mockClientAdapter) DeleteTemplates(ctx context.Context, namespace string) error { return nil }
func (m *mockClientAdapter) CreateClusterLabels(ctx context.Context, namespace string, clusterName string, newLabels map[string]string) error {
	return nil
}
func (m *mockClientAdapter) CreateTemplateLabels(ctx context.Context, namespace string, templateName string, newLabels map[string]string) error {
	return nil
}
func (m *mockClientAdapter) DefaultTemplate(ctx context.Context, namespace string) (v1alpha1.ClusterTemplate, error) {
	// Get templates with default label - use same constants as real implementation
	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%v=%v", labels.DefaultLabelKey, labels.DefaultLabelVal),
	}
	unstructuredList, err := m.ListCached(ctx, core.TemplateResourceSchema, namespace, listOptions)
	if err != nil {
		return v1alpha1.ClusterTemplate{}, err
	}
	if len(unstructuredList.Items) > 1 {
		return v1alpha1.ClusterTemplate{}, fmt.Errorf("multiple default templates found")
	}
	if len(unstructuredList.Items) == 0 {
		return v1alpha1.ClusterTemplate{}, k8s.ErrDefaultTemplateNotFound
	}
	// Return the default template found
	var template v1alpha1.ClusterTemplate
	err = convert.FromUnstructured(unstructuredList.Items[0], &template)
	if err != nil {
		return v1alpha1.ClusterTemplate{}, err
	}
	return template, nil
}
func (m *mockClientAdapter) Templates(ctx context.Context, namespace string) ([]v1alpha1.ClusterTemplate, error) {
	// Delegate to ListCached to use the mock setup
	unstructuredList, err := m.ListCached(ctx, core.TemplateResourceSchema, namespace, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var templates []v1alpha1.ClusterTemplate
	for _, item := range unstructuredList.Items {
		var template v1alpha1.ClusterTemplate
		err = convert.FromUnstructured(item, &template)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, nil
}
func (m *mockClientAdapter) Template(ctx context.Context, namespace, name string) (v1alpha1.ClusterTemplate, error) {
	// Delegate to GetCached to use the mock setup
	unstructuredTemplate, err := m.GetCached(ctx, core.TemplateResourceSchema, namespace, name)
	if err != nil {
		return v1alpha1.ClusterTemplate{}, err
	}
	var template v1alpha1.ClusterTemplate
	err = convert.FromUnstructured(*unstructuredTemplate, &template)
	if err != nil {
		return v1alpha1.ClusterTemplate{}, err
	}
	return template, nil
}
func (m *mockClientAdapter) GetCluster(ctx context.Context, namespace, name string) (*capi.Cluster, error) {
	// Delegate to GetCached to use the mock setup
	unstructuredCluster, err := m.GetCached(ctx, core.ClusterResourceSchema, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, k8s.ErrClusterNotFound
		}
		return nil, err
	}
	var cluster capi.Cluster
	err = convert.FromUnstructured(*unstructuredCluster, &cluster)
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}
func (m *mockClientAdapter) GetMachines(ctx context.Context, namespace, clusterName string) ([]capi.Machine, error) {
	// Delegate to ListCached to use the mock setup
	listOptions := metav1.ListOptions{
		LabelSelector: "cluster.x-k8s.io/cluster-name=" + clusterName,
	}
	unstructuredList, err := m.ListCached(ctx, core.MachineResourceSchema, namespace, listOptions)
	if err != nil {
		return nil, err
	}
	var machines []capi.Machine
	for _, item := range unstructuredList.Items {
		var machine capi.Machine
		err = convert.FromUnstructured(item, &machine)
		if err != nil {
			return nil, err
		}
		machines = append(machines, machine)
	}
	return machines, nil
}
func (m *mockClientAdapter) CreateMachineBinding(ctx context.Context, namespace string, binding intelProvider.IntelMachineBinding) error {
	// Convert binding to unstructured and delegate to mock interface
	unstructuredBinding, err := convert.ToUnstructured(binding)
	if err != nil {
		return err
	}
	// Use the same binding schema as used in the k8s client
	bindingSchema := schema.GroupVersionResource{
		Group:    intelProvider.GroupVersion.Group,
		Version:  intelProvider.GroupVersion.Version,
		Resource: "intelmachinebindings",
	}
	_, err = m.MockInterface.Resource(bindingSchema).Namespace(namespace).Create(ctx, unstructuredBinding, metav1.CreateOptions{})
	return err
}
func (m *mockClientAdapter) IntelMachines(ctx context.Context, namespace, clusterName string) ([]intelProvider.IntelMachine, error) {
	// Delegate to ListCached to use the mock setup
	listOptions := metav1.ListOptions{
		LabelSelector: "cluster.x-k8s.io/cluster-name=" + clusterName,
	}
	unstructuredList, err := m.ListCached(ctx, k8s.IntelMachineResourceSchema, namespace, listOptions)
	if err != nil {
		return nil, err
	}
	var machines []intelProvider.IntelMachine
	for _, item := range unstructuredList.Items {
		var machine intelProvider.IntelMachine
		err = convert.FromUnstructured(item, &machine)
		if err != nil {
			return nil, err
		}
		machines = append(machines, machine)
	}
	return machines, nil
}
func (m *mockClientAdapter) DockerMachines(ctx context.Context, namespace, clusterName string) ([]dockerProvider.DockerMachine, error) {
	// Delegate to ListCached to use the mock setup
	listOptions := metav1.ListOptions{
		LabelSelector: "cluster.x-k8s.io/cluster-name=" + clusterName,
	}
	unstructuredList, err := m.ListCached(ctx, k8s.DockerMachineResourceSchema, namespace, listOptions)
	if err != nil {
		return nil, err
	}
	var machines []dockerProvider.DockerMachine
	for _, item := range unstructuredList.Items {
		var machine dockerProvider.DockerMachine
		err = convert.FromUnstructured(item, &machine)
		if err != nil {
			return nil, err
		}
		machines = append(machines, machine)
	}
	return machines, nil
}
func (m *mockClientAdapter) IntelMachine(ctx context.Context, namespace, providerMachineName string) (intelProvider.IntelMachine, error) {
	// Delegate to GetCached to use the mock setup
	unstructuredMachine, err := m.GetCached(ctx, k8s.IntelMachineResourceSchema, namespace, providerMachineName)
	if err != nil {
		return intelProvider.IntelMachine{}, err
	}
	var machine intelProvider.IntelMachine
	err = convert.FromUnstructured(*unstructuredMachine, &machine)
	if err != nil {
		return intelProvider.IntelMachine{}, err
	}
	return machine, nil
}
func (m *mockClientAdapter) DockerMachine(ctx context.Context, namespace, providerMachineName string) (dockerProvider.DockerMachine, error) {
	// Delegate to GetCached to use the mock setup
	unstructuredMachine, err := m.GetCached(ctx, k8s.DockerMachineResourceSchema, namespace, providerMachineName)
	if err != nil {
		return dockerProvider.DockerMachine{}, err
	}
	var machine dockerProvider.DockerMachine
	err = convert.FromUnstructured(*unstructuredMachine, &machine)
	if err != nil {
		return dockerProvider.DockerMachine{}, err
	}
	return machine, nil
}
func (m *mockClientAdapter) GetMachineByHostID(ctx context.Context, namespace, hostID string) (capi.Machine, error) {
	// This method typically searches for machines by annotation or label, but for testing we'll use a simple approach
	listOptions := metav1.ListOptions{}
	unstructuredList, err := m.ListCached(ctx, core.MachineResourceSchema, namespace, listOptions)
	if err != nil {
		return capi.Machine{}, err
	}
	// Find machine with matching host ID in annotations
	for _, item := range unstructuredList.Items {
		annotations, found, _ := unstructured.NestedMap(item.Object, "metadata", "annotations")
		if found {
			if hostAnnotation, exists := annotations["intelmachine.infrastructure.cluster.x-k8s.io/host-id"]; exists {
				if hostAnnotation == hostID {
					var machine capi.Machine
					err = convert.FromUnstructured(item, &machine)
					if err != nil {
						return capi.Machine{}, err
					}
					return machine, nil
				}
			}
		}
	}
	return capi.Machine{}, nil
}
func (m *mockClientAdapter) DeleteCluster(ctx context.Context, namespace string, clusterName string) error {
	return nil
}
func (m *mockClientAdapter) SetMachineLabels(ctx context.Context, namespace string, machineName string, newUserLabels map[string]string) error {
	return nil
}
func (m *mockClientAdapter) WithInClusterConfig() *k8s.ManagerClient { return nil }
func (m *mockClientAdapter) CreateSecret(ctx context.Context, namespace string, name string, data map[string][]byte) error {
	return nil
}
func (m *mockClientAdapter) RemoveTemplateLabels(ctx context.Context, namespace string, templateName string, labelKeys ...string) error {
	return nil
}
func (m *mockClientAdapter) AddTemplateLabels(ctx context.Context, namespace string, templateName string, newLabels map[string]string) error {
	return nil
}
func (m *mockClientAdapter) HasTemplate(ctx context.Context, namespace, templateName string) bool {
	return false
}
func (m *mockClientAdapter) GetClusterTemplate(ctx context.Context, namespace, templateName string) (*v1alpha1.ClusterTemplate, error) {
	// Delegate to GetCached to use the mock setup
	unstructuredTemplate, err := m.GetCached(ctx, core.TemplateResourceSchema, namespace, templateName)
	if err != nil {
		return nil, err
	}
	var template v1alpha1.ClusterTemplate
	err = convert.FromUnstructured(*unstructuredTemplate, &template)
	if err != nil {
		return nil, err
	}
	return &template, nil
}
func (m *mockClientAdapter) SetClusterLabels(ctx context.Context, namespace string, clusterName string, newUserLabels map[string]string) error {
	if newUserLabels == nil {
		return nil
	}

	// Get the current cluster object
	unstructuredCluster, err := m.GetCached(ctx, core.ClusterResourceSchema, namespace, clusterName)
	if err != nil {
		return err
	}

	// Modify the labels (simplified version of the real logic)
	newLabels := make(map[string]string)
	// Copy existing system labels
	for k, v := range unstructuredCluster.GetLabels() {
		// Keep system labels (simplified)
		if k != "user.edge-orchestrator.intel.com/key1" {
			newLabels[k] = v
		}
	}
	// Add new user labels
	for k, v := range newUserLabels {
		newLabels[k] = v
	}
	unstructuredCluster.SetLabels(newLabels)

	// Update the cluster
	_, err = m.MockInterface.Resource(core.ClusterResourceSchema).Namespace(namespace).Update(ctx, unstructuredCluster, metav1.UpdateOptions{})
	return err
}

// wrapMockInterface wraps a MockInterface to implement the k8s.Client interface
func wrapMockInterface(mock *k8s.MockInterface) k8s.Client {
	return &mockClientAdapter{MockInterface: mock}
}
