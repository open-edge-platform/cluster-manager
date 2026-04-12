// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	"github.com/open-edge-platform/orch-library/go/pkg/tenancy"
)

type TenancyHandlerTestSuite struct {
	suite.Suite
	handler *TenancyHandler
}

func TestTenancyHandlerSuite(t *testing.T) {
	suite.Run(t, new(TenancyHandlerTestSuite))
}

func (suite *TenancyHandlerTestSuite) SetupTest() {
	k8sClient := k8s.New().WithFakeClient()
	require.NotNil(suite.T(), k8sClient)

	templates := []*v1alpha1.ClusterTemplate{}

	suite.handler = &TenancyHandler{k8s: k8sClient, templates: templates}
}

func (suite *TenancyHandlerTestSuite) TestNewTenancyHandler() {
	cases := []struct {
		name                           string
		k8sClientFunc                  func() *k8s.Client
		templateFunc                   func() ([]*v1alpha1.ClusterTemplate, error)
		podSecurityAdmissionConfigFunc func() (map[string][]byte, error)
		expectedErr                    string
		expectedHandler                bool
	}{
		{
			name: "success",
			k8sClientFunc: func() *k8s.Client {
				return k8s.New().WithFakeClient()
			},
			templateFunc: func() ([]*v1alpha1.ClusterTemplate, error) {
				return []*v1alpha1.ClusterTemplate{}, nil
			},
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) {
				return map[string][]byte{}, nil
			},
			expectedHandler: true,
		},
		{
			name: "k8s client error",
			k8sClientFunc: func() *k8s.Client {
				return nil
			},
			templateFunc:                   nil,
			podSecurityAdmissionConfigFunc: nil,
			expectedErr:                    "failed to get kubernetes client",
		},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			GetK8sClientFunc = tc.k8sClientFunc
			if tc.templateFunc != nil {
				GetTemplatesFunc = tc.templateFunc
			}
			if tc.podSecurityAdmissionConfigFunc != nil {
				GetPodSecurityAdmissionConfigFunc = tc.podSecurityAdmissionConfigFunc
			}

			handler, err := NewTenancyHandler()
			if tc.expectedHandler {
				assert.NotNil(suite.T(), handler)
				assert.NoError(suite.T(), err)
			} else {
				assert.Nil(suite.T(), handler)
				assert.ErrorContains(suite.T(), err, tc.expectedErr)
			}
		})
	}
}

func (suite *TenancyHandlerTestSuite) TestHandleEventProjectCreated() {
	projectID := uuid.New()
	event := tenancy.Event{
		EventType:    "created",
		ResourceType: "project",
		ResourceID:   projectID,
		ResourceName: "test-project",
	}

	ctx := context.Background()
	err := suite.handler.HandleEvent(ctx, event)
	assert.NoError(suite.T(), err)

	// Check that namespace was created
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err = suite.handler.k8s.Dyn.Resource(namespaceRes).Get(ctx, projectID.String(), metav1.GetOptions{})
	assert.NoError(suite.T(), err, "namespace should be created")
}

func (suite *TenancyHandlerTestSuite) TestHandleEventProjectDeleted() {
	projectID := uuid.New()

	ctx := context.Background()
	// First create the project resources
	createEvent := tenancy.Event{
		EventType:    "created",
		ResourceType: "project",
		ResourceID:   projectID,
		ResourceName: "test-project",
	}
	err := suite.handler.HandleEvent(ctx, createEvent)
	assert.NoError(suite.T(), err)

	// Then delete
	deleteEvent := tenancy.Event{
		EventType:    "deleted",
		ResourceType: "project",
		ResourceID:   projectID,
		ResourceName: "test-project",
	}
	err = suite.handler.HandleEvent(ctx, deleteEvent)
	assert.NoError(suite.T(), err)
}

func (suite *TenancyHandlerTestSuite) TestHandleEventIgnoresNonProject() {
	event := tenancy.Event{
		EventType:    "created",
		ResourceType: "org",
		ResourceID:   uuid.New(),
		ResourceName: "test-org",
	}

	ctx := context.Background()
	err := suite.handler.HandleEvent(ctx, event)
	assert.NoError(suite.T(), err)
}

func (suite *TenancyHandlerTestSuite) TestHandleEventIgnoresUnknownEventType() {
	event := tenancy.Event{
		EventType:    "updated",
		ResourceType: "project",
		ResourceID:   uuid.New(),
		ResourceName: "test-project",
	}

	ctx := context.Background()
	err := suite.handler.HandleEvent(ctx, event)
	assert.NoError(suite.T(), err)
}

func (suite *TenancyHandlerTestSuite) TestSetupProjectSetDefaultTemplate() {
	tmpDir := suite.T().TempDir()

	mockTemplateNames := []string{
		"mock-template-1",
		"mock-template-2",
		"mock-template-3",
		"mock-template-4",
		"mock-template-5",
	}

	mockTemplates := make([][]byte, len(mockTemplateNames))
	for i, name := range mockTemplateNames {
		mockTemplates[i] = []byte(fmt.Sprintf(`{"Name":"%s","Version":"v1.0.0","KubernetesVersion":"1.31"}`, name))
	}

	for i, tmpl := range mockTemplates {
		require.NoError(suite.T(), os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("mock-template%d.json", i+1)), tmpl, 0644))
	}

	oldEnv := os.Getenv("DEFAULT_TEMPLATES_DIR")
	require.NoError(suite.T(), os.Setenv("DEFAULT_TEMPLATES_DIR", tmpDir))
	suite.T().Cleanup(func() { os.Setenv("DEFAULT_TEMPLATES_DIR", oldEnv) })

	cases := []struct {
		name                           string
		k8sClientFunc                  func() *k8s.Client
		templateFunc                   func() ([]*v1alpha1.ClusterTemplate, error)
		podSecurityAdmissionConfigFunc func() (map[string][]byte, error)
		defaultTemplateName            string
		expectedErr                    error
		expectedHandler                bool
		expectedTemplateCount          int
		expectedDefault                string
	}{
		{
			name:                           "Default template is configured (should see all 5, mock-template-1 is default)",
			k8sClientFunc:                  func() *k8s.Client { return k8s.New().WithFakeClient() },
			templateFunc:                   func() ([]*v1alpha1.ClusterTemplate, error) { return template.ReadDefaultTemplates() },
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			defaultTemplateName:            "mock-template-1-v1.0.0",
			expectedErr:                    nil,
			expectedHandler:                true,
			expectedTemplateCount:          5,
			expectedDefault:                "mock-template-1-v1.0.0",
		},
		{
			name:                           "Default template is not configured (should see all 5, mock-template-1 is default)",
			k8sClientFunc:                  func() *k8s.Client { return k8s.New().WithFakeClient() },
			templateFunc:                   func() ([]*v1alpha1.ClusterTemplate, error) { return template.ReadDefaultTemplates() },
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			defaultTemplateName:            "",
			expectedErr:                    nil,
			expectedHandler:                true,
			expectedTemplateCount:          5,
			expectedDefault:                "mock-template-1-v1.0.0",
		},
		{
			name:                           "Default template is configured but not available (should see all 5, mock-template-1 is default)",
			k8sClientFunc:                  func() *k8s.Client { return k8s.New().WithFakeClient() },
			templateFunc:                   func() ([]*v1alpha1.ClusterTemplate, error) { return template.ReadDefaultTemplates() },
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			defaultTemplateName:            "not-available-template-v1.0.0",
			expectedErr:                    nil,
			expectedHandler:                true,
			expectedTemplateCount:          5,
			expectedDefault:                "mock-template-1-v1.0.0",
		},
		{
			name: "Valid default template exists in project namespace (should see all 5, valid-existing-v1.2.3 is default)",
			k8sClientFunc: func() *k8s.Client {
				client := k8s.New().WithFakeClient()
				assert.NotNil(suite.T(), client)

				client.Dyn.Resource(schema.GroupVersionResource{
					Group:    "edge-orchestrator.intel.com",
					Version:  "v1alpha1",
					Resource: "clustertemplates",
				}).Namespace("test-project-id").Create(context.Background(), &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "edge-orchestrator.intel.com/v1alpha1",
						"kind":       "ClusterTemplate",
						"metadata": map[string]interface{}{
							"name": "valid-existing-v1.2.3",
							"labels": map[string]interface{}{
								labels.DefaultLabelKey: labels.DefaultLabelVal,
							},
						},
						"spec": map[string]interface{}{
							"controlPlaneProviderType": "k3s",
							"kubernetesVersion":        "1.31",
						},
					},
				}, metav1.CreateOptions{})
				return client
			},
			templateFunc:                   func() ([]*v1alpha1.ClusterTemplate, error) { return template.ReadDefaultTemplates() },
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			defaultTemplateName:            "",
			expectedErr:                    nil,
			expectedHandler:                true,
			expectedTemplateCount:          5,
			expectedDefault:                "valid-existing-v1.2.3",
		},
		{
			name: "Invalid default template exists in project namespace (should see all 5, mock-template-1 is default)",
			k8sClientFunc: func() *k8s.Client {
				client := k8s.New().WithFakeClient()
				assert.NotNil(suite.T(), client)

				client.Dyn.Resource(schema.GroupVersionResource{
					Group:    "edge-orchestrator.intel.com",
					Version:  "v1alpha1",
					Resource: "clustertemplates",
				}).Namespace("test-project-id").Create(context.Background(), &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "edge-orchestrator.intel.com/v1alpha1",
						"kind":       "ClusterTemplate",
						"metadata": map[string]interface{}{
							"name": "invalid-existing-v1.2.3",
							"labels": map[string]interface{}{
								labels.DefaultLabelKey: labels.DefaultLabelVal,
							},
						},
						"spec": map[string]interface{}{
							"controlPlaneProviderType": "invalid",
							"kubernetesVersion":        "1.31",
						},
					},
				}, metav1.CreateOptions{})
				return client
			},
			templateFunc:                   func() ([]*v1alpha1.ClusterTemplate, error) { return template.ReadDefaultTemplates() },
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			defaultTemplateName:            "",
			expectedErr:                    nil,
			expectedHandler:                true,
			expectedTemplateCount:          5,
			expectedDefault:                "mock-template-1-v1.0.0",
		},
	}
	for _, tc := range cases {
		suite.Run(tc.name, func() {
			GetK8sClientFunc = tc.k8sClientFunc
			GetTemplatesFunc = tc.templateFunc
			GetPodSecurityAdmissionConfigFunc = tc.podSecurityAdmissionConfigFunc
			defaultTemplate = tc.defaultTemplateName

			handler, err := NewTenancyHandler()
			assert.NoError(suite.T(), err)
			assert.NotNil(suite.T(), handler)
			assert.Equal(suite.T(), tc.expectedTemplateCount, len(handler.templates))

			ctx := context.Background()
			projectID := "test-project-id"
			err = handler.setupProject(ctx, projectID, "test-project-display-name")
			assert.NoError(suite.T(), err)

			// Check templates were created
			for _, tmpl := range mockTemplateNames {
				assert.True(suite.T(), handler.k8s.HasTemplate(ctx, projectID, tmpl+"-v1.0.0"))
			}

			// Verify that the default template is set correctly
			tmpl, err := handler.k8s.DefaultTemplate(ctx, projectID)
			assert.NoError(suite.T(), err)
			assert.NotEmpty(suite.T(), tmpl, "default template should not be empty")
			assert.Equal(suite.T(), tc.expectedDefault, tmpl.GetName())
		})
	}
}

func (suite *TenancyHandlerTestSuite) TestSetupProject() {
	templates := []*v1alpha1.ClusterTemplate{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "privileged-k3s-v1.2.3"},
			Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.2.3"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "baseline-k3s-v1.2.3"},
			Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.2.3"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "restricted-k3s-v1.2.3"},
			Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.2.3"},
		},
	}

	psaData := map[string][]byte{
		"privileged": []byte("privileged-configs"),
		"baseline":   []byte("baseline-configs"),
		"restricted": []byte("restricted-configs"),
	}

	fakeK8s := k8s.New().WithFakeClient()

	h := &TenancyHandler{
		k8s:             fakeK8s,
		templates:       templates,
		defaultTemplate: "baseline-k3s-v1.2.3",
		psaData:         psaData,
	}

	ctx := context.Background()
	projectID := "test-project-id"
	err := h.setupProject(ctx, projectID, "test-project")
	assert.NoError(suite.T(), err)

	// Check that namespace was created
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err = fakeK8s.Dyn.Resource(namespaceRes).Get(ctx, projectID, metav1.GetOptions{})
	assert.NoError(suite.T(), err, "namespace should be created")

	// Check that PSA secret was created
	secretRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	secret, err := fakeK8s.Dyn.Resource(secretRes).Namespace(projectID).Get(ctx, "pod-security-admission-config", metav1.GetOptions{})
	assert.NoError(suite.T(), err, "secret should be created")

	// Check that secret has three items
	assert.Len(suite.T(), secret.Object["data"].(map[string]interface{}), 3)
}
