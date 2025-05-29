// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	provider "github.com/open-edge-platform/cluster-manager/v2/internal/providers"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

var namesemverRegex = regexp.MustCompile(`^(?P<name>.*)-v(?P<semver>\d+\.\d+\.\d+.*)$`)

// fromTemplateInfoToClusterTemplate translates a TemplateInfo object to a ClusterTemplate object
func FromTemplateInfoToClusterTemplate(templateInfo api.TemplateInfo) (*v1alpha1.ClusterTemplate, error) {
	slog.Debug("fromTemplateInfoToClusterTemplate", "templateInfo", templateInfo)
	var err error

	clusterTemplate := v1alpha1.ClusterTemplate{
		TypeMeta: v1.TypeMeta{
			APIVersion: core.TemplateResourceSchema.GroupVersion().String(),
			Kind:       "ClusterTemplate",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: templateInfo.Name + "-" + templateInfo.Version,
		},
		Spec: v1alpha1.ClusterTemplateSpec{
			KubernetesVersion: templateInfo.KubernetesVersion,
		},
	}

	var clusterConfigurationBytes []byte
	if templateInfo.Clusterconfiguration != nil {
		clusterConfigurationBytes, err = json.Marshal(templateInfo.Clusterconfiguration)
		if err != nil {
			return nil, err
		}
		clusterTemplate.Spec.ClusterConfiguration = string(clusterConfigurationBytes)
	}

	if templateInfo.Controlplaneprovidertype != nil {
		clusterTemplate.Spec.ControlPlaneProviderType = string(*templateInfo.Controlplaneprovidertype)
	}

	if templateInfo.Infraprovidertype != nil {
		clusterTemplate.Spec.InfraProviderType = string(*templateInfo.Infraprovidertype)
	}

	if templateInfo.ClusterNetwork != nil {
		clusterTemplate.Spec.ClusterNetwork = v1alpha1.ClusterNetwork{}
		if templateInfo.ClusterNetwork.Pods != nil {
			clusterTemplate.Spec.ClusterNetwork.Pods = &v1alpha1.NetworkRanges{
				CIDRBlocks: templateInfo.ClusterNetwork.Pods.CidrBlocks,
			}
		}

		if templateInfo.ClusterNetwork.Services != nil {
			clusterTemplate.Spec.ClusterNetwork.Services = &v1alpha1.NetworkRanges{
				CIDRBlocks: templateInfo.ClusterNetwork.Services.CidrBlocks,
			}
		}
	}

	if templateInfo.Description != nil {
		clusterTemplate.ObjectMeta.Annotations = map[string]string{
			"description": *templateInfo.Description,
		}
	}

	if templateInfo.ClusterLabels != nil {
		clusterTemplate.Spec.ClusterLabels = *templateInfo.ClusterLabels
	}

	return &clusterTemplate, nil
}

func FromClusterTemplateToTemplateInfo(clusterTemplate v1alpha1.ClusterTemplate) (*api.TemplateInfo, error) {
	slog.Debug("fromClusterTemplateToTemplateInfo", "clusterTemplate", clusterTemplate)
	name, version := fromJoinedNameToNameVersion(clusterTemplate.Name)
	if name == "" || version == "" {
		slog.Error("invalid clusterTemplate name format", "name", clusterTemplate.Name)
		return nil, errors.New("invalid clusterTemplate name format")
	}

	templateInfo := api.TemplateInfo{
		Name:              name,
		Version:           version,
		KubernetesVersion: clusterTemplate.Spec.KubernetesVersion,
	}

	if clusterTemplate.Spec.ClusterConfiguration != "" {
		var clusterConfiguration map[string]interface{}
		err := json.Unmarshal([]byte(clusterTemplate.Spec.ClusterConfiguration), &clusterConfiguration)
		if err != nil {
			slog.Error("failed to unmarshal cluster configuration", "error", err)
			return nil, err
		}
		templateInfo.Clusterconfiguration = &clusterConfiguration
	}

	if clusterTemplate.Spec.ControlPlaneProviderType != "" {
		templateInfo.Controlplaneprovidertype = (*api.TemplateInfoControlplaneprovidertype)(&clusterTemplate.Spec.ControlPlaneProviderType)
	}

	if clusterTemplate.Spec.InfraProviderType != "" {
		templateInfo.Infraprovidertype = (*api.TemplateInfoInfraprovidertype)(&clusterTemplate.Spec.InfraProviderType)
	}

	if description, ok := clusterTemplate.ObjectMeta.Annotations["description"]; ok {
		templateInfo.Description = &description
	}

	clusterNetwork := api.ClusterNetwork{}
	if clusterTemplate.Spec.ClusterNetwork.Pods != nil {
		clusterNetwork.Pods = &api.NetworkRanges{CidrBlocks: clusterTemplate.Spec.ClusterNetwork.Pods.CIDRBlocks}
		templateInfo.ClusterNetwork = &clusterNetwork
	}

	if clusterTemplate.Spec.ClusterNetwork.Services != nil {
		clusterNetwork.Services = &api.NetworkRanges{CidrBlocks: clusterTemplate.Spec.ClusterNetwork.Services.CIDRBlocks}
		templateInfo.ClusterNetwork = &clusterNetwork
	}

	if clusterTemplate.Spec.ClusterLabels != nil {
		templateInfo.ClusterLabels = &clusterTemplate.Spec.ClusterLabels
	}

	return &templateInfo, nil
}

func FromClusterTemplateToDefaultTemplateInfo(clusterTemplate v1alpha1.ClusterTemplate) (*api.DefaultTemplateInfo, error) {
	slog.Debug("fromClusterTemplateToDefaultTemplateInfo", "clusterTemplate", clusterTemplate)
	name, version := fromJoinedNameToNameVersion(clusterTemplate.Name)
	if name == "" || version == "" {
		message := fmt.Sprintf("invalid clusterTemplate name format: %s", clusterTemplate.Name)
		slog.Error(message)
		return nil, errors.New(message)
	}
	return &api.DefaultTemplateInfo{Name: &name, Version: version}, nil
}

func fromJoinedNameToNameVersion(joinedName string) (name, version string) {
	slog.Debug("fromJoinedNameToNameVersion", "joinedName", joinedName)

	matches := namesemverRegex.FindStringSubmatch(joinedName)
	if len(matches) != 3 {
		slog.Error("invalid clusterTemplate name format", "joinedName", joinedName)
		return "", ""
	}

	nameIndex := namesemverRegex.SubexpIndex("name")
	semverIndex := namesemverRegex.SubexpIndex("semver")

	return matches[nameIndex], "v" + matches[semverIndex]
}

func ReadDefaultTemplates(disableK3sTemplates bool) ([]*v1alpha1.ClusterTemplate, error) {
	templatesPath := os.Getenv("DEFAULT_TEMPLATES_DIR")
	if templatesPath == "" {
		templatesPath = "/default-templates"
	}

	var templates []*v1alpha1.ClusterTemplate
	entries, err := os.ReadDir(templatesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read files from template directory: %w", err)
	}
	for _, entry := range entries {
		template, err := readClusterTemplateData(templatesPath + "/" + entry.Name())
		if err != nil {
			slog.Debug("couldn't read default cluster template", "template-file", entry.Name())
			continue
		}
		if disableK3sTemplates && template != nil && strings.Contains(template.Name, provider.DefaultProvider) {
			slog.Debug("skipping k3s template", "template-name", template.Name)
			continue
		}
		slog.Debug("read default cluster template", "template-name", template.Name)
		templates = append(templates, template)
	}

	return templates, nil
}

func readClusterTemplateData(filePath string) (*v1alpha1.ClusterTemplate, error) {
	var templateInfo api.TemplateInfo
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	err = json.Unmarshal(data, &templateInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template info: %w", err)
	}

	template, err := FromTemplateInfoToClusterTemplate(templateInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to construct cluster template from info: %w", err)
	}

	return template, nil
}
