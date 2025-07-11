// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"

	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (GET /v2/healthz) TODO: remove healthz from oapi code generation and implement separately to the generated server handler
func (s *Server) GetV2Healthz(ctx context.Context, request api.GetV2HealthzRequestObject) (api.GetV2HealthzResponseObject, error) {
	return api.GetV2Healthz200JSONResponse("cm rest server is healthy"), nil
}
