// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
)

// (PUT /v2/templates/{name}/default)
func (s *Server) PutV2TemplatesNameDefault(ctx context.Context, request api.PutV2TemplatesNameDefaultRequestObject) (api.PutV2TemplatesNameDefaultResponseObject, error) {
	activeProjectID := request.Params.Activeprojectid.String()
	slog.Debug("handling request to set default template", "namespace", activeProjectID, "templateName", request.Name, "version", request.Body.Version)

	if request.Body.Version == "" {
		version, err := s.fetchAndSelectLatestVersion(ctx, request.Name, activeProjectID)
		if err != nil {
			errMsg := "failed to fetch and select latest version"
			slog.Error(errMsg, "namespace", activeProjectID, "error", err)
			return api.PutV2TemplatesNameDefault500JSONResponse{
				N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{
					Message: &errMsg,
				},
			}, nil
		}

		request.Body.Version = version
		slog.Debug("version set to latest", "version", request.Body.Version)
	}

	err := s.setDefaultTemplate(ctx, request.Name, request.Body.Version, activeProjectID)
	switch {
	case k8serrors.IsBadRequest(err):
		errMsg := "failed to set default template"
		slog.Error(errMsg, "name", request.Name, "version", request.Body.Version, "error", err)
		return api.PutV2TemplatesNameDefault400JSONResponse{N400BadRequestJSONResponse: api.N400BadRequestJSONResponse{Message: &errMsg}}, nil
	case k8serrors.IsNotFound(err):
		errMsg := "resource not found"
		slog.Error(errMsg, "name", request.Name, "version", request.Body.Version, "error", err)
		return api.PutV2TemplatesNameDefault404JSONResponse{N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{Message: &errMsg}}, nil
	case err != nil:
		errMsg := "unexpected error occurred"
		slog.Error(errMsg, "namespace", activeProjectID, "error", err)
		return api.PutV2TemplatesNameDefault500JSONResponse{N500InternalServerErrorJSONResponse: api.N500InternalServerErrorJSONResponse{Message: &errMsg}}, nil
	}

	slog.Info("default cluster template set", "namespace", activeProjectID, "name", request.Name, "version", request.Body.Version)

	return api.PutV2TemplatesNameDefault200Response{}, nil

}

// setDefaultTemplate sets the specified template as the default template for the given project by updating its labels and unlabeling any existing default templates.
func (s *Server) setDefaultTemplate(ctx context.Context, name string, version string, projectId string) error {
	if projectId == "" {
		return k8serrors.NewBadRequest("project ID is missing")
	}

	if name == "" || version == "" {
		return k8serrors.NewBadRequest("failed to extract name or version from request")
	}

	templateName := name + "-" + version
	slog.Debug("checking if cluster template is already default", "schema", core.TemplateResourceSchema, "namespace", projectId, "templateName", templateName)
	unstructuredClusterTemplate, err := s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(projectId).Get(ctx, templateName, v1.GetOptions{})
	if err != nil || unstructuredClusterTemplate == nil {
		slog.Error("failed to get cluster template", "namespace", projectId, "templateName", templateName, "error", err)
		if k8serrors.IsNotFound(err) {
			return k8serrors.NewNotFound(schema.GroupResource{Group: "template.x-k8s.io", Resource: "templates"}, templateName)
		}
		if k8serrors.IsBadRequest(err) {
			return k8serrors.NewBadRequest("failed to get cluster template")
		}
		return err
	}

	labels := unstructuredClusterTemplate.GetLabels()
	if labels != nil {
		if val, ok := labels["default"]; ok && val == "true" {
			slog.Debug("cluster template is already the default", "namespace", projectId, "templateName", templateName)
			return nil
		}
	} else {
		labels = make(map[string]string)
	}

	// Unlabel existing default cluster templates
	labelSelector := "default=true"
	slog.Debug("listing cluster templates to remove default label", "schema", core.TemplateResourceSchema, "namespace", projectId, "selector", labelSelector)
	unstructuredClusterTemplatesList, err := s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(projectId).List(ctx, v1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		slog.Error("failed to list cluster templates", "namespace", projectId, "error", err)
		return err
	}

	for _, item := range unstructuredClusterTemplatesList.Items {
		labels := item.GetLabels()
		if labels == nil {
			continue
		}

		if _, ok := labels["default"]; !ok {
			continue
		}

		delete(labels, "default")
		item.SetLabels(labels)
		if _, err := s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(projectId).Update(ctx, &item, v1.UpdateOptions{}); err != nil {
			errMsg := "unexpected error occurred: " + err.Error()
			slog.Error("failed to unlabel cluster template", "namespace", projectId, "templateName", item.GetName(), "error", err)
			return k8serrors.NewInternalError(errors.New(errMsg))
		}
		slog.Info("default cluster template unset", "namespace", projectId, "name", item.GetName(), "version", version)
	}

	// Mark the given cluster template with default:true label
	slog.Debug("setting default cluster template", "schema", core.TemplateResourceSchema, "namespace", projectId, "templateName", templateName)
	labels["default"] = "true"
	unstructuredClusterTemplate.SetLabels(labels)
	_, err = s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(projectId).Update(ctx, unstructuredClusterTemplate, v1.UpdateOptions{})
	if err != nil {
		errMsg := "unexpected error occurred: "
		slog.Error(errMsg, "namespace", projectId, "name", templateName, "error", err)
		return errors.New(errMsg)
	}
	slog.Info("default cluster template", "namespace", projectId, "name", templateName, "version", version)
	return nil
}

// fetchAndSelectLatestVersion fetches the template versions and selects the latest version.
func (s *Server) fetchAndSelectLatestVersion(ctx context.Context, templateName, namespace string) (string, error) {
	slog.Debug("fetchAndSelectLatestVersion", "templateName", templateName, "namespace", namespace)

	// List all templates in the namespace
	unstructuredClusterTemplatesList, err := s.k8sclient.Resource(core.TemplateResourceSchema).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		slog.Error("failed to list clusterTemplates", "namespace", namespace, "error", err)
		return "", err
	}

	versions := []string{}

	for _, item := range unstructuredClusterTemplatesList.Items {
		templateInfo, err := s.getTemplate(item)
		if err != nil {
			slog.Error("failed to get template", "error", err)
			return "", err
		}
		if templateName == templateInfo.Name {
			versions = append(versions, templateInfo.Version)
		}
	}

	if len(versions) == 0 {
		slog.Error("clusterTemplate not found", "namespace", namespace, "name", templateName)
		return "", fmt.Errorf("clusterTemplate with name '%s' not found", templateName)
	}

	// Sort versions to find the latest one
	sort.Strings(versions)
	latestVersion := versions[len(versions)-1]
	slog.Debug("Selected latest version", "latestVersion", latestVersion)

	return latestVersion, nil
}
