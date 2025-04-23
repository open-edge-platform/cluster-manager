// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"

	ct "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// (GET /v2/templates/{name}/{version})
func (s *Server) GetV2TemplatesNameVersion(ctx context.Context, request api.GetV2TemplatesNameVersionRequestObject) (api.GetV2TemplatesNameVersionResponseObject, error) {
	slog.Debug("GetV2TemplatesNameVersion", "params", request.Params)
	activeProjectID := request.Params.Activeprojectid.String()

	templateName := request.Name + "-" + request.Version
	slog.Debug("getting clusterTemplate", "schema", core.TemplateResourceSchema, "namespace", activeProjectID, "name", templateName)

	unstructuredClusterTemplate, err := s.k8sclient.GetCached(ctx, core.TemplateResourceSchema, activeProjectID, templateName)
	switch {
	case errors.IsNotFound(err):
		slog.Error("clusterTemplate not found", "namespace", activeProjectID, "name", templateName)
		return api.GetV2TemplatesNameVersion404JSONResponse{
			N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{
				Message: ptr(fmt.Sprintf("clusterTemplate '%s-%s' not found", request.Name, request.Version)),
			},
		}, nil
	case err != nil:
		slog.Error("failed to get clusterTemplate", "namespace", activeProjectID, "name", templateName, "error", err)
		return api.GetV2TemplatesNameVersion500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	template, err := s.getTemplate(*unstructuredClusterTemplate)
	if err != nil {
		slog.Error("failed to get clusterTemplate from unstructuredClusterTemplate", "namespace", activeProjectID, "name", templateName, "error", err)
		return api.GetV2TemplatesNameVersion500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	return api.GetV2TemplatesNameVersion200JSONResponse(*template), nil
}

func (s *Server) getTemplate(item unstructured.Unstructured) (*api.TemplateInfo, error) {
	slog.Debug("getTemplate", "item", item)
	clusterTemplate := ct.ClusterTemplate{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &clusterTemplate); err != nil {
		slog.Error("failed to convert unstructured to clusterTemplate", "unstructured", item, "error", err)
		return nil, err
	}

	return template.FromClusterTemplateToTemplateInfo(clusterTemplate)
}
