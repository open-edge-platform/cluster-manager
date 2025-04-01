// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"

	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

// (PUT /v2/clusters/{name}/template)
func (s *Server) PutV2ClustersNameTemplate(ctx context.Context, request api.PutV2ClustersNameTemplateRequestObject) (api.PutV2ClustersNameTemplateResponseObject, error) {
	errMsg := "In-place cluster updates are not supported. Please delete the cluster and create a new one with updated cluster template."
	return api.PutV2ClustersNameTemplate501JSONResponse{
		N501NotImplementedJSONResponse: api.N501NotImplementedJSONResponse{
			Message: &errMsg,
		},
	}, nil
}
