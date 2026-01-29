// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	activeWatcher "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectactivewatcher.edge-orchestrator.intel.com/v1"
	nexus "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/nexus-client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

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
	GetClusterConfigFunc  = rest.InClusterConfig
	GetK8sClientFunc      = k8s.New().WithInClusterConfig
	GetNexusClientSetFunc = nexus.NewForConfig
	nexusContextTimeout   = time.Second * 5

	GetTemplatesFunc = func() ([]*v1alpha1.ClusterTemplate, error) {
		return template.ReadDefaultTemplates()
	}

	GetPodSecurityAdmissionConfigFunc = func() (map[string][]byte, error) {
		return template.ReadPodSecurityAdmissionConfigs()
	}

	defaultTemplate string
)

type TenancyDatamodel struct {
	nexus           *nexus.Clientset
	k8s             *k8s.Client
	templates       []*v1alpha1.ClusterTemplate
	psaData         map[string][]byte
	defaultTemplate string
}

func NewDatamodelClient() (*TenancyDatamodel, error) {
	config, err := GetClusterConfigFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get orch kubernetes config: %w", err)
	}

	nexusClient, err := GetNexusClientSetFunc(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create nexus client: %w", err)
	}

	k8sClient := GetK8sClientFunc()
	if k8sClient == nil {
		return nil, fmt.Errorf("failed to get kubernetes client: %w", err)
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
		nexus:           nexusClient,
		k8s:             k8sClient,
		templates:       templates,
		psaData:         psaData,
		defaultTemplate: defaultTemplate,
	}, nil
}

func (t *TenancyDatamodel) Start() error {
	t.nexus.SubscribeAll()

	if err := t.addProjectWatcher(); err != nil {
		return fmt.Errorf("failed to register '%s' as a project watcher: %w", appName, err)
	}

	if handler, err := t.nexus.TenancyMultiTenancy().Runtime().Orgs("*").Folders("*").Projects("*").
		RegisterAddCallback(t.processRuntimeProjectsAdd); err != nil {
		slog.Error("failed to register project add callback", "error", err)
		return err
		// used for the overrideable sync check to allow fake clients in tests.
	} else if err = verifySyncedFunc(handler); err != nil {
		slog.Error("failed to verify project add handler synced", "error", err)
		return err
	}
	slog.Info("subscribed to project add events")

	if handler, err := t.nexus.TenancyMultiTenancy().Runtime().Orgs("*").Folders("*").Projects("*").
		RegisterUpdateCallback(t.processRuntimeProjectsUpdate); err != nil {
		slog.Error("failed to register project update callback", "error", err)
		return err
	} else if err = verifySyncedFunc(handler); err != nil {
		slog.Error("failed to verify project update handler synced", "error", err)
		return err
	}
	slog.Info("subscribed to project update events")

	return nil
}

func (t *TenancyDatamodel) Stop() {
	t.nexus.UnsubscribeAll()

	if err := t.deleteProjectWatcher(); err != nil {
		slog.Warn("error deleting project watcher", "error", err)
	}
}

// SetDefaultTemplate allows setting the default template
func SetDefaultTemplate(name string) {
	defaultTemplate = name
}

// processRuntimeProjectsAdd is a callback function invoked when a project is added
func (t *TenancyDatamodel) processRuntimeProjectsAdd(project *nexus.RuntimeprojectRuntimeProject) {
	slog.Debug("project add event received", "project", project.DisplayName())
	if project.Spec.Deleted {
		t.processRuntimeProjectsUpdate(nil, project)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	if _, err := project.AddActiveWatchers(ctx, &activeWatcher.ProjectActiveWatcher{
		ObjectMeta: metav1.ObjectMeta{Name: appName},
		Spec: activeWatcher.ProjectActiveWatcherSpec{
			StatusIndicator: activeWatcher.StatusIndicationInProgress,
			Message:         fmt.Sprintf("%s subscribed to project %s", appName, project.DisplayName()),
			TimeStamp:       safeUnixTime(),
		},
	}); err != nil {
		slog.Error("error creating watcher for project",
			"app", appName,
			"project_name", project.DisplayName(),
			"project_id", string(project.UID))
		return
	}

	slog.Debug("created watcher for project", "project_name", project.DisplayName(), "project_id", string(project.UID))

	err := t.setupProject(ctx, project)
	if err != nil {
		slog.Error("creation of project cluster resources failed", "error", err)
		updateWatcherStatus(project, activeWatcher.StatusIndicationError, "Error setting up cluster resources for project")
		return
	}

	updateWatcherStatus(project, activeWatcher.StatusIndicationIdle, "Successfully created project")
}

func (t *TenancyDatamodel) setupProject(ctx context.Context, project *nexus.RuntimeprojectRuntimeProject) error {
	projectId := string(project.UID)

	// Create namespace
	err := t.k8s.CreateNamespace(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to create namespace for project: %v", err)
	} else {
		slog.Debug("created namespace for project", "namespace", projectId, "project", project.DisplayName())
	}

	// Create Pod Security Admission secret
	if err := t.k8s.CreateSecret(ctx, projectId, podSecurityAdmissionSecretName, t.psaData); err != nil {
		return fmt.Errorf("failed to create pod security admission secret in namespace '%s': %v", projectId, err)
	} else {
		slog.Debug("created pod security admission secret", "namespace", projectId, "project", project.DisplayName())
	}

	// Apply templates
	for _, template := range t.templates {
		err = t.k8s.CreateTemplate(ctx, projectId, template)
		if err != nil {
			return fmt.Errorf("failed to create '%s' template: %v", template.GetName(), err)
		} else {
			slog.Debug("created template", "namespace", projectId, "template", template.GetName(), "project", project.DisplayName())
		}
	}

	// Set the default template for the project.
	// Selection order:
	// 1. Use an existing valid default template if already set for the project.
	// 2. Use the default template specified in the configuration, if available.
	// 3. Otherwise, use the first template from the available template list.
	if err := t.setDefaultTemplate(ctx, projectId); err != nil {
		return fmt.Errorf("failed to set default template for project '%s': %v", project.DisplayName(), err)
	} else {
		slog.Debug("labeled default template", "namespace", projectId, "project", project.DisplayName())
	}

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

// processRuntimeProjectsUpdate is a callback function invoked when a project is deleted
func (t *TenancyDatamodel) processRuntimeProjectsUpdate(_, project *nexus.RuntimeprojectRuntimeProject) {
	slog.Debug("project update event received", "project", project.DisplayName())
	if !project.Spec.Deleted {
		return
	}
	defer deleteActiveWatcher(project)

	updateWatcherStatus(project, activeWatcher.StatusIndicationInProgress, "Deleting edge clusters")

	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	err := t.cleanupProject(ctx, project)

	if err != nil {
		slog.Error("cleanup of project cluster resources failed: %w", "error", err)
		updateWatcherStatus(project, activeWatcher.StatusIndicationError, "Error cleaning up cluster resources for project")
		return
	}

	updateWatcherStatus(project, activeWatcher.StatusIndicationIdle, "Successfully cleaned up project")
}

func (t *TenancyDatamodel) cleanupProject(ctx context.Context, project *nexus.RuntimeprojectRuntimeProject) error {
	projectId := string(project.UID)

	// Delete all clusters
	err := t.k8s.DeleteClusters(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to delete clusters: %w", err)
	}

	// Delete all templates
	err = t.k8s.DeleteTemplates(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to delete templates: %w", err)
	}

	// Delete namespace
	err = t.k8s.DeleteNamespace(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to delete project namespace: %w", err)
	}

	return nil
}
