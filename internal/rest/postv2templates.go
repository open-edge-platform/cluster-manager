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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (POST /v2/templates)
func (s *Server) PostV2Templates(ctx context.Context, request api.PostV2TemplatesRequestObject) (api.PostV2TemplatesResponseObject, error) {
	activeProjectID := request.Params.Activeprojectid.String()
	slog.Debug("creating clusterTemplate", "namespace", activeProjectID)

	name := request.Body.Name

	ct, err := template.FromTemplateInfoToClusterTemplate(*request.Body)
	if err != nil {
		slog.Error("failed to convert templateInfo to clusterTemplate", "templateInfo", request.Body, "error", err)
		return api.PostV2Templates400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr(fmt.Sprintf("invalid clusterconfiguration: %s", err.Error())),
			},
		}, nil
	}

	templateObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&ct)
	if err != nil {
		slog.Error("failed to convert clusterTemplate to unstructured", "clusterTemplate", ct, "error", err)
		return api.PostV2Templates500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	unstructuredClusterTemplate := &unstructured.Unstructured{Object: templateObject}
	_, err = s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(activeProjectID).Create(ctx, unstructuredClusterTemplate, v1.CreateOptions{})

	if err != nil && errors.IsBadRequest(err) {
		slog.Error("failed to create clusterTemplate - invalid clusterTemplate", "namespace", activeProjectID, "name", name, "error", err)
		return api.PostV2Templates400JSONResponse{
			N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	} else if err != nil && errors.IsAlreadyExists(err) {
		slog.Error("failed to create clusterTemplate - clusterTemplate already exists", "namespace", activeProjectID, "name", name, "error", err)
		return api.PostV2Templates409JSONResponse{
			N409ConflictJSONResponse: api.N409ConflictJSONResponse{
				Message: ptr(fmt.Sprintf("template %s already exists", name)),
			},
		}, nil
	} else if err != nil {
		slog.Error("failed to create clusterTemplate", "namespace", activeProjectID, "name", name, "error", err)
		return api.PostV2Templates500JSONResponse{
			N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}

	slog.Info("Cluster Template created", "namespace", activeProjectID, "name", name)

	return api.PostV2Templates201JSONResponse(fmt.Sprintf("successfully imported template %s", name)), nil
}
