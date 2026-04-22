// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"errors"
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

	"github.com/open-edge-platform/orch-library/go/pkg/tenancy"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
)

type TenancyDatamodelTestSuite struct {
	suite.Suite
	t *TenancyDatamodel
}

func TestTenancyDatamodelSuite(t *testing.T) {
	suite.Run(t, new(TenancyDatamodelTestSuite))
}

func (suite *TenancyDatamodelTestSuite) SetupTest() {
	k8sClient := k8s.New().WithFakeClient()
	require.NotNil(suite.T(), k8sClient)

	suite.t = &TenancyDatamodel{
		k8s:       k8sClient,
		templates: []*v1alpha1.ClusterTemplate{},
	}
}

func (suite *TenancyDatamodelTestSuite) TestNewDatamodelClient() {
	cases := []struct {
		name                           string
		k8sClientFunc                  func() *k8s.Client
		templateFunc                   func() ([]*v1alpha1.ClusterTemplate, error)
		podSecurityAdmissionConfigFunc func() (map[string][]byte, error)
		expectedErr                    error
		expectedClient                 bool
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
			expectedErr:    nil,
			expectedClient: true,
		},
		{
			name: "k8s client error",
			k8sClientFunc: func() *k8s.Client {
				return nil
			},
			templateFunc:                   nil,
			podSecurityAdmissionConfigFunc: nil,
			expectedErr:                    errors.New("failed to get kubernetes client"),
			expectedClient:                 false,
		},
		{
			name: "templates error",
			k8sClientFunc: func() *k8s.Client {
				return k8s.New().WithFakeClient()
			},
			templateFunc: func() ([]*v1alpha1.ClusterTemplate, error) {
				return nil, errors.New("template read error")
			},
			podSecurityAdmissionConfigFunc: nil,
			expectedErr:                    errors.New("failed to read cluster templates: template read error"),
			expectedClient:                 false,
		},
		{
			name: "psa config error",
			k8sClientFunc: func() *k8s.Client {
				return k8s.New().WithFakeClient()
			},
			templateFunc: func() ([]*v1alpha1.ClusterTemplate, error) {
				return []*v1alpha1.ClusterTemplate{}, nil
			},
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) {
				return nil, errors.New("psa read error")
			},
			expectedErr:    errors.New("failed to read pod security admission configs: psa read error"),
			expectedClient: false,
		},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			GetK8sClientFunc = tc.k8sClientFunc
			GetTemplatesFunc = tc.templateFunc
			GetPodSecurityAdmissionConfigFunc = tc.podSecurityAdmissionConfigFunc

			client, err := NewDatamodelClient()
			if tc.expectedClient {
				assert.NotNil(suite.T(), client)
			} else {
				assert.Nil(suite.T(), client)
			}

			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestHandleEvent() {
	projectID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	projectName := "test-project"

	oneTemplate := []*v1alpha1.ClusterTemplate{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "baseline-k3s-v1.0.0"},
			Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.0.0"},
		},
	}

	cases := []struct {
		name          string
		event         tenancy.Event
		expectedErr   error
		skipPreCreate bool // if true, skip pre-creating namespace for delete events
		// verifier checks side-effects on the fake k8s client after HandleEvent returns
		verifier func(ctx context.Context, fakeK8s *k8s.Client)
	}{
		{
			name: "created event sets up project namespace and templates",
			event: tenancy.Event{
				ID:           1,
				EventType:    tenancy.EventTypeCreated,
				ResourceType: tenancy.ResourceTypeProject,
				ResourceID:   projectID,
				ResourceName: projectName,
			},
			expectedErr: nil,
			verifier: func(ctx context.Context, fakeK8s *k8s.Client) {
				nsRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
				_, err := fakeK8s.Dyn.Resource(nsRes).Get(ctx, projectID.String(), metav1.GetOptions{})
				assert.NoError(suite.T(), err, "namespace should be created")
			},
		},
		{
			name: "deleted event cleans up a previously created project namespace",
			event: tenancy.Event{
				ID:           2,
				EventType:    tenancy.EventTypeDeleted,
				ResourceType: tenancy.ResourceTypeProject,
				ResourceID:   projectID,
				ResourceName: projectName,
			},
			expectedErr: nil,
			// The fake k8s client returns not-found on deletes of non-existent resources;
			// we pre-create the namespace in the per-case setup below.
			verifier: nil,
		},
		{
			name: "org event is ignored",
			event: tenancy.Event{
				ID:           3,
				EventType:    tenancy.EventTypeCreated,
				ResourceType: tenancy.ResourceTypeOrg,
				ResourceID:   projectID,
				ResourceName: "test-org",
			},
			expectedErr: nil,
			verifier:    nil,
		},
		{
			name: "unknown event type is ignored",
			event: tenancy.Event{
				ID:           4,
				EventType:    "unknown",
				ResourceType: tenancy.ResourceTypeProject,
				ResourceID:   projectID,
				ResourceName: projectName,
			},
			expectedErr: nil,
			verifier:    nil,
		},
		{
			name: "zero uuid project event is rejected",
			event: tenancy.Event{
				ID:           5,
				EventType:    tenancy.EventTypeCreated,
				ResourceType: tenancy.ResourceTypeProject,
				ResourceID:   uuid.UUID{}, // zero value
				ResourceName: "bad-project",
			},
			expectedErr: fmt.Errorf("received tenancy event with zero project UUID (event_id=5, type=created)"),
			verifier:    nil,
		},
		{
			name: "deleted event is idempotent when resources are already absent",
			event: tenancy.Event{
				ID:           6,
				EventType:    tenancy.EventTypeDeleted,
				ResourceType: tenancy.ResourceTypeProject,
				ResourceID:   projectID,
				ResourceName: projectName,
			},
			expectedErr:   nil,
			skipPreCreate: true, // namespace was never created; cleanup must still succeed
			verifier:      nil,
		},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			fakeK8s := k8s.New().WithFakeClient()
			handler := &TenancyDatamodel{
				k8s:       fakeK8s,
				templates: oneTemplate,
				psaData:   map[string][]byte{},
			}

			ctx := context.Background()

			// For the delete test, pre-create the namespace so cleanup doesn't fail
			// (unless skipPreCreate is set, which tests the idempotent-delete path).
			if !tc.skipPreCreate && tc.event.EventType == tenancy.EventTypeDeleted && tc.event.ResourceType == tenancy.ResourceTypeProject {
				require.NoError(suite.T(), fakeK8s.CreateNamespace(ctx, tc.event.ResourceID.String()))
			}

			err := handler.HandleEvent(ctx, tc.event)

			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}

			if tc.verifier != nil {
				tc.verifier(ctx, fakeK8s)
			}
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestSetupProjectSetDefaultTemplate() {
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
		expectedClient                 bool
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
			expectedClient:                 true,
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
			expectedClient:                 true,
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
			expectedClient:                 true,
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
			expectedClient:                 true,
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
			expectedClient:                 true,
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

			client, err := NewDatamodelClient()
			assert.NoError(suite.T(), err)
			assert.NotNil(suite.T(), client)
			assert.Equal(suite.T(), tc.expectedTemplateCount, len(client.templates))

			ctx := context.Background()
			err = client.setupProject(ctx, "test-project-id", "test-project")
			assert.NoError(suite.T(), err)

			// Check templates were created
			for _, tmpl := range mockTemplateNames {
				assert.True(suite.T(), client.k8s.HasTemplate(ctx, "test-project-id", tmpl+"-v1.0.0"))
			}

			// Verify that the default template is set correctly
			tmpl, err := client.k8s.DefaultTemplate(ctx, "test-project-id")
			assert.NoError(suite.T(), err)
			assert.NotEmpty(suite.T(), tmpl, "default template should not be empty")
			assert.Equal(suite.T(), tc.expectedDefault, tmpl.GetName())
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestSetupProject() {
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

	t := &TenancyDatamodel{
		k8s:             fakeK8s,
		templates:       templates,
		defaultTemplate: "baseline-k3s-v1.2.3",
		psaData:         psaData,
	}

	ctx := context.Background()
	err := t.setupProject(ctx, "test-project-id", "test-project")
	assert.NoError(suite.T(), err)

	// Check that namespace was created
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err = fakeK8s.Dyn.Resource(namespaceRes).Get(ctx, "test-project-id", metav1.GetOptions{})
	assert.NoError(suite.T(), err, "namespace should be created")

	// Check that PSA secret was created
	secretRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	secret, err := fakeK8s.Dyn.Resource(secretRes).Namespace("test-project-id").Get(ctx, "pod-security-admission-config", metav1.GetOptions{})
	assert.NoError(suite.T(), err, "secret should be created")

	// Check that secret has three items
	assert.Len(suite.T(), secret.Object["data"].(map[string]interface{}), 3)
}

// TestHandleEventIdempotent verifies that HandleEvent for a "created" project can be called
// multiple times without error (e.g. during poller replay on startup).
func (suite *TenancyDatamodelTestSuite) TestHandleEventIdempotent() {
	projectID := uuid.MustParse("cccccccc-dddd-eeee-ffff-000000000000")

	oneTemplate := []*v1alpha1.ClusterTemplate{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "baseline-k3s-v1.0.0"},
			Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.0.0"},
		},
	}

	fakeK8s := k8s.New().WithFakeClient()
	handler := &TenancyDatamodel{
		k8s:       fakeK8s,
		templates: oneTemplate,
		psaData:   map[string][]byte{},
	}

	ctx := context.Background()
	event := tenancy.Event{
		ID:           1,
		EventType:    tenancy.EventTypeCreated,
		ResourceType: tenancy.ResourceTypeProject,
		ResourceID:   projectID,
		ResourceName: "idempotent-project",
	}

	// First call creates the project resources.
	require.NoError(suite.T(), handler.HandleEvent(ctx, event))

	// Second call (replay) must be a no-op and must not return an error.
	assert.NoError(suite.T(), handler.HandleEvent(ctx, event))
}

// TestCleanupProject verifies cleanupProject behaviour, including idempotency when
// the project namespace was already removed.
func (suite *TenancyDatamodelTestSuite) TestCleanupProject() {
	projectID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	cases := []struct {
		name        string
		setup       func(ctx context.Context, fakeK8s *k8s.Client)
		expectedErr error
	}{
		{
			name: "cleanup succeeds when namespace exists",
			setup: func(ctx context.Context, fakeK8s *k8s.Client) {
				require.NoError(suite.T(), fakeK8s.CreateNamespace(ctx, projectID))
			},
			expectedErr: nil,
		},
		{
			name:        "cleanup is idempotent when namespace does not exist",
			setup:       func(_ context.Context, _ *k8s.Client) {},
			expectedErr: nil,
		},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			fakeK8s := k8s.New().WithFakeClient()
			h := &TenancyDatamodel{k8s: fakeK8s}

			ctx := context.Background()
			tc.setup(ctx, fakeK8s)

			err := h.cleanupProject(ctx, projectID, "test-project")
			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
}

// TestSetupProjectError verifies that setupProject returns an error when no templates
// are available (setDefaultTemplate cannot pick a default).
func (suite *TenancyDatamodelTestSuite) TestSetupProjectError() {
	suite.Run("no templates available returns error", func() {
		fakeK8s := k8s.New().WithFakeClient()
		h := &TenancyDatamodel{
			k8s:       fakeK8s,
			templates: []*v1alpha1.ClusterTemplate{}, // empty — no templates
			psaData:   map[string][]byte{},
		}

		ctx := context.Background()
		err := h.setupProject(ctx, "test-project-id", "test-project")
		assert.EqualError(suite.T(), err,
			"failed to set default template for project 'test-project': no templates available to set as default for project test-project-id")
	})
}


