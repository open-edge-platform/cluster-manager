// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/open-edge-platform/cluster-manager/internal/convert"
	"github.com/open-edge-platform/cluster-manager/internal/core"
	"github.com/open-edge-platform/cluster-manager/internal/k8s"
	. "github.com/open-edge-platform/cluster-manager/internal/pagination"
	"github.com/open-edge-platform/cluster-manager/internal/template"
	"github.com/open-edge-platform/cluster-manager/pkg/api"
)

// (GET /v2/templates)
func (s *Server) GetV2Templates(ctx context.Context, request api.GetV2TemplatesRequestObject) (api.GetV2TemplatesResponseObject, error) {
	slog.Debug("GetV2Templates", "params", request.Params)
	activeProjectID := request.Params.Activeprojectid.String()

	cli, err := k8s.New(k8s.WithDynamicClient(s.k8sclient))
	if err != nil {
		message := fmt.Sprintf("failed to create k8s client: %v", err)
		slog.Error(message)
		return api.GetV2Templates500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
	}

	defaultTemplate, err := getV2TemplateDefault(ctx, cli, activeProjectID, request.Params.Default)
	if err != nil {
		response, is200OK := handleV2TemplateDefaultResponse(err, defaultTemplate)
		if !is200OK {
			return response, nil
		}
	}

	return getV2TemplatesAll(ctx, cli, activeProjectID, request.Params, defaultTemplate)
}

func getV2TemplateDefault(ctx context.Context, cli *k8s.Client, activeProjectID string, defaultParam *bool) (*api.DefaultTemplateInfo, error) {
	if defaultParam == nil { // To always return default template (see pr #76)
		return nil, nil
	}

	defaultTemplate, err := cli.DefaultTemplate(ctx, activeProjectID)
	if err == k8s.ErrDefaultTemplateNotFound {
		message := fmt.Sprintf("default template not found: %v", err)
		slog.Warn(message)
		return nil, fmt.Errorf("%s", message)
	} else if err != nil {
		message := fmt.Sprintf("failed to get default template: %v", err)
		slog.Error(message)
		return nil, fmt.Errorf("%s", message)
	}

	defaultTemplateInfo, err := template.FromClusterTemplateToDefaultTemplateInfo(defaultTemplate)
	if err != nil {
		message := fmt.Sprintf("failed to convert default template to response object: %v", err)
		slog.Error(message)
		return nil, fmt.Errorf("%s", message)
	}
	slog.Info("returned default clusterTemplate", "schema", core.TemplateResourceSchema, "namespace", activeProjectID, "name", defaultTemplate.Name)
	return defaultTemplateInfo, nil
}

func getV2TemplatesAll(ctx context.Context, cli *k8s.Client, activeProjectID string, params any, defaultTemplate *api.DefaultTemplateInfo) (api.GetV2TemplatesResponseObject, error) {
	// get all templates from k8s
	templates, err := cli.Templates(ctx, activeProjectID)
	if err != nil {
		message := fmt.Sprintf("failed to get templates: %v", err)
		slog.Error(message)
		return api.GetV2Templates500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
	}

	pageSize, offset, orderBy, filter, err := ValidateParams(params)
	if err != nil {
		message := fmt.Sprintf("invalid parameters: %v", err)
		slog.Error(message)
		return api.GetV2Templates404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message}}, nil
	}

	// convert to the response object
	var templateInfo []api.TemplateInfo
	for _, tt := range templates {
		t, err := template.FromClusterTemplateToTemplateInfo(tt)
		if err != nil {
			message := fmt.Sprintf("failed to convert template to response object: %v", err)
			slog.Error(message)
			return api.GetV2Templates500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
		}
		templateInfo = append(templateInfo, *t)
	}

	if len(templateInfo) == 0 && defaultTemplate == nil {
		message := "no templates found"
		slog.Warn(message)
		return api.GetV2Templates200JSONResponse{
			DefaultTemplateInfo: nil,
			TemplateInfoList:    &[]api.TemplateInfo{},
			TotalElements:       convert.Int32Ptr(0),
		}, nil
	}

	if filter != nil {
		templateInfo, err = FilterItems(templateInfo, *filter, filterTemplates)
		if err != nil {
			slog.Error("failed to apply filters", "error", err)
			return nil, err
		}
	}

	if orderBy != nil {
		templateInfo, err = OrderItems(templateInfo, *orderBy, orderByTemplates)
		if err != nil {
			slog.Error("failed to apply filters", "error", err)
			return nil, err
		}
	}

	paginatedTemplatesList, err := PaginateItems(templateInfo, *pageSize, *offset)
	if err != nil {
		message := fmt.Sprintf("failed to get paginated templates: %v", err)
		slog.Error(message)
		return api.GetV2Templates500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message}}, nil
	}

	slog.Info("returned clusterTemplates", "schema", core.TemplateResourceSchema, "namespace", activeProjectID, "count", len(templateInfo))
	return api.GetV2Templates200JSONResponse{
		DefaultTemplateInfo: defaultTemplate,
		TemplateInfoList:    paginatedTemplatesList,
		TotalElements:       convert.Int32Ptr(int32(len(templateInfo))),
	}, nil
}

func handleV2TemplateDefaultResponse(err error, defaultTemplate *api.DefaultTemplateInfo) (api.GetV2TemplatesResponseObject, bool) {
	message := err.Error()
	if defaultTemplate != nil || message == "failed to get default template: multiple default templates found" {
		return api.GetV2Templates500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &message},
		}, false
	} else if defaultTemplate == nil || strings.Contains(message, "default template not found") {
		return api.GetV2Templates200JSONResponse{}, true
	}
	return api.GetV2Templates404JSONResponse{
		N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &message},
	}, false
}

func filterTemplates(template api.TemplateInfo, filter *Filter) bool {
	switch filter.Name {
	case "name":
		return MatchWildcard(&template.Name, filter.Value)
	case "version":
		return MatchWildcard(&template.Version, filter.Value)
	default:
		return false
	}
}

func orderByTemplates(template1, template2 api.TemplateInfo, orderBy *OrderBy) bool {
	switch orderBy.Name {
	case "name":
		if orderBy.IsDesc {
			return template1.Name > template2.Name
		}
		return template1.Name < template2.Name
	case "version":
		if orderBy.IsDesc {
			return template1.Version > template2.Version
		}
		return template1.Version < template2.Version
	default:
		return false
	}
}
