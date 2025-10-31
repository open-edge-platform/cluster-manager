// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (DELETE /v2/templates/{name}/{version})
func (s *Server) DeleteV2TemplatesNameVersion(ctx context.Context, request api.DeleteV2TemplatesNameVersionRequestObject) (api.DeleteV2TemplatesNameVersionResponseObject, error) {
	slog.Debug("DeleteV2TemplatesNameVersion", "request", request)

	activeProjectID := request.Params.Activeprojectid.String()

	templateName := request.Name + "-" + request.Version

	err := s.k8sclient.Dynamic().Resource(core.TemplateResourceSchema).Namespace(activeProjectID).Delete(ctx, templateName, v1.DeleteOptions{})
	switch {
	case errors.IsBadRequest(err):
		message := fmt.Sprintf("Template '%s' is invalid: %v", templateName, err)
		slog.Error(message)
		return api.DeleteV2TemplatesNameVersion400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &message}}, nil
	case errors.IsNotFound(err):
		message := fmt.Sprintf("Template '%s' not found: %v", templateName, err)
		slog.Error(message)
		return api.DeleteV2TemplatesNameVersion404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message}}, nil
	case errors.IsConflict(err):
		message := fmt.Sprintf("Template '%s' is in use: %v", templateName, err)
		slog.Error(message)
		return api.DeleteV2TemplatesNameVersion409JSONResponse{N409ConflictJSONResponse: api.N409ConflictJSONResponse{Message: &message}}, nil
	case err != nil:
		message := fmt.Sprintf("Failed to delete template '%s': %v", templateName, err)
		slog.Error(message)
		return api.DeleteV2TemplatesNameVersion500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
	}

	slog.Info("deleted clusterTemplate", "schema", core.TemplateResourceSchema, "namespace", activeProjectID, "name", templateName)
	return api.DeleteV2TemplatesNameVersion204Response{}, nil
}
