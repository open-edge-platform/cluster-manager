// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	v1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/providers"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
	"github.com/open-edge-platform/orch-library/go/pkg/tenancy"
)

const (
	appName                        = "cluster-manager"
	podSecurityAdmissionSecretName = "pod-security-admission-config"
)

var (
	GetK8sClientFunc = k8s.New().WithInClusterConfig

	GetTemplatesFunc = func() ([]*v1alpha1.ClusterTemplate, error) {
		return template.ReadDefaultTemplates()
	}

	GetPodSecurityAdmissionConfigFunc = func() (map[string][]byte, error) {
		return template.ReadPodSecurityAdmissionConfigs()
	}

	defaultTemplate string
)

// TenancyHandler implements tenancy.Handler for the cluster-manager controller.
// It handles project created/deleted events by creating or cleaning up
// K8s namespaces and cluster templates.
type TenancyHandler struct {
	k8s             *k8s.Client
	templates       []*v1alpha1.ClusterTemplate
	psaData         map[string][]byte
	defaultTemplate string
}

// NewTenancyHandler creates a new TenancyHandler with the required K8s client,
// templates, and pod security admission data.
func NewTenancyHandler() (*TenancyHandler, error) {
	k8sClient := GetK8sClientFunc()
	if k8sClient == nil {
		return nil, fmt.Errorf("failed to get kubernetes client")
	}

	templates, err := GetTemplatesFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to read cluster templates: %w", err)
	}

	psaData, err := GetPodSecurityAdmissionConfigFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to read pod security admission configs: %w", err)
	}

	return &TenancyHandler{
		k8s:             k8sClient,
		templates:       templates,
		psaData:         psaData,
		defaultTemplate: defaultTemplate,
	}, nil
}

// SetDefaultTemplate allows setting the default template
func SetDefaultTemplate(name string) {
	defaultTemplate = name
}

// HandleEvent implements tenancy.Handler. It dispatches project created/deleted
// events to the appropriate setup or cleanup logic.
func (h *TenancyHandler) HandleEvent(ctx context.Context, event tenancy.Event) error {
	if event.ResourceType != "project" {
		return nil // cluster-manager only handles project events
	}

	projectID := event.ResourceID.String()
	projectName := event.ResourceName

	switch event.EventType {
	case "created":
		slog.Debug("project create event received", "project", projectName, "project_id", projectID)
		if err := h.setupProject(ctx, projectID, projectName); err != nil {
			slog.Error("creation of project cluster resources failed", "error", err)
			return err
		}
		slog.Info("successfully created project cluster resources", "project", projectName, "project_id", projectID)
		return nil

	case "deleted":
		slog.Debug("project delete event received", "project", projectName, "project_id", projectID)
		if err := h.cleanupProject(ctx, projectID, projectName); err != nil {
			slog.Error("cleanup of project cluster resources failed", "error", err)
			return err
		}
		slog.Info("successfully cleaned up project cluster resources", "project", projectName, "project_id", projectID)
		return nil

	default:
		slog.Debug("ignoring unknown event type", "eventType", event.EventType)
		return nil
	}
}

func (h *TenancyHandler) setupProject(ctx context.Context, projectID, projectName string) error {
	// Create namespace
	err := h.k8s.CreateNamespace(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to create namespace for project: %v", err)
	} else {
		slog.Debug("created namespace for project", "namespace", projectID, "project", projectName)
	}

	// Create Pod Security Admission secret
	if err := h.k8s.CreateSecret(ctx, projectID, podSecurityAdmissionSecretName, h.psaData); err != nil {
		return fmt.Errorf("failed to create pod security admission secret in namespace '%s': %v", projectID, err)
	} else {
		slog.Debug("created pod security admission secret", "namespace", projectID, "project", projectName)
	}

	// Apply templates
	for _, template := range h.templates {
		err = h.k8s.CreateTemplate(ctx, projectID, template)
		if err != nil {
			return fmt.Errorf("failed to create '%s' template: %v", template.GetName(), err)
		} else {
			slog.Debug("created template", "namespace", projectID, "template", template.GetName(), "project", projectName)
		}
	}

	// Set the default template for the project.
	// Selection order:
	// 1. Use an existing valid default template if already set for the project.
	// 2. Use the default template specified in the configuration, if available.
	// 3. Otherwise, use the first template from the available template list.
	if err := h.setDefaultTemplate(ctx, projectID); err != nil {
		return fmt.Errorf("failed to set default template for project '%s': %v", projectName, err)
	} else {
		slog.Debug("labeled default template", "namespace", projectID, "project", projectName)
	}

	return nil
}

func (h *TenancyHandler) setDefaultTemplate(ctx context.Context, projectID string) error {
	// Skip if valid default template is already set for the project
	if template, err := h.k8s.DefaultTemplate(ctx, projectID); err == nil && template.Name != "" {
		// Check if the provider type is in supported control plane provider types
		// Otherwise, proceed to find a suitable template
		if slices.Contains(providers.ControlPlaneProviders, template.Spec.ControlPlaneProviderType) {
			slog.Debug("default template already set for project", "namespace", projectID, "template", template.GetName())
			return nil
		}

		// If the template is with invalid control plane provider type,
		// remove the default label and proceed to find a suitable template
		if err := h.k8s.RemoveTemplateLabels(ctx, projectID, template.Name, labels.DefaultLabelKey); err != nil {
			return fmt.Errorf("failed to remove default label from template '%s' in namespace '%s': %v", template.Name, projectID, err)
		}
		slog.Debug("removed default label from invalid template", "namespace", projectID, "template", template.Name)
	}

	// If no default template is set in the configuration or configured default template is not available,
	// use the first template in the list
	if h.defaultTemplate == "" || !h.k8s.HasTemplate(ctx, projectID, h.defaultTemplate) {
		if len(h.templates) == 0 {
			return fmt.Errorf("no templates available to set as default for project %s", projectID)
		}
		h.defaultTemplate = h.templates[0].GetName()
		slog.Debug("no default template configured, using first template in the list", "namespace", projectID, "template", h.defaultTemplate)
	}

	// Apply default label to the default template
	if err := h.k8s.AddTemplateLabels(ctx, projectID, h.defaultTemplate, map[string]string{
		labels.DefaultLabelKey: labels.DefaultLabelVal,
	}); err != nil {
		return fmt.Errorf("failed to label default template %s in namespace %s: %v", h.defaultTemplate, projectID, err)
	}

	return nil
}

func (h *TenancyHandler) cleanupProject(ctx context.Context, projectID, projectName string) error {
	// Delete all clusters
	err := h.k8s.DeleteClusters(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete clusters: %w", err)
	}

	// Delete all templates
	err = h.k8s.DeleteTemplates(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete templates: %w", err)
	}

	// Delete namespace
	err = h.k8s.DeleteNamespace(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete project namespace: %w", err)
	}

	return nil
}
