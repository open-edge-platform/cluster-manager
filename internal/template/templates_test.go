// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package template

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
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
