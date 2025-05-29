// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

func TestFromTemplateInfoToClusterTemplate(t *testing.T) {
	description := "Test Description"
	controlPlaneProviderType := "providerType1"
	infraProviderType := "providerType2"
	clusterLabels := map[string]string{"label1": "value1"}
	templateInfo := api.TemplateInfo{
		Name:                     "test-template",
		Version:                  "v1",
		Description:              &description,
		Controlplaneprovidertype: (*api.TemplateInfoControlplaneprovidertype)(&controlPlaneProviderType),
		Infraprovidertype:        (*api.TemplateInfoInfraprovidertype)(&infraProviderType),
		KubernetesVersion:        "1.21",
		ClusterLabels:            &clusterLabels,
	}

	clusterTemplate, err := FromTemplateInfoToClusterTemplate(templateInfo)
	require.NoError(t, err)
	require.NotNil(t, clusterTemplate)
	require.Equal(t, "test-template-v1", clusterTemplate.Name)
	require.Equal(t, "Test Description", clusterTemplate.Annotations["description"])
	require.Equal(t, "providerType1", clusterTemplate.Spec.ControlPlaneProviderType)
	require.Equal(t, "providerType2", clusterTemplate.Spec.InfraProviderType)
	require.Equal(t, "1.21", clusterTemplate.Spec.KubernetesVersion)
	require.Equal(t, clusterLabels, clusterTemplate.Spec.ClusterLabels)
}

func TestFromTemplateInfoToClusterTemplateWithClusterNetwork(t *testing.T) {
	templateInfo := api.TemplateInfo{}
	t.Run("ClusterNetwork not defined", func(t *testing.T) {
		expectedClusterNetwork := v1alpha1.ClusterNetwork{}
		clusterTemplate, err := FromTemplateInfoToClusterTemplate(templateInfo)
		require.NoError(t, err)
		require.NotNil(t, clusterTemplate)
		require.Equal(t, expectedClusterNetwork, clusterTemplate.Spec.ClusterNetwork)
	})

	t.Run("WithServices", func(t *testing.T) {
		templateInfo.ClusterNetwork = &api.ClusterNetwork{
			Services: &api.NetworkRanges{CidrBlocks: []string{"10.1.0.0/16"}},
		}

		expectedclusterNetwork := &v1alpha1.ClusterNetwork{
			Services: &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
		}

		clusterTemplate, err := FromTemplateInfoToClusterTemplate(templateInfo)
		require.NoError(t, err)
		require.NotNil(t, clusterTemplate)
		require.Equal(t, expectedclusterNetwork.Services.CIDRBlocks, clusterTemplate.Spec.ClusterNetwork.Services.CIDRBlocks)
		require.Nil(t, clusterTemplate.Spec.ClusterNetwork.Pods)
	})
	t.Run("WithClusterNetworkAndPods", func(t *testing.T) {
		templateInfo.ClusterNetwork = &api.ClusterNetwork{
			Pods: &api.NetworkRanges{CidrBlocks: []string{"10.0.0.0/16"}},
		}
		expectedclusterNetwork := &v1alpha1.ClusterNetwork{
			Pods: &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
		}

		fmt.Println(templateInfo)
		clusterTemplate, err := FromTemplateInfoToClusterTemplate(templateInfo)
		require.NoError(t, err)
		require.NotNil(t, clusterTemplate)
		require.Equal(t, expectedclusterNetwork.Pods.CIDRBlocks, clusterTemplate.Spec.ClusterNetwork.Pods.CIDRBlocks)
		require.Nil(t, clusterTemplate.Spec.ClusterNetwork.Services)
	})

	t.Run("WithClusterNetworkAndServicesAndPods", func(t *testing.T) {
		templateInfo.ClusterNetwork = &api.ClusterNetwork{
			Services: &api.NetworkRanges{CidrBlocks: []string{"10.1.0.0/16"}},
			Pods:     &api.NetworkRanges{CidrBlocks: []string{"10.0.0.0/16"}},
		}

		expectedclusterNetwork := &v1alpha1.ClusterNetwork{
			Services: &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
			Pods:     &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
		}

		clusterTemplate, err := FromTemplateInfoToClusterTemplate(templateInfo)
		require.NoError(t, err)
		require.NotNil(t, clusterTemplate)
		require.Equal(t, expectedclusterNetwork.Pods.CIDRBlocks, clusterTemplate.Spec.ClusterNetwork.Pods.CIDRBlocks)
		require.Equal(t, expectedclusterNetwork.Services.CIDRBlocks, clusterTemplate.Spec.ClusterNetwork.Services.CIDRBlocks)
	})
}

func TestFromTemplateInfoToClusterTemplateWithInvalidJSON(t *testing.T) {
	templateInfo := api.TemplateInfo{
		Clusterconfiguration: &map[string]interface{}{
			"invalid": make(chan int),
		},
	}

	clusterTemplate, err := FromTemplateInfoToClusterTemplate(templateInfo)
	require.Error(t, err)
	require.Nil(t, clusterTemplate)
}

func TestFromClusterTemplateToTemplateInfo(t *testing.T) {
	description := "Test Description"
	controlPlaneProviderType := "providerType1"
	infraProviderType := "providerType2"
	clusterLabels := map[string]string{"label1": "value1"}
	clusterTemplate := v1alpha1.ClusterTemplate{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-template-v1.0.0",
			Annotations: map[string]string{
				"description": description,
			},
		},
		Spec: v1alpha1.ClusterTemplateSpec{
			KubernetesVersion:        "1.21",
			ControlPlaneProviderType: controlPlaneProviderType,
			InfraProviderType:        infraProviderType,
			ClusterLabels:            clusterLabels,
		},
	}

	templateInfo, err := FromClusterTemplateToTemplateInfo(clusterTemplate)
	require.NoError(t, err)
	require.NotNil(t, templateInfo)
	require.Equal(t, "test-template", templateInfo.Name)
	require.Equal(t, "v1.0.0", templateInfo.Version)
	require.Equal(t, "Test Description", *templateInfo.Description)
	require.Equal(t, "providerType1", string(*templateInfo.Controlplaneprovidertype))
	require.Equal(t, "providerType2", string(*templateInfo.Infraprovidertype))
	require.Equal(t, "1.21", templateInfo.KubernetesVersion)
	require.Nil(t, templateInfo.ClusterNetwork)
	require.Equal(t, clusterLabels, *templateInfo.ClusterLabels)
}

func TestFromClusterTemplateToTemplateInfoWithClusterNetwork(t *testing.T) {
	clusterTemplate := v1alpha1.ClusterTemplate{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-template-v1.0.0",
		},
	}

	t.Run("WithClusterNetworkAndServices", func(t *testing.T) {
		clusterTemplate.Spec.ClusterNetwork = v1alpha1.ClusterNetwork{
			Services: &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
		}

		expectedClusterNetwork := api.ClusterNetwork{
			Services: &api.NetworkRanges{CidrBlocks: []string{"10.1.0.0/16"}},
		}

		templateInfo, err := FromClusterTemplateToTemplateInfo(clusterTemplate)
		require.NoError(t, err)
		require.NotNil(t, templateInfo)
		require.Equal(t, expectedClusterNetwork.Services.CidrBlocks, templateInfo.ClusterNetwork.Services.CidrBlocks)
		require.Nil(t, templateInfo.ClusterNetwork.Pods)
	})

	t.Run("WithClusterNetworkAndPods", func(t *testing.T) {
		clusterTemplate.Spec.ClusterNetwork = v1alpha1.ClusterNetwork{
			Pods: &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
		}

		expectedClusterNetwork := api.ClusterNetwork{
			Pods: &api.NetworkRanges{CidrBlocks: []string{"10.0.0.0/16"}},
		}

		templateInfo, err := FromClusterTemplateToTemplateInfo(clusterTemplate)
		require.NoError(t, err)
		require.NotNil(t, templateInfo)
		require.Equal(t, expectedClusterNetwork.Pods.CidrBlocks, templateInfo.ClusterNetwork.Pods.CidrBlocks)
		require.Nil(t, templateInfo.ClusterNetwork.Services)
	})

	t.Run("WithClusterNetworkAndServicesAndPods", func(t *testing.T) {
		clusterTemplate.Spec.ClusterNetwork = v1alpha1.ClusterNetwork{
			Services: &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
			Pods:     &v1alpha1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
		}

		expectedClusterNetwork := api.ClusterNetwork{
			Services: &api.NetworkRanges{CidrBlocks: []string{"10.1.0.0/16"}},
			Pods:     &api.NetworkRanges{CidrBlocks: []string{"10.0.0.0/16"}},
		}

		templateInfo, err := FromClusterTemplateToTemplateInfo(clusterTemplate)
		require.NoError(t, err)
		require.NotNil(t, templateInfo)
		require.Equal(t, expectedClusterNetwork.Pods.CidrBlocks, templateInfo.ClusterNetwork.Pods.CidrBlocks)
		require.Equal(t, expectedClusterNetwork.Services.CidrBlocks, templateInfo.ClusterNetwork.Services.CidrBlocks)
	})
}

func TestFromClusterTemplateToTemplateInfoWithInvalidName(t *testing.T) {
	clusterTemplate := v1alpha1.ClusterTemplate{
		ObjectMeta: v1.ObjectMeta{
			Name: "invalid-name-format",
		},
	}

	templateInfo, err := FromClusterTemplateToTemplateInfo(clusterTemplate)
	require.Error(t, err)
	require.Empty(t, templateInfo)
}

func TestFromClusterTemplateToDefaultTemplateInfo(t *testing.T) {
	t.Run("ValidName", func(t *testing.T) {
		clusterTemplate := v1alpha1.ClusterTemplate{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-template-v1.0.0",
			},
		}

		defaultTemplateInfo, err := FromClusterTemplateToDefaultTemplateInfo(clusterTemplate)
		require.NoError(t, err)
		require.NotNil(t, defaultTemplateInfo)
		require.Equal(t, "test-template", *defaultTemplateInfo.Name)
		require.Equal(t, "v1.0.0", defaultTemplateInfo.Version)
	})

	t.Run("TrickyName", func(t *testing.T) {
		clusterTemplate := v1alpha1.ClusterTemplate{
			ObjectMeta: v1.ObjectMeta{
				Name: "tricky-v1.0-format-v1.0.0",
			},
		}

		defaultTemplateInfo, err := FromClusterTemplateToDefaultTemplateInfo(clusterTemplate)
		require.NoError(t, err)
		require.Equal(t, "tricky-v1.0-format", *defaultTemplateInfo.Name)
		require.Equal(t, "v1.0.0", defaultTemplateInfo.Version)
	})

	t.Run("Dev Version Name", func(t *testing.T) {
		clusterTemplate := v1alpha1.ClusterTemplate{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-template-v1.0.0-dev",
			},
		}

		defaultTemplateInfo, err := FromClusterTemplateToDefaultTemplateInfo(clusterTemplate)
		require.NoError(t, err)
		require.Equal(t, "test-template", *defaultTemplateInfo.Name)
		require.Equal(t, "v1.0.0-dev", defaultTemplateInfo.Version)
	})
}

func TestReadDefaultTemplates(t *testing.T) {
    tmpDir := t.TempDir()
    t.Cleanup(func() { os.RemoveAll(tmpDir) })
    // helper to write a template file
    writeTemplateFile := func(name string, tmpl v1alpha1.ClusterTemplate) {
        info := map[string]interface{}{
            "Name":              tmpl.Name,
            "Version":           "v1.0.0",
            "KubernetesVersion": tmpl.Spec.KubernetesVersion,
        }
        data, err := json.Marshal(info)
        require.NoError(t, err)
        require.NoError(t, os.WriteFile(filepath.Join(tmpDir, name), data, 0644))
    }
    writeTemplateFile("template1.json", v1alpha1.ClusterTemplate{
        ObjectMeta: v1.ObjectMeta{Name: "test-template"},
        Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.25"},
    })
    writeTemplateFile("k3s-template.json", v1alpha1.ClusterTemplate{
        ObjectMeta: v1.ObjectMeta{Name: "k3s-template"},
        Spec:       v1alpha1.ClusterTemplateSpec{KubernetesVersion: "1.26"},
    })
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "invalid.json"), []byte("{invalid json"), 0644))
    oldEnv := os.Getenv("DEFAULT_TEMPLATES_DIR")
    require.NoError(t, os.Setenv("DEFAULT_TEMPLATES_DIR", tmpDir))
    t.Cleanup(func() { os.Setenv("DEFAULT_TEMPLATES_DIR", oldEnv) })
    tests := []struct {
        name                string
        disableK3sTemplates bool
        setupEnv            func()
        wantNames           []string
        wantErr             bool
    }{
        {
            name:                "reads all valid templates",
            disableK3sTemplates: false,
            setupEnv:            func() { require.NoError(t, os.Setenv("DEFAULT_TEMPLATES_DIR", tmpDir)) },
            wantNames:           []string{"test-template-v1.0.0", "k3s-template-v1.0.0"},
            wantErr:             false,
        },
        {
            name:                "skips k3s templates if disabled",
            disableK3sTemplates: true,
            setupEnv:            func() { require.NoError(t, os.Setenv("DEFAULT_TEMPLATES_DIR", tmpDir)) },
            wantNames:           []string{"test-template-v1.0.0"},
            wantErr:             false,
        },
        {
            name:                "returns error if directory does not exist",
            disableK3sTemplates: false,
            setupEnv:            func() { require.NoError(t, os.Setenv("DEFAULT_TEMPLATES_DIR", "/non-existent-dir")) },
            wantNames:           nil,
            wantErr:             true,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tt.setupEnv()
            templates, err := ReadDefaultTemplates(tt.disableK3sTemplates)
            if tt.wantErr {
                require.Error(t, err)
                require.Nil(t, templates)
                return
            }
            require.NoError(t, err)
            var names []string
            for _, tmpl := range templates {
                names = append(names, tmpl.Name)
            }
            require.ElementsMatch(t, tt.wantNames, names)
        })
    }
}
