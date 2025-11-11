// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

var template1 = v1alpha1.ClusterTemplate{
	ObjectMeta: v1.ObjectMeta{
		Name:      "test-template-v0.0.1",
		Namespace: expectedActiveProjectID,
	},
	Spec: v1alpha1.ClusterTemplateSpec{
		ControlPlaneProviderType: "kubeadm",
		InfraProviderType:        "docker",
		KubernetesVersion:        "1.21.0",
		ClusterConfiguration:     "{\"fake\": \"config\"}",
	},
}

var templateInfo1 = api.TemplateInfo{
	Name:                     "test-template",
	Version:                  "v0.0.1",
	Controlplaneprovidertype: (*api.TemplateInfoControlplaneprovidertype)(ptr("kubeadm")),
	Infraprovidertype:        (*api.TemplateInfoInfraprovidertype)(ptr("docker")),
	KubernetesVersion:        "1.21.0",
	Clusterconfiguration: &map[string]interface{}{
		"fake": "config",
	},
}

var template2 = v1alpha1.ClusterTemplate{
	ObjectMeta: v1.ObjectMeta{
		Name: "test-template-v0.0.2",
	},
	Spec: v1alpha1.ClusterTemplateSpec{
		ControlPlaneProviderType: "kubeadm",
		InfraProviderType:        "docker",
		KubernetesVersion:        "1.33.0",
	},
}

var templateInfo2 = api.TemplateInfo{
	Name:                     "test-template",
	Version:                  "v0.0.2",
	Controlplaneprovidertype: (*api.TemplateInfoControlplaneprovidertype)(ptr("kubeadm")),
	Infraprovidertype:        (*api.TemplateInfoInfraprovidertype)(ptr("docker")),
	KubernetesVersion:        "1.33.0",
	Clusterconfiguration: &map[string]interface{}{
		"fake": "config",
	},
}

var template3 = v1alpha1.ClusterTemplate{
	ObjectMeta: v1.ObjectMeta{
		Name: "test-other-template-v0.0.2",
	},
	Spec: v1alpha1.ClusterTemplateSpec{
		ControlPlaneProviderType: "kubeadm",
		InfraProviderType:        "docker",
		KubernetesVersion:        "1.33.0",
	},
}

var templateInfo3 = api.TemplateInfo{
	Name:                     "test-other-template",
	Version:                  "v0.0.2",
	Controlplaneprovidertype: (*api.TemplateInfoControlplaneprovidertype)(ptr("kubeadm")),
	Infraprovidertype:        (*api.TemplateInfoInfraprovidertype)(ptr("docker")),
	KubernetesVersion:        "1.33.0",
	Clusterconfiguration: &map[string]interface{}{
		"fake": "config",
	},
}

var template4 = v1alpha1.ClusterTemplate{
	ObjectMeta: v1.ObjectMeta{
		Name: "test-other-template-v0.0.3",
	},
	Spec: v1alpha1.ClusterTemplateSpec{
		ControlPlaneProviderType: "kubeadm",
		InfraProviderType:        "docker",
		KubernetesVersion:        "1.33.0",
	},
}

var templateInfo4 = api.TemplateInfo{
	Name:                     "test-other-template",
	Version:                  "v0.0.3",
	Controlplaneprovidertype: (*api.TemplateInfoControlplaneprovidertype)(ptr("kubeadm")),
	Infraprovidertype:        (*api.TemplateInfoInfraprovidertype)(ptr("docker")),
	KubernetesVersion:        "1.33.0",
	Clusterconfiguration: &map[string]interface{}{
		"fake": "config",
	},
}

var defaultTemplate1 = v1alpha1.ClusterTemplate{
	ObjectMeta: v1.ObjectMeta{
		Name: "test-default-template-v0.0.3",
		Labels: map[string]string{
			"default": "true",
		},
	},
	Spec: v1alpha1.ClusterTemplateSpec{
		ControlPlaneProviderType: "kubeadm",
		InfraProviderType:        "docker",
		KubernetesVersion:        "1.21.0",
	},
}

var defaultTemplateInfo1 = api.TemplateInfo{
	Name:                     "test-default-template",
	Version:                  "v0.0.3",
	Controlplaneprovidertype: (*api.TemplateInfoControlplaneprovidertype)(ptr("kubeadm")),
	Infraprovidertype:        (*api.TemplateInfoInfraprovidertype)(ptr("docker")),
}

func createMockServerTemplates(t *testing.T, templates []v1alpha1.ClusterTemplate, activeProjectId string, expectedError error) *Server {
	unstructuredTemplates := make([]unstructured.Unstructured, len(templates))
	for i, template := range templates {
		unstructuredTemplate, err := convert.ToUnstructured(template)
		require.NoError(t, err, "convertClusterToUnstructured() error = %v, want nil")
		unstructuredTemplates[i] = *unstructuredTemplate
	}

	resource := k8s.NewMockResourceInterface(t)

	// adjust the mock expectation for getv2templatesnameversion 500 internal server when templates are nil
	if templates == nil {
		unstructuredTemplateList := &unstructured.UnstructuredList{
			Items: unstructuredTemplates,
		}
		resource.EXPECT().List(mock.Anything, mock.Anything).Return(unstructuredTemplateList, expectedError)
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(activeProjectId).Return(resource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)
		return NewServer(mockedk8sclient)
	}

	// Expectation for when no label selector is applied (return all templates)
	resource.EXPECT().List(mock.Anything, mock.MatchedBy(func(opts v1.ListOptions) bool {
		return opts.LabelSelector == ""
	})).Return(&unstructured.UnstructuredList{Items: unstructuredTemplates}, nil).Maybe()

	// Expectation for when the default label selector is applied (return only the default template)
	resource.EXPECT().List(mock.Anything, mock.MatchedBy(func(opts v1.ListOptions) bool {
		selector, err := v1.LabelSelectorAsSelector(&v1.LabelSelector{
			MatchLabels: map[string]string{"default": "true"},
		})
		require.NoError(t, err)
		return selector.Matches(labels.Set{"default": "true"})
	})).Return(&unstructured.UnstructuredList{Items: filterDefaultTemplates(unstructuredTemplates)}, nil).Maybe()

	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(activeProjectId).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource).Maybe()

	return NewServer(mockedk8sclient)
}

func filterDefaultTemplates(templates []unstructured.Unstructured) []unstructured.Unstructured {
	var defaultTemplates []unstructured.Unstructured
	for _, template := range templates {
		if template.GetLabels()["default"] == "true" {
			defaultTemplates = append(defaultTemplates, template)
		}
	}
	return defaultTemplates
}

func TestGetV2Templates200(t *testing.T) {
	expectedActiveProjectID := "ae9007c6-dcac-11ef-98fb-2fe5d3d8a8b4"
	t.Run("No Templates", func(t *testing.T) {
		templates := []v1alpha1.ClusterTemplate{}
		server := createMockServerTemplates(t, templates, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Empty(t, response.TemplateInfoList, "TemplateInfoList should be empty")
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("One Template - No Defaults", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 1, "TemplateInfoList should have 1 item")
		require.Equal(t, templateInfo1.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, template1.Name)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})
	t.Run("Three templates - filter name: two", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=name=test-template", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 2, "TemplateInfoList should have 2 items")
		require.Equal(t, templateInfo1.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, template1.Name)
		require.Equal(t, templateInfo2.Name, (*response.TemplateInfoList)[1].Name, "TemplateInfoList[1].Name = %v, want %v", (*response.TemplateInfoList)[1].Name, template2.Name)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Three templates - filter name: one", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=name=test-other-template", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 1, "TemplateInfoList should have 1 item")
		require.Equal(t, templateInfo3.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, template3.Name)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Three templates - filter k8s version", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=kubernetesVersion=1.33.0", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 2, "TemplateInfoList should have 2 items")
		require.Equal(t, templateInfo2.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, templateInfo2.Name)
		require.Equal(t, templateInfo3.Name, (*response.TemplateInfoList)[1].Name, "TemplateInfoList[1].Name = %v, want %v", (*response.TemplateInfoList)[1].Name, templateInfo3.Name)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Three templates - filter k8s version - partial search", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=kubernetesVersion=1.33", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 2, "TemplateInfoList should have 2 items")
		require.Equal(t, templateInfo2.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, templateInfo2.Name)
		require.Equal(t, templateInfo3.Name, (*response.TemplateInfoList)[1].Name, "TemplateInfoList[1].Name = %v, want %v", (*response.TemplateInfoList)[1].Name, templateInfo3.Name)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Four templates - filter k8s version with template name and order by template version (default ascending)", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3, template4}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=kubernetesVersion=1.33.0%20AND%20name=test-other-template&orderBy=version", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 2, "TemplateInfoList should have 2 items")
		require.Equal(t, templateInfo3.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, templateInfo3.Name)
		require.Equal(t, templateInfo4.Name, (*response.TemplateInfoList)[1].Name, "TemplateInfoList[1].Name = %v, want %v", (*response.TemplateInfoList)[1].Name, templateInfo4.Name)
		require.Equal(t, templateInfo3.Version, (*response.TemplateInfoList)[0].Version, "TemplateInfoList[0].Version = %v, want %v", (*response.TemplateInfoList)[0].Version, templateInfo3.Version)
		require.Equal(t, templateInfo4.Version, (*response.TemplateInfoList)[1].Version, "TemplateInfoList[1].Version = %v, want %v", (*response.TemplateInfoList)[1].Version, templateInfo4.Version)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Four templates - filter k8s version with template name and order by template version in descending order", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3, template4}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=kubernetesVersion=1.33.0%20AND%20name=test-other-template&orderBy=version%20desc", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 2, "TemplateInfoList should have 2 items")
		require.Equal(t, templateInfo4.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, templateInfo4.Name)
		require.Equal(t, templateInfo3.Name, (*response.TemplateInfoList)[1].Name, "TemplateInfoList[1].Name = %v, want %v", (*response.TemplateInfoList)[1].Name, templateInfo3.Name)
		require.Equal(t, templateInfo4.Version, (*response.TemplateInfoList)[0].Version, "TemplateInfoList[0].Version = %v, want %v", (*response.TemplateInfoList)[0].Version, templateInfo4.Version)
		require.Equal(t, templateInfo3.Version, (*response.TemplateInfoList)[1].Version, "TemplateInfoList[1].Version = %v, want %v", (*response.TemplateInfoList)[1].Version, templateInfo3.Version)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Three templates - filter partial name: three", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=name=mpla", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 3, "TemplateInfoList should have 3 items")
		require.Equal(t, templateInfo1.Name, (*response.TemplateInfoList)[0].Name, "TemplateInfoList[0].Name = %v, want %v", (*response.TemplateInfoList)[0].Name, template1.Name)
		require.Equal(t, templateInfo2.Name, (*response.TemplateInfoList)[1].Name, "TemplateInfoList[1].Name = %v, want %v", (*response.TemplateInfoList)[1].Name, template2.Name)
		require.Equal(t, templateInfo3.Name, (*response.TemplateInfoList)[2].Name, "TemplateInfoList[2].Name = %v, want %v", (*response.TemplateInfoList)[2].Name, template3.Name)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Three templates - filter version: one", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=version=v0.0.1", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 1, "TemplateInfoList should have 1 item")
		require.Equal(t, templateInfo1.Version, (*response.TemplateInfoList)[0].Version, "TemplateInfoList[0].Version = %v, want %v", (*response.TemplateInfoList)[0].Version, templateInfo1.Version)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("Three templates - filter version: two", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates?filter=version=v0.0.2", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates200JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusOK, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 200)
		require.Len(t, *response.TemplateInfoList, 2, "TemplateInfoList should have 2 item")
		require.Equal(t, templateInfo2.Version, (*response.TemplateInfoList)[0].Version, "TemplateInfoList[0].Version = %v, want %v", (*response.TemplateInfoList)[0].Version, templateInfo2.Version)
		require.Equal(t, templateInfo3.Version, (*response.TemplateInfoList)[1].Version, "TemplateInfoList[0].Version = %v, want %v", (*response.TemplateInfoList)[1].Version, templateInfo3.Version)
		require.Empty(t, response.DefaultTemplateInfo, "DefaultTemplateInfo should be empty")
	})

	t.Run("three templates not matching filter", func(t *testing.T) {
		server := createMockServerTemplates(t, []v1alpha1.ClusterTemplate{template1, template2, template3}, expectedActiveProjectID, nil)
		require.NotNil(t, server)

		// create request and recorer
		req := httptest.NewRequest("GET", "/v2/templates?filter=kubernetesVersion=v1.23.0", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rec := httptest.NewRecorder()

		// create a handler and serve request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rec, req)

		// parse response
		var response api.GetV2Templates200JSONResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))

		// verify response
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, response.TotalElements)
	})
}

func TestGetV2ClustersDefault(t *testing.T) {
	tests := []struct {
		name               string
		templates          []v1alpha1.ClusterTemplate
		queryParams        map[string]string
		expectedStatusCode int
		expectedLen        int
		expectedNames      []string
		expectedDefault    *string
	}{
		{
			name:               "no default templates",
			templates:          []v1alpha1.ClusterTemplate{},
			queryParams:        map[string]string{"default": "true"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        0,
			expectedNames:      []string{},
			expectedDefault:    nil,
		},
		{
			name:               "one default template",
			templates:          []v1alpha1.ClusterTemplate{defaultTemplate1},
			queryParams:        map[string]string{"default": "true"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        1,
			expectedNames:      []string{defaultTemplateInfo1.Name},
			expectedDefault:    &defaultTemplateInfo1.Name,
		},
		{
			name:               "three templates - one default",
			templates:          []v1alpha1.ClusterTemplate{defaultTemplate1, template1, template2},
			queryParams:        map[string]string{"default": "true"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        3,
			expectedNames:      []string{defaultTemplateInfo1.Name, templateInfo1.Name, templateInfo2.Name},
			expectedDefault:    &defaultTemplateInfo1.Name,
		},
		{
			name:               "three Templates (one default) asc ordering",
			templates:          []v1alpha1.ClusterTemplate{defaultTemplate1, template1, template2},
			queryParams:        map[string]string{"default": "true", "orderBy": "name"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        3,
			expectedNames:      []string{defaultTemplateInfo1.Name, templateInfo1.Name, templateInfo2.Name},
			expectedDefault:    &defaultTemplateInfo1.Name,
		},
		{
			name:               "three Templates (one Default) desc ordering",
			templates:          []v1alpha1.ClusterTemplate{defaultTemplate1, template1, template2},
			queryParams:        map[string]string{"default": "true", "orderBy": "name desc"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        3,
			expectedNames:      []string{templateInfo1.Name, templateInfo2.Name, defaultTemplateInfo1.Name},
			expectedDefault:    &defaultTemplateInfo1.Name,
		},
		{
			name:               "paginated 4 templates (one Default) offset1",
			templates:          []v1alpha1.ClusterTemplate{defaultTemplate1, template1, template2, template3},
			queryParams:        map[string]string{"default": "true", "pageSize": "2", "offset": "1"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        2,
			expectedNames:      []string{templateInfo1.Name, templateInfo2.Name},
			expectedDefault:    &defaultTemplateInfo1.Name,
		},
		{
			name:               "paginated 4 templates (one Default) offset2",
			templates:          []v1alpha1.ClusterTemplate{defaultTemplate1, template1, template2, template3},
			queryParams:        map[string]string{"default": "true", "pageSize": "2", "offset": "2"},
			expectedStatusCode: http.StatusOK,
			expectedLen:        2,
			expectedNames:      []string{templateInfo2.Name, templateInfo3.Name},
			expectedDefault:    &defaultTemplateInfo1.Name,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := createMockServerTemplates(t, tt.templates, expectedActiveProjectID, nil)
			require.NotNil(t, server, "NewServer() returned nil, want not nil")

			// Create a new request & response recorder
			req := httptest.NewRequest("GET", "/v2/templates", nil)
			req.Header.Set("Activeprojectid", expectedActiveProjectID)

			q := req.URL.Query()
			for key, value := range tt.queryParams {
				q.Add(key, value)
			}
			req.URL.RawQuery = q.Encode()

			rr := httptest.NewRecorder()

			// create a handler with middleware to serve the request
			handler, err := server.ConfigureHandler()
			require.Nil(t, err)
			handler.ServeHTTP(rr, req)

			// parse the response body
			var response api.GetV2Templates200JSONResponse
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			require.NoError(t, err, "Failed to unmarshal response body")

			// check the response status
			require.Equal(t, tt.expectedStatusCode, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, tt.expectedStatusCode)
			if tt.expectedStatusCode != http.StatusOK {
				return
			}
			require.Len(t, *response.TemplateInfoList, tt.expectedLen, "TemplateInfoList should have %d items", tt.expectedLen)

			for i, name := range tt.expectedNames {
				require.Equal(t, name, (*response.TemplateInfoList)[i].Name, "TemplateInfoList[%d].Name = %v, want %v", i, (*response.TemplateInfoList)[i].Name, name)
			}

			if tt.expectedDefault != nil {
				require.NotNil(t, response.DefaultTemplateInfo)
				require.Equal(t, *tt.expectedDefault, *response.DefaultTemplateInfo.Name, "DefaultTemplateInfo.Name = %v, want %v", *response.DefaultTemplateInfo.Name, *tt.expectedDefault)
			} else {
				require.Nil(t, response.DefaultTemplateInfo)
			}
		})
	}
}

func TestGetV2Templates400(t *testing.T) {
	t.Run("No Active Project ID", func(t *testing.T) {
		mockedk8sclient := k8s.NewMockInterface(t)
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates", nil)
		// req.Header.Set("Activeprojectid", expectedActiveProjectID)
		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates400JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusBadRequest, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, 400)
		require.NotEmpty(t, response.Message, "Message should not be empty")
	})
}

func TestGetV2Templates500(t *testing.T) {
	t.Run("Internal Server Error", func(t *testing.T) {
		expectedActiveProjectID := "655a6892-4280-4c37-97b1-31161ac0b99e"
		expectedErrorMessage := "Internal server error"

		// configure mockery to return an internal server error on a List() call
		resource := k8s.NewMockResourceInterface(t)
		resource.EXPECT().List(mock.Anything, mock.Anything).Return(nil, &errors.StatusError{
			ErrStatus: v1.Status{
				Status:  v1.StatusFailure,
				Code:    http.StatusInternalServerError,
				Reason:  v1.StatusReasonInternalError,
				Message: expectedErrorMessage,
			},
		})
		nsResource := k8s.NewMockNamespaceableResourceInterface(t)
		nsResource.EXPECT().Namespace(expectedActiveProjectID).Return(resource)
		mockedk8sclient := k8s.NewMockInterface(t)
		mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource)

		// create a new server with the mocked mockedk8sclient
		server := NewServer(mockedk8sclient)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// create a handler with middleware
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)

		// create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		// serve the request
		handler.ServeHTTP(rr, req)

		// check the response status
		require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)

		// check the response body
		var respbody api.GetV2Templates500JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &respbody)
		require.NoError(t, err, "json.Unmarshal() error = %v, want nil", err)
		require.Contains(t, *respbody.Message, expectedErrorMessage, "ServeHTTP() body = %v, want %v", *respbody.Message, expectedErrorMessage)
	})

	t.Run("Too Many Default Templates", func(t *testing.T) {
		templates := []v1alpha1.ClusterTemplate{defaultTemplate1, defaultTemplate1}
		server := createMockServerTemplates(t, templates, expectedActiveProjectID, nil)
		require.NotNil(t, server, "NewServer() returned nil, want not nil")

		// Create a new request & response recorder
		req := httptest.NewRequest("GET", "/v2/templates", nil)
		req.Header.Set("Activeprojectid", expectedActiveProjectID)

		q := req.URL.Query()
		q.Add("default", "true")
		req.URL.RawQuery = q.Encode()

		rr := httptest.NewRecorder()

		// create a handler with middleware to serve the request
		handler, err := server.ConfigureHandler()
		require.Nil(t, err)
		handler.ServeHTTP(rr, req)

		// Parse the response body
		var response api.GetV2Templates500JSONResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err, "Failed to unmarshal response body")

		// Check the response status
		require.Equal(t, http.StatusInternalServerError, rr.Code, "ServeHTTP() status = %v, want %v", rr.Code, http.StatusInternalServerError)
		require.NotEmpty(t, response.Message, "Message should not be empty")
	})
}

func createGetV2TemplatesStubServer(t *testing.T) *Server {
	unstructuredTemplates := make([]unstructured.Unstructured, 0)
	unstructuredTemplateList := &unstructured.UnstructuredList{
		Items: unstructuredTemplates,
	}
	resource := k8s.NewMockResourceInterface(t)
	resource.EXPECT().List(mock.Anything, mock.Anything).Return(unstructuredTemplateList, nil).Maybe()
	nsResource := k8s.NewMockNamespaceableResourceInterface(t)
	nsResource.EXPECT().Namespace(mock.Anything).Return(resource).Maybe()
	mockedk8sclient := k8s.NewMockInterface(t)
	mockedk8sclient.EXPECT().Resource(core.TemplateResourceSchema).Return(nsResource).Maybe()
	return &Server{
		k8sclient: mockedk8sclient,
	}
}

func FuzzGetV2Templates(f *testing.F) {
	f.Add(true, byte(0), byte(1), byte(2), byte(3), byte(4), byte(5), byte(6), byte(7),
		byte(8), byte(9), byte(10), byte(11), byte(12), byte(13), byte(14), byte(15))
	f.Fuzz(func(t *testing.T, b bool, u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15 byte) {
		server := createGetV2TemplatesStubServer(t)
		uuid := [16]byte{u0, u1, u2, u3, u4, u5, u6, u7, u8, u9, u10, u11, u12, u13, u14, u15}
		activeprojectid := api.ActiveProjectIdHeader(openapi_types.UUID(uuid))
		req := api.GetV2TemplatesRequestObject{
			Params: api.GetV2TemplatesParams{
				Default:         &b,
				Activeprojectid: activeprojectid,
			},
		}
		_, _ = server.GetV2Templates(context.Background(), req)
	})
}
