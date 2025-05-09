// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"k8s.io/apimachinery/pkg/api/errors"
)

// (PUT /v2/clusters/{name}/labels)
func (s *Server) PutV2ClustersNameLabels(ctx context.Context, request api.PutV2ClustersNameLabelsRequestObject) (api.PutV2ClustersNameLabelsResponseObject, error) {
	activeProjectID := request.Params.Activeprojectid.String()

	clusterName := request.Name
	if clusterName == "" {
		errMsg := "no cluster name provided"
		slog.Warn(errMsg, "name", clusterName)
		return api.PutV2ClustersNameLabels400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: &errMsg,
			},
		}, nil
	}

	if request.Body == nil || request.Body.Labels == nil {
		errMsg := "no labels provided"
		slog.Warn(errMsg)
		return api.PutV2ClustersNameLabels400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: &errMsg,
			},
		}, nil
	}

	newLabels := *request.Body.Labels
	if !labels.Valid(newLabels) {
		errMsg := "invalid cluster label keys"
		slog.Warn(errMsg, "labels", newLabels)
		return api.PutV2ClustersNameLabels400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: &errMsg,
			},
		}, nil
	}

	cli, err := k8s.New(k8s.WithDynamicClient(s.k8sclient))
	if err != nil {
		message := fmt.Sprintf("failed to create k8s client: %v", err)
		slog.Error(message)
		return api.PutV2ClustersNameLabels500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
	}

	err = cli.SetClusterLabels(ctx, activeProjectID, clusterName, newLabels)

	switch {
	case errors.IsBadRequest(err):
		message := fmt.Sprintf("cluster '%s' is invalid: %v", clusterName, err)
		slog.Error(message)
		return api.PutV2ClustersNameLabels400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &message}}, nil
	case errors.IsNotFound(err):
		message := fmt.Sprintf("cluster '%s' not found: %v", clusterName, err)
		slog.Error(message)
		return api.PutV2ClustersNameLabels404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message}}, nil
	case err != nil:
		message := fmt.Sprintf("failed to update Cluster '%s': %v", clusterName, err)
		slog.Error(message)
		return api.PutV2ClustersNameLabels500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
	}

	slog.Info("Cluster labels updated", "namespace", activeProjectID, "name", request.Name, "labels", newLabels)
	return api.PutV2ClustersNameLabels200Response{}, nil
}
