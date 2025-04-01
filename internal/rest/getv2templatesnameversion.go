// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// (GET /v2/templates/{name}/versions)
func (s *Server) GetV2TemplatesNameVersions(ctx context.Context, request api.GetV2TemplatesNameVersionsRequestObject) (api.GetV2TemplatesNameVersionsResponseObject, error) {
	slog.Debug("GetV2TemplatesNameVersions", "params", request.Params)
	activeProjectID := request.Params.Activeprojectid.String()

	// TODO: Revisit this. Field selector does not handle regex/wildcards.
	// Gathering all templates and filtering them manually is too much time and memory consuming.
	unstructuredClusterTemplatesList, err := s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(activeProjectID).List(ctx, v1.ListOptions{})
	if err != nil {
		slog.Error("failed to list clusterTemplates", "namespace", activeProjectID, "error", err)
		return api.GetV2TemplatesNameVersions500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	versions := []string{}

	for _, item := range unstructuredClusterTemplatesList.Items {
		template, err := s.getTemplate(item)
		if err != nil {
			slog.Error("failed to get template", "error", err)
			return api.GetV2TemplatesNameVersions500JSONResponse{
				N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
					Message: ptr(err.Error()),
				},
			}, nil
		}
		if request.Name == template.Name {
			versions = append(versions, template.Version)
		}
	}

	if len(versions) == 0 {
		slog.Error("clusterTemplate not found", "namespace", activeProjectID, "name", request.Name)
		return api.GetV2TemplatesNameVersions404JSONResponse{
			N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{
				Message: ptr(fmt.Sprintf("clusterTemplate with name '%s' not found", request.Name)),
			},
		}, nil
	}

	return api.GetV2TemplatesNameVersions200JSONResponse{VersionList: &versions}, nil
}
