// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/open-edge-platform/orch-library/go/pkg/tenancy"

	v1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/internal/providers"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
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

// TenancyDatamodel implements tenancy.Handler and manages per-project k8s resources.
type TenancyDatamodel struct {
	k8s             *k8s.Client
	templates       []*v1alpha1.ClusterTemplate
	psaData         map[string][]byte
	defaultTemplate string
}

// NewDatamodelClient creates a TenancyDatamodel ready to be used as a tenancy.Handler.
func NewDatamodelClient() (*TenancyDatamodel, error) {
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

	return &TenancyDatamodel{
		k8s:             k8sClient,
		templates:       templates,
		psaData:         psaData,
		defaultTemplate: defaultTemplate,
	}, nil
}

// SetDefaultTemplate allows setting the default template name used for new projects.
func SetDefaultTemplate(name string) {
	defaultTemplate = name
}

// HandleEvent implements tenancy.Handler. It is called for every project lifecycle
// event (both replay on startup and incremental). Handlers must be idempotent.
func (t *TenancyDatamodel) HandleEvent(ctx context.Context, event tenancy.Event) error {
	if event.ResourceType != tenancy.ResourceTypeProject {
		return nil // cluster-manager only handles project events
	}

	projectId := event.ResourceID.String()
	projectName := event.ResourceName

	switch event.EventType {
	case tenancy.EventTypeCreated:
		slog.Debug("project created event received", "project_name", projectName, "project_id", projectId)
		if err := t.setupProject(ctx, projectId, projectName); err != nil {
			slog.Error("creation of project cluster resources failed", "project_name", projectName, "error", err)
			return err
		}
		slog.Info("successfully set up project", "project_name", projectName, "project_id", projectId)
	case tenancy.EventTypeDeleted:
		slog.Debug("project deleted event received", "project_name", projectName, "project_id", projectId)
		if err := t.cleanupProject(ctx, projectId, projectName); err != nil {
			slog.Error("cleanup of project cluster resources failed", "project_name", projectName, "error", err)
			return err
		}
		slog.Info("successfully cleaned up project", "project_name", projectName, "project_id", projectId)
	default:
		slog.Warn("unrecognized event type, skipping", "event_type", event.EventType)
	}

	return nil
}

func (t *TenancyDatamodel) setupProject(ctx context.Context, projectId, projectName string) error {
	// Create namespace
	if err := t.k8s.CreateNamespace(ctx, projectId); err != nil {
		return fmt.Errorf("failed to create namespace for project: %w", err)
	}
	slog.Debug("created namespace for project", "namespace", projectId, "project", projectName)

	// Create Pod Security Admission secret
	if err := t.k8s.CreateSecret(ctx, projectId, podSecurityAdmissionSecretName, t.psaData); err != nil {
		return fmt.Errorf("failed to create pod security admission secret in namespace '%s': %w", projectId, err)
	}
	slog.Debug("created pod security admission secret", "namespace", projectId, "project", projectName)

	// Apply templates
	for _, tmpl := range t.templates {
		if err := t.k8s.CreateTemplate(ctx, projectId, tmpl); err != nil {
			return fmt.Errorf("failed to create '%s' template: %w", tmpl.GetName(), err)
		}
		slog.Debug("created template", "namespace", projectId, "template", tmpl.GetName(), "project", projectName)
	}

	// Set the default template for the project.
	// Selection order:
	// 1. Use an existing valid default template if already set for the project.
	// 2. Use the default template specified in the configuration, if available.
	// 3. Otherwise, use the first template from the available template list.
	if err := t.setDefaultTemplate(ctx, projectId); err != nil {
		return fmt.Errorf("failed to set default template for project '%s': %w", projectName, err)
	}
	slog.Debug("labeled default template", "namespace", projectId, "project", projectName)

	return nil
}

func (t *TenancyDatamodel) setDefaultTemplate(ctx context.Context, projectId string) error {
	// Skip if valid default template is already set for the project
	if template, err := t.k8s.DefaultTemplate(ctx, projectId); err == nil && template.Name != "" {
		// Check if the provider type is in supported control plane provider types
		// Otherwise, proceed to find a suitable template
		if slices.Contains(providers.ControlPlaneProviders, template.Spec.ControlPlaneProviderType) {
			slog.Debug("default template already set for project", "namespace", projectId, "template", template.GetName())
			return nil
		}

		// If the template is with invalid control plane provider type,
		// remove the default label and proceed to find a suitable template
		if err := t.k8s.RemoveTemplateLabels(ctx, projectId, template.Name, labels.DefaultLabelKey); err != nil {
			return fmt.Errorf("failed to remove default label from template '%s' in namespace '%s': %v", template.Name, projectId, err)
		}
		slog.Debug("removed default label from invalid template", "namespace", projectId, "template", template.Name)
	}

	// If no default template is set in the configuration or configured default template is not available,
	// use the first template in the list
	if t.defaultTemplate == "" || !t.k8s.HasTemplate(ctx, projectId, t.defaultTemplate) {
		if len(t.templates) == 0 {
			return fmt.Errorf("no templates available to set as default for project %s", projectId)
		}
		t.defaultTemplate = t.templates[0].GetName()
		slog.Debug("no default template configured, using first template in the list", "namespace", projectId, "template", t.defaultTemplate)
	}

	// Apply default label to the default template
	if err := t.k8s.AddTemplateLabels(ctx, projectId, t.defaultTemplate, map[string]string{
		labels.DefaultLabelKey: labels.DefaultLabelVal,
	}); err != nil {
		return fmt.Errorf("failed to label default template %s in namespace %s: %v", t.defaultTemplate, projectId, err)
	}

	return nil
}

func (t *TenancyDatamodel) cleanupProject(ctx context.Context, projectId, projectName string) error {
	// Delete all clusters. Ignore not-found: the namespace may have been removed already
	// on a previous run (the poller replays all delete events on startup for idempotency).
	if err := t.k8s.DeleteClusters(ctx, projectId); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete clusters: %w", err)
	}
	slog.Debug("deleted clusters for project", "namespace", projectId, "project", projectName)

	// Delete all templates — same not-found tolerance.
	if err := t.k8s.DeleteTemplates(ctx, projectId); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete templates: %w", err)
	}
	slog.Debug("deleted templates for project", "namespace", projectId, "project", projectName)

	// Delete namespace — same not-found tolerance.
	if err := t.k8s.DeleteNamespace(ctx, projectId); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete project namespace: %w", err)
	}
	slog.Debug("deleted namespace for project", "namespace", projectId, "project", projectName)

	return nil
}
