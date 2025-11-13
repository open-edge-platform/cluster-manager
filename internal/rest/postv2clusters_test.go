// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func TestPostV2Clusters201(t *testing.T) {

	t.Run("Create Cluster", func(t *testing.T) {
		// Prepare test data
		expectedCluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "cluster.x-k8s.io/v1beta1",
				Kind:       "clusters",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-cluster",
				Namespace: "655a6892-4280-4c37-97b1-31161ac0b99e",
				Labels: map[string]string{
					"edge-orchestrator.intel.com/clustername": "example-cluster",
					"edge-orchestrator.intel.com/project-id":  "655a6892-4280-4c37-97b1-31161ac0b99e",
					"prometheusMetricsURL":                    "metrics-node.kind.internal",
					"trusted-compute-compatible":              "false",
					"default-extension":                       "baseline",
					"test":                                    "true",
				},
				Annotations: map[string]string{
					"edge-orchestrator.intel.com/template": "baseline-kubeadm",
				},
			},
			Spec: capi.ClusterSpec{
				Paused: true,
				ClusterNetwork: &capi.ClusterNetwork{
					Pods: &capi.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
					Services: &capi.NetworkRanges{
						CIDRBlocks: []string{},
					},
				},
				Topology: &capi.Topology{
					Class:   "example-cluster-class",
					Version: "v1.30.0",
					ControlPlane: capi.ControlPlaneTopology{
						Replicas: ptr(int32(1)),
					},
					Variables: []capi.ClusterVariable{},
				},
			},
		}
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-kubeadm"

		// Convert expected cluster to unstructured
		unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedCluster)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")

		// Mock the template fetching
		expectedTemplate := clusterv1alpha1.ClusterTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedTemplateName,
			},
			Spec: clusterv1alpha1.ClusterTemplateSpec{
				ControlPlaneProviderType: "kubeadm",
				InfraProviderType:        "docker",
				KubernetesVersion:        "v1.30.0",
				ClusterConfiguration:     "",
				ClusterNetwork: clusterv1alpha1.ClusterNetwork{
					Services: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
				},
				ClusterLabels: map[string]string{"default-extension": "baseline"},
			},
			Status: clusterv1alpha1.ClusterTemplateStatus{
				Ready: true,
				ClusterClassRef: &corev1.ObjectReference{
					Name: "example-cluster-class",
				},
			},
		}
		unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
		require.NoError(t, err, "failed to convert template to unstructured")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock resource interface for clusters
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Create(mock.Anything, &unstructured.Unstructured{Object: unstructuredCluster}, metav1.CreateOptions{}).Return(&unstructured.Unstructured{Object: unstructuredCluster}, nil)

		// Create a mock namespaceable resource interface for clusters
		nsClusterResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsClusterResource.EXPECT().Namespace(expectedActiveProjectID).Return(clusterResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsClusterResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   "27b4e138-ea0b-11ef-8552-8b663d95bc01",
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{"test": "true"},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusCreated, rr.Code)
		expectedResponse := fmt.Sprintf("successfully created cluster %s", "example-cluster")
		assert.Contains(t, rr.Body.String(), expectedResponse)
	})
	t.Run("Create RKE2 Cluster and IntelMachineBindings", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-rke2"
		expectedIntelMachineTemplateName := fmt.Sprintf("%s-controlplane", expectedTemplateName)
		expectedNodeid := "27b4e138-ea0b-11ef-8552-8b663d95bc01"
		expectedClusterName := "example-cluster"
		expectedBindingName := fmt.Sprintf("%s-%s", expectedClusterName, expectedNodeid)

		expectedCluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "cluster.x-k8s.io/v1beta1",
				Kind:       "clusters",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedClusterName,
				Namespace: expectedActiveProjectID,
				Labels: map[string]string{
					"edge-orchestrator.intel.com/clustername": "example-cluster",
					"edge-orchestrator.intel.com/project-id":  "655a6892-4280-4c37-97b1-31161ac0b99e",
					"prometheusMetricsURL":                    "metrics-node.kind.internal",
					"trusted-compute-compatible":              "false",
					"default-extension":                       "privileged",
					"test2":                                   "true",
				},
				Annotations: map[string]string{
					"edge-orchestrator.intel.com/template": "baseline-rke2",
				},
			},
			Spec: capi.ClusterSpec{
				Paused: true,
				ClusterNetwork: &capi.ClusterNetwork{
					Pods: &capi.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
					Services: &capi.NetworkRanges{
						CIDRBlocks: []string{},
					},
				},
				Topology: &capi.Topology{
					Class:   "example-cluster-class",
					Version: "v1.30.6+rke2r1",
					ControlPlane: capi.ControlPlaneTopology{
						Replicas: ptr(int32(1)),
					},
					Variables: []capi.ClusterVariable{},
				},
			},
		}

		// Convert expected cluster to unstructured
		unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedCluster)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")

		expectedBinding := intelv1alpha1.IntelMachineBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
				Kind:       "IntelMachineBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedBindingName,
				Namespace: expectedActiveProjectID,
			},
			Spec: intelv1alpha1.IntelMachineBindingSpec{
				NodeGUID:                 expectedNodeid,
				ClusterName:              expectedClusterName,
				IntelMachineTemplateName: expectedIntelMachineTemplateName,
			},
		}

		// Convert expected binding to unstructured
		unstructuredBinding, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedBinding)
		require.NoError(t, err, "convertBindingToUnstructured() error = %v, want nil")

		// Mock the template fetching
		expectedTemplate := clusterv1alpha1.ClusterTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedTemplateName,
			},
			Spec: clusterv1alpha1.ClusterTemplateSpec{
				ControlPlaneProviderType: "rke2",
				InfraProviderType:        "intel",
				KubernetesVersion:        "v1.30.6+rke2r1",
				ClusterConfiguration:     "",
				ClusterNetwork: clusterv1alpha1.ClusterNetwork{
					Services: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
				},
				ClusterLabels: map[string]string{"default-extension": "privileged"},
			},
			Status: clusterv1alpha1.ClusterTemplateStatus{
				Ready: true,
				ClusterClassRef: &corev1.ObjectReference{
					Name: "example-cluster-class",
				},
			},
		}
		unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
		require.NoError(t, err, "failed to convert template to unstructured")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock resource interface for clusters
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Create(mock.Anything, &unstructured.Unstructured{Object: unstructuredCluster}, metav1.CreateOptions{}).Return(&unstructured.Unstructured{Object: unstructuredCluster}, nil)

		// Create a mock resource interface for bindings
		bindingResource := k8s.NewMockResourceInterface(t)
		bindingResource.EXPECT().Create(mock.Anything, &unstructured.Unstructured{Object: unstructuredBinding}, metav1.CreateOptions{}).Return(&unstructured.Unstructured{Object: unstructuredBinding}, nil)

		// Create a mock namespaceable resource interface for clusters
		nsClusterResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsClusterResource.EXPECT().Namespace(expectedActiveProjectID).Return(clusterResource)

		// Create a mock namespaceable resource interface for clusters
		nsBindingResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsBindingResource.EXPECT().Namespace(expectedActiveProjectID).Return(bindingResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsClusterResource)
		mockedk8sclient.EXPECT().Resource(core.BindingsResourceSchema).Return(nsBindingResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   expectedNodeid,
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{"test2": "true"},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusCreated, rr.Code)
		expectedResponse := fmt.Sprintf("successfully created cluster %s", "example-cluster")
		assert.Contains(t, rr.Body.String(), expectedResponse)
	})
}

func TestPostV2Clusters201K3sAirGap(t *testing.T) {

	t.Run("Create k3s Cluster in AirGap Mode", func(t *testing.T) {
		// Prepare test data
		expectedCluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "cluster.x-k8s.io/v1beta1",
				Kind:       "clusters",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-cluster",
				Namespace: "655a6892-4280-4c37-97b1-31161ac0b99e",
				Labels: map[string]string{
					"edge-orchestrator.intel.com/clustername": "example-cluster",
					"edge-orchestrator.intel.com/project-id":  "655a6892-4280-4c37-97b1-31161ac0b99e",
					"prometheusMetricsURL":                    "metrics-node.kind.internal",
					"trusted-compute-compatible":              "false",
					"default-extension":                       "baseline",
					"test":                                    "true",
				},
				Annotations: map[string]string{
					"edge-orchestrator.intel.com/template": "baseline-k3s",
				},
			},
			Spec: capi.ClusterSpec{
				Paused: true,
				ClusterNetwork: &capi.ClusterNetwork{
					Pods: &capi.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
					Services: &capi.NetworkRanges{
						CIDRBlocks: []string{},
					},
				},
				Topology: &capi.Topology{
					Class:   "example-cluster-class",
					Version: "v1.30.0",
					ControlPlane: capi.ControlPlaneTopology{
						Replicas: ptr(int32(1)),
					},
					Variables: []capi.ClusterVariable{},
				},
			},
		}
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-k3s"

		// Convert expected cluster to unstructured
		unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedCluster)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")

		// Mock the template fetching
		expectedTemplate := clusterv1alpha1.ClusterTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedTemplateName,
			},
			Spec: clusterv1alpha1.ClusterTemplateSpec{
				ControlPlaneProviderType: "k3s",
				InfraProviderType:        "docker",
				KubernetesVersion:        "v1.30.0",
				ClusterConfiguration:     "",
				ClusterNetwork: clusterv1alpha1.ClusterNetwork{
					Services: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
				},
				ClusterLabels: map[string]string{"default-extension": "baseline"},
			},
			Status: clusterv1alpha1.ClusterTemplateStatus{
				Ready: true,
				ClusterClassRef: &corev1.ObjectReference{
					Name: "example-cluster-class",
				},
			},
		}
		unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
		require.NoError(t, err, "failed to convert template to unstructured")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock resource interface for clusters
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Create(mock.Anything, &unstructured.Unstructured{Object: unstructuredCluster}, metav1.CreateOptions{}).Return(&unstructured.Unstructured{Object: unstructuredCluster}, nil)

		// Create a mock namespaceable resource interface for clusters
		nsClusterResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsClusterResource.EXPECT().Namespace(expectedActiveProjectID).Return(clusterResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsClusterResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   "27b4e138-ea0b-11ef-8552-8b663d95bc01",
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{"test": "true"},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusCreated, rr.Code)
		expectedResponse := fmt.Sprintf("successfully created cluster %s", "example-cluster")
		assert.Contains(t, rr.Body.String(), expectedResponse)
	})
}

func TestPostV2Clusters201NoNameNoTemplate(t *testing.T) {
	expectedCluster := capi.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.x-k8s.io/v1beta1",
			Kind:       "clusters",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-cluster",
			Namespace: "655a6892-4280-4c37-97b1-31161ac0b99e",
			Labels: map[string]string{
				"edge-orchestrator.intel.com/clustername": "example-cluster",
				"edge-orchestrator.intel.com/project-id":  "655a6892-4280-4c37-97b1-31161ac0b99e",
				"prometheusMetricsURL":                    "metrics-node.kind.internal",
				"trusted-compute-compatible":              "false",
			},
			Annotations: map[string]string{
				"edge-orchestrator.intel.com/template": "baseline-kubeadm",
			},
		},
		Spec: capi.ClusterSpec{
			ClusterNetwork: &capi.ClusterNetwork{
				Pods: &capi.NetworkRanges{
					CIDRBlocks: []string{"10.0.0.0/16"},
				},
				Services: &capi.NetworkRanges{
					CIDRBlocks: []string{},
				},
			},
			Topology: &capi.Topology{
				Class:   "example-cluster-class",
				Version: "v1.30.0",
				ControlPlane: capi.ControlPlaneTopology{
					Replicas: ptr(int32(1)),
				},
				Variables: []capi.ClusterVariable{},
			},
		},
	}
	unstructuredCluster, err := convert.ToUnstructured(expectedCluster)
	require.NoError(t, err, "failed to convert cluster to unstructured")

	expectedTemplate := clusterv1alpha1.ClusterTemplate{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "baseline-kubeadm",
			Labels: map[string]string{
				"default":                    "true",
				"prometheusMetricsURL":       "metrics-node.kind.internal",
				"trusted-compute-compatible": "false",
			},
		},
		Spec: clusterv1alpha1.ClusterTemplateSpec{
			ControlPlaneProviderType: "kubeadm",
			InfraProviderType:        "docker",
			KubernetesVersion:        "v1.30.0",
			ClusterConfiguration:     "",
			ClusterNetwork: clusterv1alpha1.ClusterNetwork{
				Services: &clusterv1alpha1.NetworkRanges{
					CIDRBlocks: []string{},
				},
				Pods: &clusterv1alpha1.NetworkRanges{
					CIDRBlocks: []string{"10.0.0.0/16"},
				},
			},
			ClusterLabels: map[string]string{"default-extension": "baseline"},
		},
		Status: clusterv1alpha1.ClusterTemplateStatus{
			Ready: true,
			ClusterClassRef: &corev1.ObjectReference{
				Name: "example-cluster-class",
			},
		},
	}
	unstructuredTemplate, err := convert.ToUnstructured(expectedTemplate)
	unstructuredTemplateList := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*unstructuredTemplate}}

	require.NoError(t, err, "failed to convert template to unstructured")

	// Create k8s mock
	templateResource := k8s.NewMockResourceInterface(t)
	templateResource.EXPECT().List(mock.Anything, metav1.ListOptions{LabelSelector: "default=true"}).Return(unstructuredTemplateList, nil)
	templateResource.EXPECT().Get(mock.Anything, "baseline-kubeadm", metav1.GetOptions{}).Return(unstructuredTemplate, nil)
	nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)
	clusterResource := k8s.NewMockResourceInterface(t)
	clusterResource.EXPECT().Create(mock.Anything, mock.Anything, metav1.CreateOptions{}).Return(unstructuredCluster, nil)
	nsClusterResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsClusterResource.EXPECT().Namespace(expectedActiveProjectID).Return(clusterResource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)
	mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsClusterResource)

	// Create a server instance with the mock k8s client
	server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a new request & response recorder
	clusterSpec := api.ClusterSpec{
		//Name:     ptr("example-cluster"),
		//Template: ptr(expectedTemplate.ObjectMeta.Name),
		Nodes: []api.NodeSpec{
			{
				Id:   "27b4e138-ea0b-11ef-8552-8b663d95bc01",
				Role: api.NodeSpecRole(api.All),
			},
		},
		Labels: &map[string]string{},
	}
	requestBody, err := json.Marshal(clusterSpec)
	require.NoError(t, err, "Failed to marshal request body")
	req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
	req.Header.Set("Activeprojectid", expectedActiveProjectID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Handle request
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)
	handler.ServeHTTP(rr, req)

	// Check the response
	assert.Equal(t, http.StatusCreated, rr.Code)

}

func TestPostV2Clusters500(t *testing.T) {
	t.Run("Template Not Ready", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-kubeadm"

		// Mock the template fetching with Status.Ready = false
		expectedTemplate := clusterv1alpha1.ClusterTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedTemplateName,
			},
			Spec: clusterv1alpha1.ClusterTemplateSpec{
				ControlPlaneProviderType: "kubeadm",
				InfraProviderType:        "docker",
				KubernetesVersion:        "v1.30.0",
				ClusterConfiguration:     "",
				ClusterNetwork: clusterv1alpha1.ClusterNetwork{
					Services: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
				},
				ClusterLabels: map[string]string{"default-extension": "baseline"},
			},
			Status: clusterv1alpha1.ClusterTemplateStatus{
				Ready: false,
				ClusterClassRef: &corev1.ObjectReference{
					Name: "example-cluster-class",
				},
			},
		}
		unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
		require.NoError(t, err, "failed to convert template to unstructured")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   "27b4e138-ea0b-11ef-8552-8b663d95bc01",
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message":"failed to create cluster: template baseline-kubeadm is not ready"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})

	t.Run("ClusterClassRefNil", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-kubeadm"

		// Mock the template fetching with ClusterClassRef = nil
		expectedTemplate := clusterv1alpha1.ClusterTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedTemplateName,
			},
			Spec: clusterv1alpha1.ClusterTemplateSpec{
				ControlPlaneProviderType: "kubeadm",
				InfraProviderType:        "docker",
				KubernetesVersion:        "v1.30.0",
				ClusterConfiguration:     "",
				ClusterNetwork: clusterv1alpha1.ClusterNetwork{
					Services: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
				},
				ClusterLabels: map[string]string{"default-extension": "baseline"},
			},
			Status: clusterv1alpha1.ClusterTemplateStatus{
				Ready:           true,
				ClusterClassRef: nil,
			},
		}
		unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
		require.NoError(t, err, "failed to convert template to unstructured")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   "27b4e138-ea0b-11ef-8552-8b663d95bc01",
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message":"failed to create cluster: template baseline-kubeadm is not ready"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})

	t.Run("Failed to Get Cluster Template", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-kubeadm"

		// Mock the template fetching to return an error
		expectedError := fmt.Errorf("failed to get cluster template")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(nil, expectedError)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   "27b4e138-ea0b-11ef-8552-8b663d95bc01",
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message":"failed to create cluster: failed to get cluster template"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})

	t.Run("Failed to create Binding", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-rke2"
		expectedIntelMachineTemplateName := fmt.Sprintf("%s-controlplane", expectedTemplateName)
		expectedNodeid := "27b4e138-ea0b-11ef-8552-8b663d95bc01"
		expectedClusterName := "example-cluster"
		expectedBindingName := fmt.Sprintf("%s-%s", expectedClusterName, expectedNodeid)
		expectedError := k8serrors.StatusError{
			ErrStatus: metav1.Status{
				Status:  metav1.StatusFailure,
				Code:    http.StatusInternalServerError,
				Reason:  metav1.StatusReasonInternalError,
				Message: "Internal Server Error",
			},
		}

		expectedCluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "cluster.x-k8s.io/v1beta1",
				Kind:       "clusters",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedClusterName,
				Namespace: expectedActiveProjectID,
				Labels: map[string]string{
					"edge-orchestrator.intel.com/clustername": "example-cluster",
					"edge-orchestrator.intel.com/project-id":  "655a6892-4280-4c37-97b1-31161ac0b99e",
					"prometheusMetricsURL":                    "metrics-node.kind.internal",
					"trusted-compute-compatible":              "false",
					"default-extension":                       "restricted",
				},
				Annotations: map[string]string{
					"edge-orchestrator.intel.com/template": "baseline-rke2",
				},
			},
			Spec: capi.ClusterSpec{
				Paused: true,
				ClusterNetwork: &capi.ClusterNetwork{
					Pods: &capi.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
					Services: &capi.NetworkRanges{
						CIDRBlocks: []string{},
					},
				},
				Topology: &capi.Topology{
					Class:   "example-cluster-class",
					Version: "v1.30.6+rke2r1",
					ControlPlane: capi.ControlPlaneTopology{
						Replicas: ptr(int32(1)),
					},
					Variables: []capi.ClusterVariable{},
				},
			},
		}

		// Convert expected cluster to unstructured
		unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedCluster)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")

		expectedBinding := intelv1alpha1.IntelMachineBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
				Kind:       "IntelMachineBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedBindingName,
				Namespace: expectedActiveProjectID,
			},
			Spec: intelv1alpha1.IntelMachineBindingSpec{
				NodeGUID:                 expectedNodeid,
				ClusterName:              expectedClusterName,
				IntelMachineTemplateName: expectedIntelMachineTemplateName,
			},
		}

		// Convert expected binding to unstructured
		unstructuredBinding, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedBinding)
		require.NoError(t, err, "convertBindingToUnstructured() error = %v, want nil")

		// Mock the template fetching
		expectedTemplate := clusterv1alpha1.ClusterTemplate{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedTemplateName,
			},
			Spec: clusterv1alpha1.ClusterTemplateSpec{
				ControlPlaneProviderType: "rke2",
				InfraProviderType:        "intel",
				KubernetesVersion:        "v1.30.6+rke2r1",
				ClusterConfiguration:     "",
				ClusterNetwork: clusterv1alpha1.ClusterNetwork{
					Services: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: &clusterv1alpha1.NetworkRanges{
						CIDRBlocks: []string{"10.0.0.0/16"},
					},
				},
				ClusterLabels: map[string]string{"default-extension": "restricted"},
			},
			Status: clusterv1alpha1.ClusterTemplateStatus{
				Ready: true,
				ClusterClassRef: &corev1.ObjectReference{
					Name: "example-cluster-class",
				},
			},
		}
		unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
		require.NoError(t, err, "failed to convert template to unstructured")

		// Create a mock resource interface for clustertemplates
		templateResource := k8s.NewMockResourceInterface(t)
		templateResource.EXPECT().Get(mock.Anything, expectedTemplateName, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil)

		// Create a mock namespaceable resource interface for clustertemplates
		nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsTemplateResource.EXPECT().Namespace(expectedActiveProjectID).Return(templateResource)

		// Create a mock resource interface for clusters
		clusterResource := k8s.NewMockResourceInterface(t)
		clusterResource.EXPECT().Create(mock.Anything, &unstructured.Unstructured{Object: unstructuredCluster}, metav1.CreateOptions{}).Return(&unstructured.Unstructured{Object: unstructuredCluster}, nil)

		// Create a mock resource interface for bindings
		bindingResource := k8s.NewMockResourceInterface(t)
		bindingResource.EXPECT().Create(mock.Anything, &unstructured.Unstructured{Object: unstructuredBinding}, metav1.CreateOptions{}).Return(nil, &expectedError)

		// Create a mock namespaceable resource interface for clusters
		nsClusterResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsClusterResource.EXPECT().Namespace(expectedActiveProjectID).Return(clusterResource)

		// Create a mock namespaceable resource interface for clusters
		nsBindingResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsBindingResource.EXPECT().Namespace(expectedActiveProjectID).Return(bindingResource)

		// Create a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsClusterResource)
		mockedk8sclient.EXPECT().Resource(core.BindingsResourceSchema).Return(nsBindingResource)

		// Create a server instance with the mock k8s client
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes: []api.NodeSpec{
				{
					Id:   expectedNodeid,
					Role: api.NodeSpecRole(api.All),
				},
			},
			Labels: &map[string]string{},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := "Internal Server Error"
		assert.Contains(t, rr.Body.String(), expectedResponse)
	})
}

func TestPostV2Clusters400(t *testing.T) {
	t.Run("Invalid Project ID", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "00000000-0000-0000-0000-000000000000"
		expectedTemplateName := "baseline-kubeadm"

		// Create a server instance with a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		clusterSpec := api.ClusterSpec{
			Name:     ptr("example-cluster"),
			Template: ptr(expectedTemplateName),
			Nodes:    []api.NodeSpec{},
			Labels:   &map[string]string{},
		}
		requestBody, err := json.Marshal(clusterSpec)
		require.NoError(t, err, "Failed to marshal request body")
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader(requestBody))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		expectedResponse := `{"message":"no active project id provided"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})

	t.Run("Missing Fields", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		// Create a server instance with a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder with missing fields
		requestBody := `{"template": "baseline-kubeadm"}`
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader([]byte(requestBody)))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		expectedResponse := `{"message":"request body has an error: doesn't match schema #/components/schemas/ClusterSpec: Error at \"/nodes\": property \"nodes\" is missing"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
	t.Run("Create Cluster with Invalid JSON", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		// Create a server instance with a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder with invalid JSON
		requestBody := `{"template": "baseline-kubeadm", "name": "example-cluster"`
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader([]byte(requestBody)))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		expectedResponse := `{"message":"request body has an error: failed to decode request body: unexpected EOF"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
	t.Run("Create Cluster with Invalid Data Types", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		// Create a server instance with a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		server := NewServer(mockedk8sclient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder with invalid data types
		requestBody := `{"template": "baseline-kubeadm", "name": 12345}`
		req := httptest.NewRequest("POST", "/v2/clusters", bytes.NewReader([]byte(requestBody)))
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		expectedResponse := `{"message":"request body has an error: doesn't match schema #/components/schemas/ClusterSpec: Error at \"/name\": value must be a string"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}

func createPostV2ClustersStubServer(t *testing.T) *Server {
	expectedCluster := capi.Cluster{}
	unstructuredCluster, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedCluster)
	require.NoError(t, err, "failed to convert cluester to unstructured, error = %v, want nil")

	expectedTemplate := clusterv1alpha1.ClusterTemplate{}
	unstructuredTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&expectedTemplate)
	require.NoError(t, err, "failed to convert template to unstructured, error = %v, want nil")

	// Create a mock resource interface for clusters
	clusterResource := k8s.NewMockResourceInterface(t)
	clusterResource.EXPECT().Create(mock.Anything, mock.Anything, metav1.CreateOptions{}).Return(&unstructured.Unstructured{Object: unstructuredCluster}, nil).Maybe()

	// Create a mock namespaceable resource interface for clusters
	nsClusterResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsClusterResource.EXPECT().Namespace(mock.Anything).Return(clusterResource).Maybe()

	// Create a mock resource interface for clustertemplates
	templateResource := k8s.NewMockResourceInterface(t)
	templateResource.EXPECT().Get(mock.Anything, mock.Anything, metav1.GetOptions{}).Return(&unstructured.Unstructured{Object: unstructuredTemplate}, nil).Maybe()
	// Add support for List() calls when fetching default templates
	emptyTemplateList := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}
	templateResource.EXPECT().List(mock.Anything, mock.Anything).Return(emptyTemplateList, nil).Maybe()

	// Create a mock namespaceable resource interface for clustertemplates
	nsTemplateResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsTemplateResource.EXPECT().Namespace(mock.Anything).Return(templateResource).Maybe()

	// Create a mock k8s client
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsClusterResource).Maybe()
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsTemplateResource).Maybe()

	return &Server{
		k8sclient: mockedk8sclient,
	}
}

func FuzzPostV2Clusters(f *testing.F) {
	f.Add("abc", "def", "ghi", "jkl", "mno", "pqr",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, labelKey, labelVal, id, role, name, template string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createPostV2ClustersStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.PostV2ClustersParams{
			Activeprojectid: activeprojectid,
		}
		labels := map[string]string{labelKey: labelVal}
		nodes := []api.NodeSpec{{
			Id:   id,
			Role: api.NodeSpecRole(role),
		}}
		clusterSpec := api.ClusterSpec{
			Labels:   &labels,
			Name:     &name,
			Nodes:    nodes,
			Template: &template,
		}
		body := api.PostV2ClustersJSONRequestBody(clusterSpec)
		req := api.PostV2ClustersRequestObject{
			Params: params,
			Body:   &body,
		}
		_, _ = server.PostV2Clusters(context.Background(), req)
	})
}
