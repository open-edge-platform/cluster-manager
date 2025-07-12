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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	activeWatcher "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectactivewatcher.edge-orchestrator.intel.com/v1"
	watcherv1 "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectwatcher.edge-orchestrator.intel.com/v1"
	projectv1 "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/runtimeproject.edge-orchestrator.intel.com/v1"
	nexus "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/nexus-client"
)

type TenancyDatamodelTestSuite struct {
	suite.Suite
	tdm *TenancyDatamodel
}

func TestTenancyDatamodelSuite(t *testing.T) {
	suite.Run(t, new(TenancyDatamodelTestSuite))
}

func (suite *TenancyDatamodelTestSuite) SetupTest() {
	client := nexus.NewFakeClient()
	require.NotNil(suite.T(), client)

	k8s := k8s.NewClientFake()

	templates := []*v1alpha1.ClusterTemplate{}

	suite.tdm = &TenancyDatamodel{client: client, k8s: k8s, templates: templates}
}

func (suite *TenancyDatamodelTestSuite) TestStart() {
	err := suite.tdm.Start()
	assert.NoError(suite.T(), err)
}

func (suite *TenancyDatamodelTestSuite) TestStop() {
	suite.tdm.Stop()
}

func (suite *TenancyDatamodelTestSuite) TestNewDatamodelClient() {
	cases := []struct {
		name                           string
		configFunc                     func() (*rest.Config, error)
		clientSetFunc                  func(*rest.Config) (*nexus.Clientset, error)
		k8sClientFunc                  func(*config.Config) (*k8s.Client, error)
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
			k8sClientFunc: func(*config.Config) (*k8s.Client, error) {
				return k8s.NewClientFake(), nil
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
			expectedErr:                    errors.New("failed to get kubeconfig: config error"),
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
			err := suite.tdm.addProjectWatcher()
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
				_, err := suite.tdm.client.TenancyMultiTenancy().Config().AddProjectWatchers(context.Background(),
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
			err := suite.tdm.deleteProjectWatcher()
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

	suite.tdm.addProjectWatcher()

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			ctx := context.Background()
			_, err := suite.tdm.client.Runtimeproject().CreateRuntimeProjectByName(ctx, tc.project)
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

	suite.tdm.addProjectWatcher()

	for _, tc := range cases {
		suite.Run(tc.name, func() {
			ctx := context.Background()
			project, _ := suite.tdm.client.Runtimeproject().CreateRuntimeProjectByName(ctx, tc.project)
			project.AddActiveWatchers(ctx, &activeWatcher.ProjectActiveWatcher{
				ObjectMeta: metav1.ObjectMeta{Name: appName},
				Spec: activeWatcher.ProjectActiveWatcherSpec{
					StatusIndicator: activeWatcher.StatusIndicationInProgress,
					Message:         fmt.Sprintf("%s subscribed to project %s", appName, project.DisplayName()),
					TimeStamp:       safeUnixTime(),
				},
			})
			err := suite.tdm.client.Runtimeproject().DeleteRuntimeProjectByName(ctx, tc.project.DisplayName())
			if tc.expectedErr != nil {
				assert.EqualError(suite.T(), err, tc.expectedErr.Error())
			} else {
				assert.NoError(suite.T(), err)
			}
		})
	}
}

func (suite *TenancyDatamodelTestSuite) TestNewDatamodelClientK3Templates() {
	tmpDir := suite.T().TempDir()
	mockTemplate1 := []byte(`{"Name":"mock-k3s-template","Version":"v1.0.0","KubernetesVersion":"1.25"}`)
	mockTemplate2 := []byte(`{"Name":"mock-kubeadm-template","Version":"v1.1.0","KubernetesVersion":"1.26"}`)
	mockTemplate3 := []byte(`{"Name":"baseline-rke2","Version":"v2.0.0","KubernetesVersion":"1.27"}`)
	mockBaselineTemplate := []byte(`{"Name":"baseline","Version":"v1.2.3","KubernetesVersion":"1.28"}`)
	mockBaselineK3sTemplate := []byte(`{"Name":"baseline-k3s","Version":"v1.2.3","KubernetesVersion":"1.28"}`)

	require.NoError(suite.T(), os.WriteFile(filepath.Join(tmpDir, "mock-template1.json"), mockTemplate1, 0644))
	require.NoError(suite.T(), os.WriteFile(filepath.Join(tmpDir, "mock-template2.json"), mockTemplate2, 0644))
	require.NoError(suite.T(), os.WriteFile(filepath.Join(tmpDir, "mock-template3.json"), mockTemplate3, 0644))
	require.NoError(suite.T(), os.WriteFile(filepath.Join(tmpDir, "mock-baseline.json"), mockBaselineTemplate, 0644))
	require.NoError(suite.T(), os.WriteFile(filepath.Join(tmpDir, "mock-baseline-k3s.json"), mockBaselineK3sTemplate, 0644))

	oldEnv := os.Getenv("DEFAULT_TEMPLATES_DIR")
	require.NoError(suite.T(), os.Setenv("DEFAULT_TEMPLATES_DIR", tmpDir))
	suite.T().Cleanup(func() { os.Setenv("DEFAULT_TEMPLATES_DIR", oldEnv) })

	cases := []struct {
		name                           string
		disableK3s                     bool
		configFunc                     func() (*rest.Config, error)
		clientSetFunc                  func(*rest.Config) (*nexus.Clientset, error)
		k8sClientFunc                  func(*config.Config) (*k8s.Client, error)
		templateFunc                   func() ([]*v1alpha1.ClusterTemplate, error)
		podSecurityAdmissionConfigFunc func() (map[string][]byte, error)
		expectedErr                    error
		expectedClient                 bool
		expectedTemplateCount          int
		expectedDefault                string
	}{
		{
			name:                           "K3s templates enabled (should see all 5, baseline-k3s is default)",
			disableK3s:                     false,
			configFunc:                     func() (*rest.Config, error) { return &rest.Config{}, nil },
			clientSetFunc:                  func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
			k8sClientFunc:                  func(*config.Config) (*k8s.Client, error) { return &k8s.Client{}, nil },
			templateFunc:                   nil, // call for the real function
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			expectedErr:                    nil,
			expectedClient:                 true,
			expectedTemplateCount:          5,
			expectedDefault:                "baseline-k3s-v1.2.3",
		},
		{
			name:                           "K3s templates disabled (should see only 4, baseline is default)",
			disableK3s:                     true,
			configFunc:                     func() (*rest.Config, error) { return &rest.Config{}, nil },
			clientSetFunc:                  func(*rest.Config) (*nexus.Clientset, error) { return &nexus.Clientset{}, nil },
			k8sClientFunc:                  func(*config.Config) (*k8s.Client, error) { return &k8s.Client{}, nil },
			templateFunc:                   nil, // call for the real function
			podSecurityAdmissionConfigFunc: func() (map[string][]byte, error) { return map[string][]byte{}, nil },
			expectedErr:                    nil,
			expectedClient:                 true,
			expectedTemplateCount:          3,
			expectedDefault:                "baseline-v1.2.3",
		},
	}
	for _, tc := range cases {
		suite.Run(tc.name, func() {
			disableK3sTemplates = tc.disableK3s
			GetTemplatesFunc = func() ([]*v1alpha1.ClusterTemplate, error) {
				return template.ReadDefaultTemplates(disableK3sTemplates)
			}
			GetClusterConfigFunc = tc.configFunc
			GetNexusClientSetFunc = tc.clientSetFunc
			GetK8sClientFunc = tc.k8sClientFunc
			GetPodSecurityAdmissionConfigFunc = tc.podSecurityAdmissionConfigFunc
			client, err := NewDatamodelClient()
			assert.NoError(suite.T(), err)
			assert.NotNil(suite.T(), client)
			assert.Equal(suite.T(), tc.expectedTemplateCount, len(client.templates))

			defaultTemplateName := selectDefaultTemplateName(client.templates, disableK3sTemplates)
			assert.Equal(suite.T(), tc.expectedDefault, defaultTemplateName)
		})
	}
}
func (suite *TenancyDatamodelTestSuite) TestSetupProject() {
	disableK3sTemplates = false
	testProject := &nexus.RuntimeprojectRuntimeProject{
		RuntimeProject: &projectv1.RuntimeProject{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-project",
				UID:  "test-uid",
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

	fakeK8s := k8s.NewClientFake()

	tdm := &TenancyDatamodel{
		client:    suite.tdm.client,
		k8s:       fakeK8s,
		templates: templates,
		psaData:   psaData,
	}

	ctx := context.Background()
	err := tdm.setupProject(ctx, testProject)
	assert.NoError(suite.T(), err)

	// Check that namespace was created
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err = fakeK8s.Dyn.Resource(namespaceRes).Get(ctx, "test-uid", metav1.GetOptions{})
	assert.NoError(suite.T(), err, "namespace should be created")

	// Check that PSA secret was created
	secretRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	secret, err := fakeK8s.Dyn.Resource(secretRes).Namespace("test-uid").Get(ctx, "pod-security-admission-config", metav1.GetOptions{})
	assert.NoError(suite.T(), err, "secret should be created")

	// Check that secret has three items
	assert.Len(suite.T(), secret.Object["data"].(map[string]interface{}), 3)

	// Check that templates were created
	templateRes := schema.GroupVersionResource{Group: "edge-orchestrator.intel.com", Version: "v1alpha1", Resource: "clustertemplates"}
	for _, tmpl := range templates {
		_, err = fakeK8s.Dyn.Resource(templateRes).Namespace("test-uid").Get(ctx, tmpl.GetName(), metav1.GetOptions{})
		assert.NoError(suite.T(), err, "template should be created")
	}
}
