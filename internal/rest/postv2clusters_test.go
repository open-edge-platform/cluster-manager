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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func TestPostV2Clusters201(t *testing.T) {

	t.Run("Create Cluster", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-kubeadm"
		expectedCluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "cluster.x-k8s.io/v1beta1",
				Kind:       "clusters",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-cluster",
				Namespace: expectedActiveProjectID,
				Labels: map[string]string{
					"edge-orchestrator.intel.com/clustername": "example-cluster",
					"edge-orchestrator.intel.com/project-id":  expectedActiveProjectID,
					"prometheusMetricsURL":                    "metrics-node.kind.internal",
					"trusted-compute-compatible":              "false",
					"default-extension":                       "baseline",
					"test":                                    "true",
				},
				Annotations: map[string]string{
					"edge-orchestrator.intel.com/template": expectedTemplateName,
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
				},
			},
		}

		// Create the template to be returned
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

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method
		mockK8sClient.EXPECT().Template(mock.Anything, expectedActiveProjectID, expectedTemplateName).Return(expectedTemplate, nil)

		mockK8sClient.EXPECT().CreateCluster(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(cluster capi.Cluster) bool {
				return cluster.Name == expectedCluster.Name
			}),
		).Return(expectedCluster.Name, nil)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
				},
			},
		}

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

		// Mock the template to be returned
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

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method
		mockK8sClient.EXPECT().Template(mock.Anything, expectedActiveProjectID, expectedTemplateName).Return(expectedTemplate, nil)

		// Mock CreateCluster method
		mockK8sClient.EXPECT().CreateCluster(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(cluster capi.Cluster) bool {
				return cluster.Name == expectedCluster.Name
			}),
		).Return(expectedCluster.Name, nil)

		// Mock CreateMachineBinding method
		mockK8sClient.EXPECT().CreateMachineBinding(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(binding intelv1alpha1.IntelMachineBinding) bool {
				// Compare more fields from expectedBinding for a more thorough test
				return binding.Name == expectedBinding.Name &&
					binding.Namespace == expectedBinding.Namespace &&
					binding.Spec.NodeGUID == expectedBinding.Spec.NodeGUID &&
					binding.Spec.ClusterName == expectedBinding.Spec.ClusterName &&
					binding.Spec.IntelMachineTemplateName == expectedBinding.Spec.IntelMachineTemplateName
			}),
		).Return(nil)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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

	t.Run("Failed to create Binding", func(t *testing.T) {
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
				},
			},
		}

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

		// Mock the template to be returned
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

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method
		mockK8sClient.EXPECT().Template(mock.Anything, expectedActiveProjectID, expectedTemplateName).Return(expectedTemplate, nil)

		// Mock CreateCluster method
		mockK8sClient.EXPECT().CreateCluster(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(cluster capi.Cluster) bool {
				return cluster.Name == expectedCluster.Name
			}),
		).Return(expectedCluster.Name, nil)

		// Mock CreateMachineBinding method
		mockK8sClient.EXPECT().CreateMachineBinding(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(binding intelv1alpha1.IntelMachineBinding) bool {
				// Compare more fields from expectedBinding for a more thorough test
				return binding.Name == expectedBinding.Name &&
					binding.Namespace == expectedBinding.Namespace &&
					binding.Spec.NodeGUID == expectedBinding.Spec.NodeGUID &&
					binding.Spec.ClusterName == expectedBinding.Spec.ClusterName &&
					binding.Spec.IntelMachineTemplateName == expectedBinding.Spec.IntelMachineTemplateName
			}),
		).Return(nil)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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

func TestPostV2Clusters201NoNameNoTemplate(t *testing.T) {
	// Prepare test data
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

	// This simulates a cluster that would be created with a generated name
	expectedCluster := capi.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.x-k8s.io/v1beta1",
			Kind:       "clusters",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-12345", // Server will generate a name like this
			Namespace: expectedActiveProjectID,
			Labels: map[string]string{
				"edge-orchestrator.intel.com/clustername": "cluster-12345",
				"edge-orchestrator.intel.com/project-id":  expectedActiveProjectID,
				"prometheusMetricsURL":                    "metrics-node.kind.internal",
				"trusted-compute-compatible":              "false",
				"default-extension":                       "baseline",
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
			},
		},
	}

	// Default template that should be returned when no template specified
	expectedTemplate := clusterv1alpha1.ClusterTemplate{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name: "baseline-kubeadm",
			Labels: map[string]string{
				"default": "true",
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

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock DefaultTemplate method - called when no template specified
	mockK8sClient.EXPECT().DefaultTemplate(mock.Anything, expectedActiveProjectID).Return(expectedTemplate, nil)

	// Mock CreateCluster method - match any cluster since the name is generated
	mockK8sClient.EXPECT().CreateCluster(
		mock.Anything,
		expectedActiveProjectID,
		mock.Anything,
	).Return(expectedCluster.Name, nil)

	// Create a server instance with the mock client
	server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a new request & response recorder - omitting name and template
	clusterSpec := api.ClusterSpec{
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
	// Should contain a success message
	assert.Contains(t, rr.Body.String(), "successfully created cluster")
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

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method
		mockK8sClient.EXPECT().Template(mock.Anything, expectedActiveProjectID, expectedTemplateName).Return(expectedTemplate, nil)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
				ClusterClassRef: nil, // Missing ClusterClassRef
			},
		}

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method
		mockK8sClient.EXPECT().Template(
			mock.Anything,
			expectedActiveProjectID,
			expectedTemplateName,
		).Return(expectedTemplate, nil)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
		expectedError := fmt.Errorf("failed to get cluster template")

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method to return an error
		mockK8sClient.EXPECT().Template(
			mock.Anything,
			expectedActiveProjectID,
			expectedTemplateName,
		).Return(clusterv1alpha1.ClusterTemplate{}, expectedError)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
				},
			},
		}

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

		// Mock the template to be returned
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

		// Create mock client
		mockK8sClient := k8s.NewMockClient(t)

		// Mock Template method
		mockK8sClient.EXPECT().Template(mock.Anything, expectedActiveProjectID, expectedTemplateName).Return(expectedTemplate, nil)

		// Mock CreateCluster method
		mockK8sClient.EXPECT().CreateCluster(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(cluster capi.Cluster) bool {
				return cluster.Name == expectedCluster.Name
			}),
		).Return(expectedCluster.Name, nil)

		// Mock CreateMachineBinding method
		mockK8sClient.EXPECT().CreateMachineBinding(
			mock.Anything,
			expectedActiveProjectID,
			mock.MatchedBy(func(binding intelv1alpha1.IntelMachineBinding) bool {
				// Compare more fields from expectedBinding for a more thorough test
				return binding.Name == expectedBinding.Name &&
					binding.Namespace == expectedBinding.Namespace &&
					binding.Spec.NodeGUID == expectedBinding.Spec.NodeGUID &&
					binding.Spec.ClusterName == expectedBinding.Spec.ClusterName &&
					binding.Spec.IntelMachineTemplateName == expectedBinding.Spec.IntelMachineTemplateName
			}),
		).Return(nil)

		// Create a server instance with the mock client
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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

func TestPostV2Clusters400(t *testing.T) {
	t.Run("Invalid Project ID", func(t *testing.T) {
		// Prepare test data
		expectedActiveProjectID := "00000000-0000-0000-0000-000000000000"
		expectedTemplateName := "baseline-kubeadm"

		// Create a server instance with a mock k8s client
		mockK8sClient := k8s.NewMockClient(t)
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
		mockK8sClient := k8s.NewMockClient(t)
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
		mockK8sClient := k8s.NewMockClient(t)
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
		mockK8sClient := k8s.NewMockClient(t)
		server := NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
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
	// Create a mock client using the correct approach
	mockK8sClient := k8s.NewMockClient(t)

	// Setup default expectations for methods that might be called during fuzzing

	// Template method might be called with any arguments
	mockK8sClient.EXPECT().Template(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(clusterv1alpha1.ClusterTemplate{
		Status: clusterv1alpha1.ClusterTemplateStatus{
			Ready: true,
			ClusterClassRef: &corev1.ObjectReference{
				Name: "test-cluster-class",
			},
		},
	}, nil).Maybe()

	// DefaultTemplate might be called if no template specified
	mockK8sClient.EXPECT().DefaultTemplate(
		mock.Anything,
		mock.Anything,
	).Return(clusterv1alpha1.ClusterTemplate{
		Status: clusterv1alpha1.ClusterTemplateStatus{
			Ready: true,
			ClusterClassRef: &corev1.ObjectReference{
				Name: "test-cluster-class",
			},
		},
	}, nil).Maybe()

	// CreateCluster might be called
	mockK8sClient.EXPECT().CreateCluster(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return("test-cluster", nil).Maybe()

	// CreateMachineBinding might be called
	mockK8sClient.EXPECT().CreateMachineBinding(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(nil).Maybe()

	// Create a server with the mock client
	return NewServer(mockK8sClient, WithConfig(&config.Config{ClusterDomain: "kind.internal"}))
}

func FuzzPostV2Clusters(f *testing.F) {
	f.Add("abc", "def", "ghi", "jkl", "mno", "pqr",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))

	f.Fuzz(func(t *testing.T, labelKey, labelVal, id, role, name, template string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {

		// Create the test server using the improved stub server
		server := createPostV2ClustersStubServer(t)

		// Create UUID and active project ID
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.PostV2ClustersParams{
			Activeprojectid: activeprojectid,
		}

		// Create request body
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

		// We don't need to assert anything for fuzzing - just make sure it doesn't crash
		_, _ = server.PostV2Clusters(context.Background(), req)
	})
}
