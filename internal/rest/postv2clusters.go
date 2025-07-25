// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	controlplaneprovider "github.com/open-edge-platform/cluster-manager/v2/internal/providers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	ct "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (POST /v2/clusters)
func (s *Server) PostV2Clusters(ctx context.Context, request api.PostV2ClustersRequestObject) (api.PostV2ClustersResponseObject, error) {
	namespace := request.Params.Activeprojectid.String()

	// validate nodes (only single node clusters are supported)
	nodes := request.Body.Nodes
	if nodes == nil {
		msg := "nodes are required"
		slog.Error(msg)
		return api.PostV2Clusters400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &msg}}, nil
	}

	if len(nodes) != 1 {
		msg := fmt.Sprintf("only single node clusters are supported, got %d nodes", len(nodes))
		slog.Warn(msg)
		return api.PostV2Clusters400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &msg}}, nil
	}

	var userLabels map[string]string
	if request.Body.Labels == nil {
		userLabels = map[string]string{}
	} else {
		userLabels = *request.Body.Labels
	}

	// cluster name is optional, if not provided we generate one
	var clusterName string
	if request.Body.Name == nil || *request.Body.Name == "" {
		clusterName = fmt.Sprintf("cluster-%v", time.Now().Unix())
		slog.Info("cluster name not provided, generating one", "name", clusterName)
	} else {
		clusterName = *request.Body.Name
	}

	// create k8s client
	cli, err := k8s.New(k8s.WithDynamicClient(s.k8sclient))
	if err != nil {
		msg := fmt.Sprintf("failed to create k8s client: %v", err)
		slog.Error(msg)
		return api.PostV2Clusters500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &msg}}, nil
	}

	// fetch cluster template
	template, err := fetchTemplate(ctx, cli, namespace, request.Body.Template)
	if err != nil {
		msg := fmt.Sprintf("failed to create cluster: %v", err)
		slog.Error(msg)
		return api.PostV2Clusters500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &msg}}, nil
	}

	// fetch host from inventory to check for trusted compute
	trustedCompute, err := s.inventory.GetHostTrustedCompute(ctx, namespace, nodes[0].Id)
	if err != nil {
		slog.Warn("failed to get host trusted compute", "error", err)
	}

	// merge user labels with template and system labels
	clusterLabels := labels.Merge(userLabels, template.Spec.ClusterLabels, map[string]string{
		fmt.Sprintf("%s/clustername", labels.PlatformPrefix): clusterName,
		fmt.Sprintf("%s/project-id", labels.PlatformPrefix):  namespace,
		labels.PrometheusMetricsUrlLabelKey:                  fmt.Sprintf("%s.%s", labels.PrometheusMetricsSubdomain, s.config.ClusterDomain),
		labels.TrustedComputeLabelKey:                        strconv.FormatBool(trustedCompute),
	})

	// validate cluster labels against k8s label format
	if !labels.Valid(clusterLabels) {
		msg := "invalid cluster labels"
		slog.Error(msg, "labels", clusterLabels)
		return api.PostV2Clusters400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &msg}}, nil
	}

	// create cluster
	slog.Debug("creating cluster", "namespace", namespace)
	createdClusterName, err := s.createCluster(ctx, cli, namespace, clusterName, template, nodes, clusterLabels)
	if err != nil {
		slog.Error("failed to create cluster", "namespace", namespace, "name", clusterName, "error", err)
		return api.PostV2Clusters500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(fmt.Sprintf("failed to create cluster: %v", err)),
			},
		}, nil
	}

	// create machine binding for Intel infra provider
	if api.TemplateInfoInfraprovidertype(template.Spec.InfraProviderType) == api.Intel {
		err := createBindings(ctx, cli, namespace, clusterName, template.Name, nodes)
		if err != nil {
			msg := fmt.Sprintf("failed to create machine bindings: %v", err)
			slog.Error(msg)
			return api.PostV2Clusters500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &msg}}, nil
		}
	}

	slog.Info("Cluster created", "namespace", namespace, "name", createdClusterName)
	return api.PostV2Clusters201JSONResponse(fmt.Sprintf("successfully created cluster %s", createdClusterName)), nil
}

func fetchTemplate(ctx context.Context, cli *k8s.Client, namespace string, templateName *string) (ct.ClusterTemplate, error) {
	// template name is optional, if not provided we use default
	var template ct.ClusterTemplate
	var err error
	if templateName == nil || *templateName == "" {
		slog.Info("template name not provided, using default template")
		if template, err = cli.DefaultTemplate(ctx, namespace); err != nil {
			return ct.ClusterTemplate{}, err
		}
	} else {
		if template, err = cli.Template(ctx, namespace, *templateName); err != nil {
			return ct.ClusterTemplate{}, err
		}
	}

	if !template.Status.Ready || template.Status.ClusterClassRef == nil {
		return ct.ClusterTemplate{}, fmt.Errorf("template %s is not ready", template.Name)
	}
	return template, nil
}

func (s *Server) createCluster(ctx context.Context, cli *k8s.Client, namespace, clusterName string, template ct.ClusterTemplate, nodes []api.NodeSpec, labels map[string]string) (string, error) {
	slog.Debug("creating cluster", "namespace", namespace, "name", clusterName, "nodes", nodes, "labels", labels)

	// Assumes single node cluster for now, so we can use the first node's ID for air-gap installation check
	// TODO: This will need to change when we support multi-node clusters
	enableReadOnly, err := s.enableReadOnlyInstall(ctx, cli, namespace, clusterName, nodes[0].Id, template)
	if err != nil {
		return "", err
	}

	var variables []capi.ClusterVariable
	if enableReadOnly {
		variables = append(variables, capi.ClusterVariable{
			Name: controlplaneprovider.ReadOnly,
			Value: apiextensionsv1.JSON{
				Raw: []byte(strconv.FormatBool(enableReadOnly)),
			},
		})
	}

	// create cluster
	replicas := int32(len(nodes))
	cluster := capi.Cluster{
		TypeMeta: v1.TypeMeta{
			APIVersion: core.ClusterResourceSchema.GroupVersion().String(),
			Kind:       core.ClusterResourceSchema.Resource,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				core.TemplateLabelKey: template.Name,
			},
		},
		Spec: capi.ClusterSpec{
			ClusterNetwork: convertClusterNetwork(&template.Spec.ClusterNetwork),
			Topology: &capi.Topology{
				Class:   template.Status.ClusterClassRef.Name,
				Version: template.Spec.KubernetesVersion,
				ControlPlane: capi.ControlPlaneTopology{
					Replicas: &replicas,
				},
				Variables: variables,
			},
		},
	}

	newClusterName, err := cli.CreateCluster(ctx, namespace, cluster)
	if err != nil {
		return "", err
	}
	return newClusterName, nil
}

func createBindings(ctx context.Context, cli *k8s.Client, namespace, clusterName, templateName string, nodes []api.NodeSpec) error {
	for _, nodes := range nodes {
		binding := intelv1alpha1.IntelMachineBinding{
			TypeMeta: v1.TypeMeta{
				APIVersion: core.BindingsResourceSchema.GroupVersion().String(),
				Kind:       "IntelMachineBinding",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", clusterName, nodes.Id),
				Namespace: namespace,
			},
			Spec: intelv1alpha1.IntelMachineBindingSpec{
				NodeGUID:                 nodes.Id,
				ClusterName:              clusterName,
				IntelMachineTemplateName: fmt.Sprintf("%s-controlplane", templateName),
			},
		}

		err := cli.CreateMachineBinding(ctx, namespace, binding)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) enableReadOnlyInstall(ctx context.Context, cli *k8s.Client, namespace, clusterName, nodeUuid string, template ct.ClusterTemplate) (bool, error) {
	// Fetch the cluster template
	clusterTemplate, err := cli.GetClusterTemplate(ctx, namespace, template.Name)
	if err != nil {
		return false, fmt.Errorf("failed to fetch cluster template %s: %w", template.Name, err)
	}

	if clusterTemplate == nil {
		return false, fmt.Errorf("cluster template %s not found", template.Name)
	}

	// Validate the control plane provider type
	if clusterTemplate.Spec.ControlPlaneProviderType == "" {
		return false, fmt.Errorf("control plane provider type is empty for template %s", template.Name)
	}

	// Check if air-gap installation is required
	if clusterTemplate.Spec.ControlPlaneProviderType == "k3s" {
		if s.config.DisableInventory {
			// This is specifically used in integration tests. However, when the inventory is enabled, the decision to
			// airgap mode or not is made based on the OS type of the host, as determined by the inventory service.
			slog.Debug("enable air gap by default for k3s, when inventory is disabled", "namespace", namespace, "name", clusterName, "node", nodeUuid)
			return true, nil
		}
		enableReadOnly, err := s.inventory.IsImmutable(ctx, namespace, nodeUuid)
		if err != nil {
			return false, fmt.Errorf("failed to determine read-only install for cluster %s, node: %s: %w", clusterName, nodeUuid, err)
		}
		slog.Debug("enable read-only install", "namespace", namespace, "name", clusterName, "node", nodeUuid, "immutable", enableReadOnly, "controlPlaneProviderType", clusterTemplate.Spec.ControlPlaneProviderType)
		return enableReadOnly, nil
	}

	// Default case: air-gap installation not required
	slog.Debug("read-only install is not required", "namespace", namespace, "name", clusterName, "node", nodeUuid, "controlPlaneProviderType", clusterTemplate.Spec.ControlPlaneProviderType)
	return false, nil
}

func convertClusterNetwork(network *ct.ClusterNetwork) *capi.ClusterNetwork {
	pods := &capi.NetworkRanges{}
	services := &capi.NetworkRanges{}

	if network != nil {
		if network.Pods != nil {
			pods.CIDRBlocks = network.Pods.CIDRBlocks
		}

		if network.Services != nil {
			services.CIDRBlocks = network.Services.CIDRBlocks
		}
	}
	return &capi.ClusterNetwork{Pods: pods, Services: services}
}
