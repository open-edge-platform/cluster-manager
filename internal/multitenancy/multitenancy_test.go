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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	activeWatcher "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectactivewatcher.edge-orchestrator.intel.com/v1"
	watcherv1 "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectwatcher.edge-orchestrator.intel.com/v1"
	projectv1 "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/runtimeproject.edge-orchestrator.intel.com/v1"
	nexus "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/nexus-client"
)

type TenancyDatamodelTestSuite struct {
	suite.Suite
	t *TenancyDatamodel
}

func TestTenancyDatamodelSuite(t *testing.T) {
	suite.Run(t, new(TenancyDatamodelTestSuite))
}

func (suite *TenancyDatamodelTestSuite) SetupTest() {
	nexus := nexus.NewFakeClient()
	require.NotNil(suite.T(), nexus)

	k8s := k8s.New().WithFakeClient()
	require.NotNil(suite.T(), k8s)

	templates := []*v1alpha1.ClusterTemplate{}

	suite.t = &TenancyDatamodel{nexus: nexus, k8s: k8s, templates: templates}
}

func (suite *TenancyDatamodelTestSuite) TestStart() {
	err := suite.t.Start()
	assert.NoError(suite.T(), err)
}

func (suite *TenancyDatamodelTestSuite) TestStop() {
	suite.t.Stop()
}

func (suite *TenancyDatamodelTestSuite) TestNewDatamodelClient() {
	cases := []struct {
		name                           string
		configFunc                     func() (*rest.Config, error)
		clientSetFunc                  func(*rest.Config) (*nexus.Clientset, error)
		k8sClientFunc                  func() *k8s.Client
		templateFunc                   func() ([]*v1alpha1.ClusterTemplate, error)
		podSecurityAdmissionConfigFunc func() (map[string][]byte, error)
		expectedErr                    error
		expectedClient                 bool
	}{
		{
			name: "success",
			configFunc: func() (*rest.Config, error) {
				return &rest.Config{}, nil
			},
			clientSetFunc: func(*rest.Config) (*nexus.Clientset, error) {
				return nexus.NewFakeClient(), nil
			},
			k8sClientFunc: func() *k8s.Client {
				return k8s.New().WithFakeClient()
			},
			templateFunc: func() ([]*v1alpha1.ClusterTemplate, error) {
				templates := []*v1alpha1.ClusterTemplate{}
				return templates, nil
			},
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) {
				return map[string][]byte{}, nil
			},
			expectedErr:    nil,
			expectedClient: true,
		},
		{
			name: "config error",
			configFunc: func() (*rest.Config, error) {
				return nil, errors.New("config error")
			},
			clientSetFunc:                  nil,
			k8sClientFunc:                  nil,
			templateFunc:                   nil,
			podSecurityAdmissionConfigFunc: nil,
			expectedErr:                    errors.New("failed to get orch kubernetes config: config error"),
			expectedClient:                 false,
		},
		{
			name: "clientset error",
			configFunc: func() (*rest.Config, error) {
				return &rest.Config{}, nil
			},
			clientSetFunc: func(*rest.Config) (*nexus.Clientset, error) {
				return nil, errors.New("client set error")
			},
			k8sClientFunc:                  nil,
			templateFunc:                   nil,
			podSecurityAdmissionConfigFunc: nil,
			expectedErr:                    errors.New("failed to create nexus client: client set error"),
			expectedClient:                 false,
		},
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			GetClusterConfigFunc = tc.configFunc
			GetNexusClientSetFunc = tc.clientSetFunc
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

func (suite *TenancyDatamodelTestSuite) TestAddProjectWatcher() {
	cases := []struct {
		name        string
		expectedErr error
	}{
		{
			name:        "success",
			expectedErr: nil,
		},
		{
			name:        "already exists", // watcher already exists from previous test case
			expectedErr: nil,
		},
		// TODO add error case
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			err := suite.t.addProjectWatcher()
			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestDeleteProjectWatcher() {
	cases := []struct {
		name        string
		setupClient func()
		expectedErr error
	}{
		{
			name: "success",
			setupClient: func() {
				_, err := suite.t.nexus.TenancyMultiTenancy().Config().AddProjectWatchers(context.Background(),
					&watcherv1.ProjectWatcher{ObjectMeta: metav1.ObjectMeta{Name: appName}})
				require.Nil(suite.T(), err)
			},
			expectedErr: nil,
		},
		{
			name:        "not found",
			setupClient: func() {},
			expectedErr: nil,
		},
		// TODO add error case
	}

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			tc.setupClient()
			err := suite.t.deleteProjectWatcher()
			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestAddCallback() {
	cases := []struct {
		name        string
		project     *projectv1.RuntimeProject
		expectedErr error
	}{
		{
			name: "success",
			project: &projectv1.RuntimeProject{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-project",
				},
				Spec: projectv1.RuntimeProjectSpec{
					Deleted: false,
				},
				Status: projectv1.RuntimeProjectNexusStatus{},
			},
			expectedErr: nil,
		},
		// {
		// 	name: "deleted-project",
		// 	project: &projectv1.RuntimeProject{
		// 		TypeMeta: metav1.TypeMeta{},
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name: "test-project2",
		// 		},
		// 		Spec: projectv1.RuntimeProjectSpec{
		// 			Deleted: true,
		// 		},
		// 		Status: projectv1.RuntimeProjectNexusStatus{},
		// 	},
		// 	expectedErr: nil,
		// },
	}

	suite.t.addProjectWatcher()

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			ctx := context.Background()
			_, err := suite.t.nexus.Runtimeproject().CreateRuntimeProjectByName(ctx, tc.project)
			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestDeleteCallback() {
	cases := []struct {
		name        string
		project     *projectv1.RuntimeProject
		expectedErr error
	}{
		{
			name: "success",
			project: &projectv1.RuntimeProject{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-project",
				},
				Spec: projectv1.RuntimeProjectSpec{
					Deleted: true,
				},
				Status: projectv1.RuntimeProjectNexusStatus{},
			},
			expectedErr: nil,
		},
	}

	suite.t.addProjectWatcher()

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			ctx := context.Background()
			project, _ := suite.t.nexus.Runtimeproject().CreateRuntimeProjectByName(ctx, tc.project)
			project.AddActiveWatchers(ctx, &activeWatcher.ProjectActiveWatcher{
				ObjectMeta: metav1.ObjectMeta{Name: appName},
				Spec: activeWatcher.ProjectActiveWatcherSpec{
					StatusIndicator: activeWatcher.StatusIndicationInProgress,
					Message:         fmt.Sprintf("%s subscribed to project %s", appName, project.DisplayName()),
					TimeStamp:       safeUnixTime(),
				},
			})
			err := suite.t.nexus.Runtimeproject().DeleteRuntimeProjectByName(ctx, tc.project.DisplayName())
			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
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
		configFunc                     func() (*rest.Config, error)
		nexusClientFunc                func(*rest.Config) (*nexus.Clientset, error)
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
			configFunc:                     func() (*rest.Config, error) { return &rest.Config{}, nil },
			nexusClientFunc:                func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
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
			configFunc:                     func() (*rest.Config, error) { return &rest.Config{}, nil },
			nexusClientFunc:                func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
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
			configFunc:                     func() (*rest.Config, error) { return &rest.Config{}, nil },
			nexusClientFunc:                func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
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
			name:            "Valid default template exists in project namespace (should see all 5, valid-existing-v1.2.3 is default)",
			configFunc:      func() (*rest.Config, error) { return &rest.Config{}, nil },
			nexusClientFunc: func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
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
			name:            "Invalid default template exists in project namespace (should see all 5, mock-template-1 is default)",
			configFunc:      func() (*rest.Config, error) { return &rest.Config{}, nil },
			nexusClientFunc: func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
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
			GetClusterConfigFunc = tc.configFunc
			GetNexusClientSetFunc = tc.nexusClientFunc
			GetK8sClientFunc = tc.k8sClientFunc
			GetTemplatesFunc = tc.templateFunc
			GetPodSecurityAdmissionConfigFunc = tc.podSecurityAdmissionConfigFunc
			defaultTemplate = tc.defaultTemplateName

			client, err := NewDatamodelClient()
			assert.NoError(suite.T(), err)
			assert.NotNil(suite.T(), client)
			assert.Equal(suite.T(), tc.expectedTemplateCount, len(client.templates))

			ctx := context.Background()
			err = client.setupProject(ctx, &nexus.RuntimeprojectRuntimeProject{
				RuntimeProject: &projectv1.RuntimeProject{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-project",
						UID:  "test-project-id",
						Labels: map[string]string{
							"nexus/display_name": "test-project-display-name",
						},
					},
					Spec: projectv1.RuntimeProjectSpec{
						Deleted: false,
					},
				},
			})
			assert.NoError(suite.T(), err)

			// Check templates were created
			for _, tmpl := range mockTemplateNames {
				assert.True(suite.T(), client.k8s.HasTemplate(ctx, "test-project-id", tmpl+"-v1.0.0"))
			}

			// Verify that the default template is set correctly
			template, err := client.k8s.DefaultTemplate(ctx, "test-project-id")
			assert.NoError(suite.T(), err)
			assert.NotEmpty(suite.T(), template, "default template should not be empty")
			assert.Equal(suite.T(), tc.expectedDefault, template.GetName())
		})
	}
}
func (suite *TenancyDatamodelTestSuite) TestSetupProject() {
	testProject := &nexus.RuntimeprojectRuntimeProject{
		RuntimeProject: &projectv1.RuntimeProject{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-project",
				UID:  "test-project-id",
				Labels: map[string]string{
					"nexus/display_name": "test-project-display-name",
				},
			},
			Spec: projectv1.RuntimeProjectSpec{
				Deleted: false,
			},
		},
	}

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
		nexus:           suite.t.nexus,
		k8s:             fakeK8s,
		templates:       templates,
		defaultTemplate: "baseline-k3s-v1.2.3",
		psaData:         psaData,
	}

	ctx := context.Background()
	err := t.setupProject(ctx, testProject)
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
