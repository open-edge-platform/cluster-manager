// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"net/http"
	"net/http/httptest"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"testing"

	"github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/inventory"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func TestDeleteV2ClustersName204(t *testing.T) {
	t.Run("Successful Deletion", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedTemplateName := "baseline-rke2"
		expectedIntelMachineTemplateName := fmt.Sprintf("%s-controlplane", expectedTemplateName)
		expectedNodeid := "27b4e138-ea0b-11ef-8552-8b663d95bc01"
		expectedClusterName := "example-cluster"
		expectedBindingName := fmt.Sprintf("%s-%s", expectedClusterName, expectedNodeid)

		binding := v1alpha1.IntelMachineBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
				Kind:       "IntelMachineBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedBindingName,
				Namespace: expectedActiveProjectID,
			},
			Spec: v1alpha1.IntelMachineBindingSpec{
				NodeGUID:                 expectedNodeid,
				ClusterName:              expectedClusterName,
				IntelMachineTemplateName: expectedIntelMachineTemplateName,
			},
		}
		unstructured, err := convert.ToUnstructuredList([]v1alpha1.IntelMachineBinding{binding})
		require.NoError(t, err, "convert.ToUnstructuredList() failed")

		cluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.ClusterResourceSchema.GroupVersion().String(),
				Kind:       "IntelCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: expectedActiveProjectID,
			},
			Spec: capi.ClusterSpec{
				InfrastructureRef: &v1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
					Kind:       "IntelCluster",
				},
			},
			Status: capi.ClusterStatus{},
		}
		unstructuredCluster, err := convert.ToUnstructured(cluster)
		require.NoError(t, err, "convert.ToUnstructured() failed for cluster")

		// Mock the delete cluster to succeed
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().Delete(mock.Anything, name, metav1.DeleteOptions{}).Return(nil)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%s", name),
		}).Return(unstructured, nil)
		resource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(unstructuredCluster, nil)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(resource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.BindingsResourceSchema).Return(nsResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

		// Create a new server with the mocked k8s client
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s", name), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusNoContent, rr.Code)
	})
}

func TestDeleteV2ClustersName400(t *testing.T) {
	t.Run("Missing Active Project ID", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "00000000-0000-0000-0000-000000000000"
		// Mock the delete cluster to succeed
		mockedk8sclient := k8s.NewMockInterface(t)
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s", name), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		expectedResponse := `{"message": "no active project id provided"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}

func TestDeleteV2ClustersName404(t *testing.T) {
	t.Run("Cluster Not Found", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		expectedTemplateName := "baseline-rke2"
		expectedIntelMachineTemplateName := fmt.Sprintf("%s-controlplane", expectedTemplateName)
		expectedNodeid := "27b4e138-ea0b-11ef-8552-8b663d95bc01"
		expectedClusterName := "example-cluster"
		expectedBindingName := fmt.Sprintf("%s-%s", expectedClusterName, expectedNodeid)

		binding := v1alpha1.IntelMachineBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
				Kind:       "IntelMachineBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedBindingName,
				Namespace: expectedActiveProjectID,
			},
			Spec: v1alpha1.IntelMachineBindingSpec{
				NodeGUID:                 expectedNodeid,
				ClusterName:              expectedClusterName,
				IntelMachineTemplateName: expectedIntelMachineTemplateName,
			},
		}
		unstructured, err := convert.ToUnstructuredList([]v1alpha1.IntelMachineBinding{binding})
		require.NoError(t, err, "convert.ToUnstructuredList() failed")

		cluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.ClusterResourceSchema.GroupVersion().String(),
				Kind:       "IntelCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: expectedActiveProjectID,
			},
			Spec: capi.ClusterSpec{
				InfrastructureRef: &v1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
					Kind:       "IntelCluster",
				},
			},
			Status: capi.ClusterStatus{},
		}
		unstructuredCluster, err := convert.ToUnstructured(cluster)
		require.NoError(t, err, "convert.ToUnstructured() failed for cluster")

		// Mock the delete cluster to succeed
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%s", name),
		}).Return(unstructured, nil)
		resource.EXPECT().Delete(mock.Anything, name, metav1.DeleteOptions{}).Return(errors.NewNotFound(schema.GroupResource{Group: "core", Resource: "clusters"}, name))
		resource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(unstructuredCluster, nil)

		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(resource)
		// Create a server instance with a mock k8s client
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.BindingsResourceSchema).Return(nsResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

		// Create a new server with the mocked k8s client
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s", name), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusNotFound, rr.Code)
		expectedResponse := fmt.Sprintf(`{"message":"cluster %s not found in namespace %s"}`, name, activeProjectID)
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}

func TestDeleteV2ClustersName500(t *testing.T) {
	t.Run("Error when Deleting Cluster", func(t *testing.T) {
		// Prepare test data
		name := "example-cluster"
		activeProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"

		expectedTemplateName := "baseline-rke2"
		expectedIntelMachineTemplateName := fmt.Sprintf("%s-controlplane", expectedTemplateName)
		expectedNodeid := "27b4e138-ea0b-11ef-8552-8b663d95bc01"
		expectedClusterName := "example-cluster"
		expectedBindingName := fmt.Sprintf("%s-%s", expectedClusterName, expectedNodeid)

		binding := v1alpha1.IntelMachineBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
				Kind:       "IntelMachineBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      expectedBindingName,
				Namespace: expectedActiveProjectID,
			},
			Spec: v1alpha1.IntelMachineBindingSpec{
				NodeGUID:                 expectedNodeid,
				ClusterName:              expectedClusterName,
				IntelMachineTemplateName: expectedIntelMachineTemplateName,
			},
		}
		unstructured, err := convert.ToUnstructuredList([]v1alpha1.IntelMachineBinding{binding})
		require.NoError(t, err, "convert.ToUnstructuredList() failed")

		cluster := capi.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: core.ClusterResourceSchema.GroupVersion().String(),
				Kind:       "IntelCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: expectedActiveProjectID,
			},
			Spec: capi.ClusterSpec{
				InfrastructureRef: &v1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
					Kind:       "IntelCluster",
				},
			},
			Status: capi.ClusterStatus{},
		}
		unstructuredCluster, err := convert.ToUnstructured(cluster)
		require.NoError(t, err, "convert.ToUnstructured() failed for cluster")

		// Mock the get cluster to succeed and delete cluster to fail
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%s", name),
		}).Return(unstructured, nil)
		resource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(unstructuredCluster, nil)

		resource.EXPECT().Delete(mock.Anything, name, metav1.DeleteOptions{}).Return(fmt.Errorf("delete error"))
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectID).Return(resource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.BindingsResourceSchema).Return(nsResource)
		mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource)

		// Create a new server with the mocked k8s client
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")
		// Create a new request & response recorder
		req := httptest.NewRequest("DELETE", fmt.Sprintf("/v2/clusters/%s", name), nil)
		req.Header.Set("Activeprojectid", activeProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// Create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Check the response
		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		expectedResponse := `{"message":"failed to delete cluster"}`
		assert.JSONEq(t, expectedResponse, rr.Body.String())
	})
}

func createDeleteV2ClustersNameStubServer(t *testing.T) *Server {

	name := "abc"
	expectedTemplateName := "baseline-rke2"
	expectedIntelMachineTemplateName := fmt.Sprintf("%s-controlplane", expectedTemplateName)
	expectedNodeid := "27b4e138-ea0b-11ef-8552-8b663d95bc01"
	expectedClusterName := "example-cluster"
	expectedBindingName := fmt.Sprintf("%s-%s", expectedClusterName, expectedNodeid)

	binding := v1alpha1.IntelMachineBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
			Kind:       "IntelMachineBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedBindingName,
			Namespace: expectedActiveProjectID,
		},
		Spec: v1alpha1.IntelMachineBindingSpec{
			NodeGUID:                 expectedNodeid,
			ClusterName:              expectedClusterName,
			IntelMachineTemplateName: expectedIntelMachineTemplateName,
		},
	}
	unstructured, err := convert.ToUnstructuredList([]v1alpha1.IntelMachineBinding{binding})
	require.NoError(t, err, "convert.ToUnstructuredList() failed")

	cluster := capi.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: core.ClusterResourceSchema.GroupVersion().String(),
			Kind:       "IntelCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: expectedActiveProjectID,
		},
		Spec: capi.ClusterSpec{
			InfrastructureRef: &v1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
				Kind:       "IntelCluster",
			},
		},
		Status: capi.ClusterStatus{},
	}
	unstructuredCluster, err := convert.ToUnstructured(cluster)
	require.NoError(t, err, "convert.ToUnstructured() failed for cluster")

	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().List(mock.Anything, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%s", name),
	}).Return(unstructured, nil).Maybe()
	resource.EXPECT().Get(mock.Anything, name, metav1.GetOptions{}).Return(unstructuredCluster, nil)

	resource.EXPECT().Delete(mock.Anything, mock.Anything, metav1.DeleteOptions{}).Return(nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.BindingsResourceSchema).Return(nsResource).Maybe()
	mockedk8sclient.EXPECT().Resource(core.ClusterResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
		inventory: inventory.NewNoopInventoryClient(),
	}
}

func FuzzDeleteV2ClustersName(f *testing.F) {
	f.Add("abc", byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name string, u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createDeleteV2ClustersNameStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.DeleteV2ClustersNameParams{
			Activeprojectid: activeprojectid,
		}
		req := api.DeleteV2ClustersNameRequestObject{
			Name:   name,
			Params: params,
		}
		_, _ = server.DeleteV2ClustersName(context.Background(), req)
	})
}
