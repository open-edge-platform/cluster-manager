// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestPutV2Templates200(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create unstructured template with default:true label
	unstructuredCluster := &unstructured.Unstructured{}
	unstructuredCluster.SetLabels(map[string]string{"default": "true"})

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Setup the GetCached mock
	mockK8sClient.EXPECT().GetCached(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(unstructuredCluster, nil)

	// Create a server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/restricted/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)

	// Validate that the response body is empty
	require.Empty(t, rr.Body.String(), "Response body is not empty")
}

func TestPutV2Templates404(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Setup the not found error using GetCached directly
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		"privileged-v1.0.0",
	).Return(nil, k8serrors.NewNotFound(schema.GroupResource{Group: "template.x-k8s.io", Resource: "templates"}, "template1-v1.0.0"),)

	// Create a server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/privileged/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusNotFound, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusNotFound)
}

func TestPutV2Templates400(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Setup the bad request error using GetCached directly
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		"baseline-v1.0.0",
	).Return(nil, k8serrors.NewBadRequest("simulated bad request error"))

	// Create a server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)

	// Validate the error message in the response body
	expectedErrorMessage := `{"message":"failed to set default template"}`
	require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
}

func TestPutV2TemplatesInvalidProjectID(t *testing.T) {
	testCtx := context.Background()

	server := NewServer(nil)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", "00000000-0000-0000-0000-000000000000")

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
}

func TestPutV2TemplatesMissingNameOrVersion(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	server := NewServer(nil)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusBadRequest)
}

func TestPutV2TemplatesAlreadyDefault(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create unstructured template with default:true label
	unstructuredCluster := &unstructured.Unstructured{}
	unstructuredCluster.SetLabels(map[string]string{"default": "true"})

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Setup the mock using GetCached directly
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		"restricted-v1.0.0",
	).Return(unstructuredCluster, nil)

	// Create a server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/restricted/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)
}

func TestPutV2TemplatesListWithLabelSelector(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create unstructured template
	unstructuredCluster := &unstructured.Unstructured{}

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Setup the GetCached mock
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		"baseline-v1.0.0",
	).Return(unstructuredCluster, nil)

	// Mock the ListCached call to return existing default templates
	// Fixed: Use ListOptions instead of a string for the label selector
	existingDefaultTemplate := &unstructured.Unstructured{}
	existingDefaultTemplate.SetLabels(map[string]string{"default": "true"})
	existingDefaultTemplate.SetName("existing-default-template")

	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		v1.ListOptions{LabelSelector: "default=true"},
	).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*existingDefaultTemplate}}, nil)

	// Mock the Dynamic -> Resource -> Namespace -> Update chain
	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface).Times(2)
	mockDynamicInterface.EXPECT().Resource(core.TemplateResourceSchema).Return(mockNamespaceableResourceInterface).Times(2)
	mockNamespaceableResourceInterface.EXPECT().Namespace(expectedActiveProjectID).Return(mockResourceInterface).Times(2)
	mockResourceInterface.EXPECT().Update(mock.Anything,mock.Anything,v1.UpdateOptions{}).Return(nil, nil).Times(2)

	// Create a server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)
}

func TestFetchTemplateVersions(t *testing.T) {
	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Create a fake ClusterTemplate
	clusterTemplate := &v1alpha1.ClusterTemplate{
		ObjectMeta: v1.ObjectMeta{
			Name:      "template1-v1.0.0",
			Namespace: "default",
		},
	}

	unstructuredClusterTemplate, err := runtime.DefaultUnstructuredConverter.ToUnstructured(clusterTemplate)
	require.NoError(t, err)

	templateList := &unstructured.UnstructuredList{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "TemplateList",
		},
		Items: []unstructured.Unstructured{{Object: unstructuredClusterTemplate}},
	}

	// Mock ListCached instead of the Dynamic chain, using v1.ListOptions{} not ""
	mockK8sClient.EXPECT().ListCached(mock.Anything, core.TemplateResourceSchema,"default",v1.ListOptions{}).Return(templateList, nil)

	// Create a server instance
	server := NewServer(mockK8sClient)

	// Call the fetchAndSelectLatestVersion method
	version, err := server.fetchAndSelectLatestVersion(context.Background(), "template1", "default")
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", version)
}

func TestPutV2TemplatesFetchTemplateVersionsError(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Simulate an error in ListCached - UPDATED: use v1.ListOptions{} instead of ""
	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		v1.ListOptions{},
	).Return(nil, errors.New("internal server error"))

	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/privileged/default", strings.NewReader(`{"version": ""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)

	// Validate the error message in the response body
	expectedErrorMessage := `{"message":"failed to fetch and select latest version"}`
	require.JSONEq(t, expectedErrorMessage, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), expectedErrorMessage)
}

func TestFetchAndSelectLatestVersion(t *testing.T) {
	expectedNamespace := "default"
	templateName := "baseline"

	// Create mock templates
	template1v1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "baseline-v1.0.0",
				"namespace": expectedNamespace,
			},
			"spec": map[string]interface{}{
				"version": "v1.0.0",
			},
		},
	}

	template1v2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "baseline-v2.0.0",
				"namespace": expectedNamespace,
			},
			"spec": map[string]interface{}{
				"version": "v2.0.0",
			},
		},
	}

	template1v3 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "baseline-v3.0.0",
				"namespace": expectedNamespace,
			},
			"spec": map[string]interface{}{
				"version": "v3.0.0",
			},
		},
	}

	other1v3 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "other-v3.0.0",
				"namespace": expectedNamespace,
			},
			"spec": map[string]interface{}{
				"version": "v2.1.1",
			},
		},
	}

	// Create a list of templates
	templateList := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{*template1v1, *template1v2, *template1v3, *other1v3},
	}

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock ListCached instead of Dynamic chain
	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedNamespace,
		v1.ListOptions{}, // Changed from "" to v1.ListOptions{}
	).Return(templateList, nil)

	// Create a server instance
	server := NewServer(mockK8sClient)

	// Call the fetchAndSelectLatestVersion method
	version, err := server.fetchAndSelectLatestVersion(context.Background(), templateName, expectedNamespace)
	require.NoError(t, err)
	require.Equal(t, "v3.0.0", version)
}

func TestFetchAndSelectLatestVersionNotFound(t *testing.T) {
	expectedNamespace := "default"
	templateName := "template1"

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock ListCached to return an empty list
	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedNamespace,
		v1.ListOptions{},
	).Return(&unstructured.UnstructuredList{}, nil)

	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	_, err := server.fetchAndSelectLatestVersion(context.Background(), templateName, expectedNamespace)
	require.Error(t, err)
	require.Equal(t, "clusterTemplate with name 'template1' not found", err.Error())
}

func TestPutV2TemplatesSetMostRecentVersionAsDefault2(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Mock templates
	template1v1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "baseline-v1.0.0",
				"namespace": expectedActiveProjectID,
			},
			"spec": map[string]interface{}{
				"version": "v1.0.0",
			},
		},
	}

	template1v2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "baseline-v2.0.0",
				"namespace": expectedActiveProjectID,
			},
			"spec": map[string]interface{}{
				"version": "v2.0.0",
			},
		},
	}

	template1v3 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "baseline-v3.0.0",
				"namespace": expectedActiveProjectID,
			},
			"spec": map[string]interface{}{
				"version": "v3.0.0",
			},
		},
	}

	otherTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ClusterTemplate",
			"metadata": map[string]interface{}{
				"name":      "privileged-v1.0.0",
				"namespace": expectedActiveProjectID,
			},
			"spec": map[string]interface{}{
				"version": "v1.0.0",
			},
		},
	}

	// Create a list of templates
	templateList := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{*template1v1, *template1v2, *template1v3, *otherTemplate},
	}

	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Mock ListCached to return the template list
	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		v1.ListOptions{},
	).Return(templateList, nil)

	// Mock GetCached to return the most recent template
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		"baseline-v3.0.0",
	).Return(template1v3, nil)

	// Mock ListCached to return existing default templates
	existingDefaultTemplate := &unstructured.Unstructured{}
	existingDefaultTemplate.SetLabels(map[string]string{"default": "true"})
	existingDefaultTemplate.SetName("existing-default-template")

	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		core.TemplateResourceSchema,
		expectedActiveProjectID,
		v1.ListOptions{LabelSelector: "default=true"},
	).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*existingDefaultTemplate}}, nil)

	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface).Times(2)
	mockDynamicInterface.EXPECT().Resource(core.TemplateResourceSchema).Return(mockNamespaceableResourceInterface).Times(2)
	mockNamespaceableResourceInterface.EXPECT().Namespace(expectedActiveProjectID).Return(mockResourceInterface).Times(2)
	mockResourceInterface.EXPECT().Update(mock.Anything,mock.Anything,v1.UpdateOptions{}).Return(nil, nil).Times(2)

	// Create a server with the mocked client
	server := NewServer(mockK8sClient)
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// Create a new request & response recorder
	requestBody := `{"version": "", "randomField": "randomValue"}`
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// Log the request body
	t.Log("Request Body:", requestBody)

	// Serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println("Response Body:", rr.Body.String())
	t.Log("Response Body:", rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)

	// Validate that the response body is empty
	require.Empty(t, rr.Body.String(), "Response body is not empty")
}

func createPutV2TemplatesNameStubServer(t *testing.T) *Server {
	// Create mock client
	mockK8sClient := k8s.NewMockClient(t)

	// Set up the chain for Dynamic() -> Resource() -> Namespace() -> Get/List/etc
	mockDynamicInterface := k8s.NewMockInterface(t)
	mockNamespaceableResourceInterface := k8s.NewMockNamespaceableResourceInterface(t)
	mockResourceInterface := k8s.NewMockResourceInterface(t)

	// Setup the basic chain that will be used for all operations
	mockK8sClient.EXPECT().Dynamic().Return(mockDynamicInterface).Maybe()
	mockDynamicInterface.EXPECT().Resource(mock.Anything).Return(mockNamespaceableResourceInterface).Maybe()
	mockNamespaceableResourceInterface.EXPECT().Namespace(mock.Anything).Return(mockResourceInterface).Maybe()

	// Now we can set up the actual operations on the resource interface
	mockResourceInterface.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(&unstructured.Unstructured{}, nil).Maybe()
	mockResourceInterface.EXPECT().List(mock.Anything, mock.Anything).Return(&unstructured.UnstructuredList{}, nil).Maybe()
	mockResourceInterface.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	mockResourceInterface.EXPECT().UpdateStatus(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	// Set up GetCached and ListCached which are direct methods on the client
	mockK8sClient.EXPECT().GetCached(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(&unstructured.Unstructured{}, nil).Maybe()

	mockK8sClient.EXPECT().ListCached(
		mock.Anything,
		mock.Anything,
		mock.Anything,
		mock.MatchedBy(func(options interface{}) bool {
			// Match any v1.ListOptions parameter
			_, ok := options.(v1.ListOptions)
			return ok
		}),
	).Return(&unstructured.UnstructuredList{}, nil).Maybe()

	// Mock Cluster method
	mockK8sClient.EXPECT().GetCluster(
		mock.Anything,
		mock.Anything,
		mock.Anything,
	).Return(&capi.Cluster{}, nil).Maybe()

	return NewServer(mockK8sClient)
}

func FuzzPutV2TemplatesName(f *testing.F) {
	f.Add("abc", "def", "ghi",
		byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, name, infoName, ver string,
		u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createPutV2TemplatesNameStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		params := api.PutV2TemplatesNameDefaultParams{
			Activeprojectid: activeprojectid,
		}
		info := api.DefaultTemplateInfo{
			Name:    &infoName,
			Version: ver,
		}
		body := api.PutV2TemplatesNameDefaultJSONRequestBody(info)
		req := api.PutV2TemplatesNameDefaultRequestObject{
			Name:   name,
			Params: params,
			Body:   &body,
		}
		server.PutV2TemplatesNameDefault(context.Background(), req)
	})
}
