// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (DELETE /v2/clusters/{name})
func (s *Server) DeleteV2ClustersName(ctx context.Context, request api.DeleteV2ClustersNameRequestObject) (api.DeleteV2ClustersNameResponseObject, error) {
	name := request.Name
	if name == "" {
		slog.Error("no cluster name provided")
		return api.DeleteV2ClustersName400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr("no cluster name provided"),
			},
		}, nil
	}

	activeProjectID := request.Params.Activeprojectid.String()
	err := s.unpauseClusterIfPaused(ctx, activeProjectID, name)
	if errors.IsNotFound(err) {
		message := fmt.Sprintf("cluster '%s' not found in namespace '%s'", name, activeProjectID)
		return api.DeleteV2ClustersName404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message}}, nil
	}
	if err != nil {
		slog.Error("failed to unpause cluster before deletion", "namespace", activeProjectID, "name", name, "error", err)
		return api.DeleteV2ClustersName500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr("failed to unpause cluster before deletion"),
			},
		}, nil
	}
	err = s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(activeProjectID).Delete(ctx, name, v1.DeleteOptions{})
	if errors.IsNotFound(err) {
		message := fmt.Sprintf("cluster '%s' not found in namespace '%s'", name, activeProjectID)
		return api.DeleteV2ClustersName404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message}}, nil
	}
	if err != nil {
		slog.Error("failed to delete cluster", "namespace", activeProjectID, "name", name, "error", err)
		return api.DeleteV2ClustersName500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr("failed to delete cluster"),
			},
		}, nil
	}

	slog.Debug("cluster deleted", "namespace", activeProjectID, "name", name)
	return api.DeleteV2ClustersName204Response{}, nil
}

func (s *Server) unpauseClusterIfPaused(ctx context.Context, namespace, name string) error {
	cli := s.k8sclient.Resource(core.ClusterResourceSchema).Namespace(namespace)
	clusterObj, err := cli.Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return err
	}
	paused, found, err := unstructured.NestedBool(clusterObj.Object, "spec", "paused")
	if err != nil {
		return err
	}
	if !found || !paused {
		// not paused
		return nil
	}

	err = unstructured.SetNestedField(clusterObj.Object, false, "spec", "paused")
	if err != nil {
		return err
	}

	patchData := []byte(`{"spec":{"paused":false}}`)
	_, err = cli.Patch(ctx, name, types.MergePatchType, patchData, v1.PatchOptions{})
	if err != nil {
		return err
	}

	slog.Info("cluster unpaused before deletion", "namespace", namespace, "name", name)
	return nil
}
