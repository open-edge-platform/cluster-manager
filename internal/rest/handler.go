// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"

	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (PUT /v2/clusters/{name}/nodes)
func (s *Server) PutV2ClustersNameNodes(ctx context.Context, request api.PutV2ClustersNameNodesRequestObject) (api.PutV2ClustersNameNodesResponseObject, error) {
	errMsg := "Cluster node updates are not yet supported."
	return api.PutV2ClustersNameNodes501JSONResponse{
		N501NotImplementedJSONResponse: api.N501NotImplementedJSONResponse{
			Message: &errMsg,
		},
	}, nil
}
