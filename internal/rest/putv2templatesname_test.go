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
)

func TestPutV2Templates200(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	unstructuredCluster := &unstructured.Unstructured{}
	unstructuredCluster.SetLabels(map[string]string{"default": "true"})
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, "restricted-v1.0.0", v1.GetOptions{}).Return(unstructuredCluster, nil)
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	server := NewServer(wrapMockInterface(mockedk8sclient))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/restricted/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)

	// Validate that the response body is empty
	require.Empty(t, rr.Body.String(), "Response body is not empty")
}

func TestPutV2Templates404(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, "privileged-v1.0.0", v1.GetOptions{}).Return(nil, k8serrors.NewNotFound(schema.GroupResource{Group: "template.x-k8s.io", Resource: "templates"}, "template1-v1.0.0"))
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	server := NewServer(wrapMockInterface(mockedk8sclient))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// create a handler with middleware
	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	// create a new request & response recorder
	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/privileged/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	// serve the request
	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusNotFound, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusNotFound)
}

func TestPutV2Templates400(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create a mock dynamic client
	mockClient := new(k8s.MockInterface)
	mockResource := new(k8s.MockResourceInterface)
	mockNamespaceableResource := new(k8s.MockNamespaceableResourceInterface)

	// Set up the mock expectations
	gvr := core.TemplateResourceSchema // Use the correct GroupVersionResource from the core package
	mockClient.On("Resource", gvr).Return(mockNamespaceableResource)
	mockNamespaceableResource.On("Namespace", expectedActiveProjectID).Return(mockResource)

	// Mock the Get call to return a bad request error
	mockResource.On("Get", mock.Anything, "baseline-v1.0.0", v1.GetOptions{}).Return(nil, k8serrors.NewBadRequest("simulated bad request error"))

	server := NewServer(wrapMockInterface(mockClient))
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

	unstructuredCluster := &unstructured.Unstructured{}
	unstructuredCluster.SetLabels(map[string]string{"default": "true"})
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, "restricted-v1.0.0", v1.GetOptions{}).Return(unstructuredCluster, nil)
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	server := NewServer(wrapMockInterface(mockedk8sclient))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/restricted/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)
}

func TestPutV2TemplatesListWithLabelSelector(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	unstructuredCluster := &unstructured.Unstructured{}
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, "baseline-v1.0.0", v1.GetOptions{}).Return(unstructuredCluster, nil)
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

	// Mock the list call to return existing default templates
	existingDefaultTemplate := &unstructured.Unstructured{}
	existingDefaultTemplate.SetLabels(map[string]string{"default": "true"})
	existingDefaultTemplate.SetName("existing-default-template")
	resource.EXPECT().List(mock.Anything, v1.ListOptions{LabelSelector: "default=true"}).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*existingDefaultTemplate}}, nil)

	// Mock the update call to unlabel the existing default template
	resource.EXPECT().Update(mock.Anything, mock.Anything, v1.UpdateOptions{}).Return(nil, nil).Times(2)

	server := NewServer(wrapMockInterface(mockedk8sclient))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	handler, err := server.ConfigureHandler()
	require.Nil(t, err)

	req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(`{"version": "v1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Activeprojectid", expectedActiveProjectID)

	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	fmt.Println(rr.Body.String())
	require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusOK)
}

func TestFetchTemplateVersions(t *testing.T) {
	// Create a mock dynamic client
	mockClient := new(k8s.MockInterface)
	mockResource := new(k8s.MockResourceInterface)
	mockNamespaceableResource := new(k8s.MockNamespaceableResourceInterface)

	// Set up the mock expectations
	gvr := core.TemplateResourceSchema // Use the correct GroupVersionResource from the core package
	mockClient.On("Resource", gvr).Return(mockNamespaceableResource)
	mockNamespaceableResource.On("Namespace", "default").Return(mockResource)

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
	mockResource.On("List", mock.Anything, v1.ListOptions{}).Return(templateList, nil)

	// Create a server instance
	server := NewServer(wrapMockInterface(mockClient))

	// Call the fetchAndSelectLatestVersion method
	version, err := server.fetchAndSelectLatestVersion(context.Background(), "template1", "default")
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", version)

	// Assert that the expectations were met
	mockClient.AssertExpectations(t)
	mockResource.AssertExpectations(t)
	mockNamespaceableResource.AssertExpectations(t)
}

func TestPutV2TemplatesFetchTemplateVersionsError(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	// Create a mock dynamic client
	mockClient := new(k8s.MockInterface)
	mockResource := new(k8s.MockResourceInterface)
	mockNamespaceableResource := new(k8s.MockNamespaceableResourceInterface)

	// Set up the mock expectations
	gvr := core.TemplateResourceSchema // Use the correct GroupVersionResource from the core package
	mockClient.On("Resource", gvr).Return(mockNamespaceableResource)
	mockNamespaceableResource.On("Namespace", expectedActiveProjectID).Return(mockResource)

	// Simulate an error in fetchTemplateVersions
	mockResource.On("List", mock.Anything, mock.Anything).Return(nil, errors.New("internal server error"))

	server := NewServer(wrapMockInterface(mockClient))
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

func TestPutV2TemplatesInternalServerError(t *testing.T) {
	expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
	testCtx := context.WithValue(context.Background(), core.ActiveProjectIdContextKey, expectedActiveProjectID)

	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedError  string
		mockSetup      func(resource *k8s.MockResourceInterface)
	}{
		{
			name:           "500 Internal Server Error",
			requestBody:    `{"version": "v1.0.0"}`,
			expectedStatus: http.StatusInternalServerError,
			expectedError:  `{"message":"unexpected error occurred"}`,
			mockSetup: func(resource *k8s.MockResourceInterface) {
				resource.EXPECT().Get(mock.Anything, "baseline-v1.0.0", v1.GetOptions{}).Return(nil, errors.New("internal server error"))
			},
		},
		{
			name:           "Fetch Template Versions Error",
			requestBody:    `{"version": ""}`,
			expectedStatus: http.StatusInternalServerError,
			expectedError:  `{"message":"failed to fetch and select latest version"}`,
			mockSetup: func(resource *k8s.MockResourceInterface) {
				resource.EXPECT().List(mock.Anything, mock.Anything).Return(nil, errors.New("internal server error"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := k8s.NewMockResourceInterface(t)
			nsResource := k8s.NewMockNamespaceableResourceInterface(t)
			nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
			mockedk8sclient := k8s.NewMockInterface(t)
			mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

			tt.mockSetup(resource)

			server := NewServer(wrapMockInterface(mockedk8sclient))
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			handler, err := server.ConfigureHandler()
			require.Nil(t, err)

			req := httptest.NewRequestWithContext(testCtx, "PUT", "/v2/templates/baseline/default", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Activeprojectid", expectedActiveProjectID)

			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			fmt.Println(rr.Body.String())
			require.Equal(t, tt.expectedStatus, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, tt.expectedStatus)
			require.JSONEq(t, tt.expectedError, rr.Body.String(), "Response body = %v, want %v", rr.Body.String(), tt.expectedError)
		})
	}
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

	// Create a mock dynamic client
	mockClient := new(k8s.MockInterface)
	mockResource := new(k8s.MockResourceInterface)
	mockNamespaceableResource := new(k8s.MockNamespaceableResourceInterface)

	// Set up the mock expectations
	gvr := core.TemplateResourceSchema // Use the correct GroupVersionResource from the core package
	mockClient.On("Resource", gvr).Return(mockNamespaceableResource)
	mockNamespaceableResource.On("Namespace", expectedNamespace).Return(mockResource)

	// Mock the List call to return the template list
	mockResource.On("List", mock.Anything, v1.ListOptions{}).Return(templateList, nil)

	server := NewServer(wrapMockInterface(mockClient))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Call the fetchAndSelectLatestVersion method
	version, err := server.fetchAndSelectLatestVersion(context.Background(), templateName, expectedNamespace)
	require.NoError(t, err)
	require.Equal(t, "v3.0.0", version)

	// Assert that the expectations were met
	mockClient.AssertExpectations(t)
	mockResource.AssertExpectations(t)
	mockNamespaceableResource.AssertExpectations(t)
}

func TestFetchAndSelectLatestVersionNotFound(t *testing.T) {
	expectedNamespace := "default"
	templateName := "template1"

	// Create a mock dynamic client
	mockClient := new(k8s.MockInterface)
	mockResource := new(k8s.MockResourceInterface)
	mockNamespaceableResource := new(k8s.MockNamespaceableResourceInterface)

	// Set up the mock expectations
	gvr := core.TemplateResourceSchema // Use the correct GroupVersionResource from the core package
	mockClient.On("Resource", gvr).Return(mockNamespaceableResource)
	mockNamespaceableResource.On("Namespace", expectedNamespace).Return(mockResource)

	// Mock the List call to return an empty list
	mockResource.On("List", mock.Anything, v1.ListOptions{}).Return(&unstructured.UnstructuredList{}, nil)

	server := NewServer(wrapMockInterface(mockClient))
	require.NotNil(t, server, "NewServer() returned nil, want not nil")

	// Call the fetchAndSelectLatestVersion method
	_, err := server.fetchAndSelectLatestVersion(context.Background(), templateName, expectedNamespace)
	require.Error(t, err)
	require.Equal(t, "clusterTemplate with name 'template1' not found", err.Error())

	// Assert that the expectations were met
	mockClient.AssertExpectations(t)
	mockResource.AssertExpectations(t)
	mockNamespaceableResource.AssertExpectations(t)
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

	// Create a mock dynamic client
	mockClient := new(k8s.MockInterface)
	mockResource := new(k8s.MockResourceInterface)
	mockNamespaceableResource := new(k8s.MockNamespaceableResourceInterface)

	// Set up the mock expectations
	gvr := core.TemplateResourceSchema // Use the correct GroupVersionResource from the core package
	mockClient.On("Resource", gvr).Return(mockNamespaceableResource)
	mockNamespaceableResource.On("Namespace", expectedActiveProjectID).Return(mockResource)

	// Mock the List call to return the template list
	mockResource.On("List", mock.Anything, v1.ListOptions{}).Return(templateList, nil)

	// Mock the Get call to return the most recent template
	mockResource.On("Get", mock.Anything, "baseline-v3.0.0", v1.GetOptions{}).Return(template1v3, nil)

	// Mock the List call to return existing default templates
	existingDefaultTemplate := &unstructured.Unstructured{}
	existingDefaultTemplate.SetLabels(map[string]string{"default": "true"})
	existingDefaultTemplate.SetName("existing-default-template")
	mockResource.On("List", mock.Anything, v1.ListOptions{LabelSelector: "default=true"}).Return(&unstructured.UnstructuredList{Items: []unstructured.Unstructured{*existingDefaultTemplate}}, nil)

	// Mock the Update call to unlabel the existing default template
	mockResource.On("Update", mock.Anything, mock.Anything, v1.UpdateOptions{}).Return(nil, nil).Times(2)

	server := NewServer(wrapMockInterface(mockClient))
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

	// Reset mocks
	mockClient.AssertExpectations(t)
	mockResource.AssertExpectations(t)
	mockNamespaceableResource.AssertExpectations(t)
}

func createPutV2TemplatesNameStubServer(t *testing.T) *Server {
	unstructuredCluster := &unstructured.Unstructured{}
	unstructuredTemplate := &unstructured.UnstructuredList{}
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().Get(mock.Anything, mock.Anything, v1.GetOptions{}).Return(unstructuredCluster, nil).Maybe()
	resource.EXPECT().List(mock.Anything, mock.Anything).Return(unstructuredTemplate, nil).Maybe()
	resource.EXPECT().Update(mock.Anything, mock.Anything, v1.UpdateOptions{}).Return(nil, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: wrapMockInterface(mockedk8sclient),
	}
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
