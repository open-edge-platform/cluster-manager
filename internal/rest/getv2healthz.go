// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"

	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

// (GET /v2/healthz)
func (s *Server) GetV2Healthz(ctx context.Context, request api.GetV2HealthzRequestObject) (api.GetV2HealthzResponseObject, error) {
	return api.GetV2Healthz200JSONResponse("cm rest server is healthy"), nil
}
