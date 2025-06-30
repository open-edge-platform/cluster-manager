// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	activeWatcher "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectactivewatcher.edge-orchestrator.intel.com/v1"
	nexus "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/nexus-client"

	ct "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	provider "github.com/open-edge-platform/cluster-manager/v2/internal/providers"
	"github.com/open-edge-platform/cluster-manager/v2/internal/template"
)

const appName = "cluster-manager"

var (
	// The default baseline regex matches template names like baseline-<provider>-vX.Y.Z. for exmple:
	// baseline-v1.2.3, baseline-k3s-v1.2.3, baseline-rke2-v1.2.3, baseline-kubeadm-v1.2.3..
	baselineRegex         = regexp.MustCompile(`^baseline(?:-[a-zA-Z0-9]+)?-v\d+\.\d+\.\d+$`)
	nexusContextTimeout   = time.Second * 5
	GetClusterConfigFunc  = rest.InClusterConfig
	GetNexusClientSetFunc = nexus.NewForConfig
	GetK8sClientFunc      = k8s.NewClient
	// If disableK3sTemplates is true (defaultProvider == "rke2"), templates like baseline-k3s-vX.Y.Z are excluded.
	// and baseline-vX.Y.Z becomes the default. Later with renaming of templates, this will be removed.
	// If disableK3sTemplates is false, all templates are available and baseline-k3s-vX.Y.Z becomes the default.
	disableK3sTemplates bool
	GetTemplatesFunc    = func() ([]*ct.ClusterTemplate, error) {
		return template.ReadDefaultTemplates(disableK3sTemplates)
	}
)

type TenancyDatamodel struct {
	client    *nexus.Clientset
	k8s       *k8s.Client
	templates []*ct.ClusterTemplate
}

func NewDatamodelClient() (*TenancyDatamodel, error) {
	config, err := GetClusterConfigFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	var client *nexus.Clientset
	if client, err = GetNexusClientSetFunc(config); err != nil {
		return nil, fmt.Errorf("failed to create nexus client: %w", err)
	}

	// Prepare k8s connection
	k8s, err := GetK8sClientFunc(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes: %w", err)
	}

	// Read all the default templates
	templates, err := GetTemplatesFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to read default cluster templates: %w", err)
	}

	return &TenancyDatamodel{client: client, k8s: k8s, templates: templates}, nil
}

func (tdm *TenancyDatamodel) Start() error {
	tdm.client.SubscribeAll()

	if err := tdm.addProjectWatcher(); err != nil {
		return fmt.Errorf("failed to register '%s' as a project watcher: %w", appName, err)
	}

	handler, err := tdm.client.TenancyMultiTenancy().Runtime().Orgs("*").Folders("*").Projects("*").RegisterAddCallback(tdm.processRuntimeProjectsAdd)
	if err != nil {
		slog.Error("failed to register project add callback", "error", err)
		return err
	} else if err = verifySynced(handler); err != nil {
		slog.Error("failed to verify project add handler synced", "error", err)
		return err
	}
	slog.Info("subscribed to project add events")

	handler, err = tdm.client.TenancyMultiTenancy().Runtime().Orgs("*").Folders("*").Projects("*").RegisterUpdateCallback(tdm.processRuntimeProjectsUpdate)
	if err != nil {
		slog.Error("failed to register project update callback", "error", err)
		return err
	} else if err = verifySynced(handler); err != nil {
		slog.Error("failed to verify project update handler synced", "error", err)
		return err
	}
	slog.Info("subscribed to project update events")

	return nil
}

func (tdm *TenancyDatamodel) Stop() {
	tdm.client.UnsubscribeAll()

	if err := tdm.deleteProjectWatcher(); err != nil {
		slog.Warn("error deleting project watcher", "error", err)
	}
}

// SetDisableK3sTemplates allows disabling the k3s templates
func SetDisableK3sTemplates(disable bool) {
	disableK3sTemplates = disable
}

// processRuntimeProjectsAdd is a callback function invoked when a project is added
func (tdm *TenancyDatamodel) processRuntimeProjectsAdd(project *nexus.RuntimeprojectRuntimeProject) {
	slog.Debug("project add event received", "project", project.DisplayName())
	if project.Spec.Deleted {
		tdm.processRuntimeProjectsUpdate(nil, project)
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
		slog.Error("error creating watcher for project", "app", appName, "project_name", project.DisplayName(), "project_id", string(project.UID))
		return
	}
	slog.Debug("created watcher for project", "project_name", project.DisplayName(), "project_id", string(project.UID))

	err := tdm.setupProject(ctx, project)
	if err != nil {
		slog.Error("creation of project cluster resources failed", "error", err)
		updateWatcherStatus(project, activeWatcher.StatusIndicationError, "Error setting up cluster resources for project")
		return
	}

	updateWatcherStatus(project, activeWatcher.StatusIndicationIdle, "Successfully created project")
}

func (tdm *TenancyDatamodel) setupProject(ctx context.Context, project *nexus.RuntimeprojectRuntimeProject) error {
	projectId := string(project.UID)

	// Create namespace
	err := tdm.k8s.CreateNamespace(ctx, projectId)
	if err != nil {
		slog.Warn(fmt.Sprintf("failed to create namespace for project: %v", err))
	} else {
		slog.Debug("created namespace for project", "namespace", projectId, "project", project.DisplayName())
	}

	// Apply templates
	for _, template := range tdm.templates {
		err = tdm.k8s.CreateTemplate(ctx, projectId, template)
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to create '%s' default template: %v", template.GetName(), err))
		} else {
			slog.Debug("created template", "namespace", projectId, "template", template.GetName(), "project", project.DisplayName())
		}
	}

	defaultTemplateName := selectDefaultTemplateName(tdm.templates, disableK3sTemplates)

	if defaultTemplateName == "" {
		slog.Warn("default template not found", "namespace", projectId, "project", project.DisplayName())
		return nil
	}

	// Label default template
	labels := map[string]string{labels.DefaultLabelKey: labels.DefaultLabelVal}
	if err = tdm.k8s.AddTemplateLabels(ctx, projectId, defaultTemplateName, labels); err != nil {
		return fmt.Errorf("failed to label default template: %w", err)
	}

	slog.Debug("labeled default template", "namespace", projectId, "project", project.DisplayName())

	return nil
}

func selectDefaultTemplateName(templates []*ct.ClusterTemplate, disableK3sTemplates bool) string {
	var defaultTemplateName string
Loop:
	for _, t := range templates {
		name := t.GetName()
		if !baselineRegex.MatchString(name) {
			continue
		}
		switch {
		case !disableK3sTemplates && strings.Contains(name, provider.DefaultProvider):
			defaultTemplateName = name
			break Loop
		case defaultTemplateName == "":
			defaultTemplateName = name
		}
	}
	return defaultTemplateName
}

// processRuntimeProjectsUpdate is a callback function invoked when a project is deleted
func (tdm *TenancyDatamodel) processRuntimeProjectsUpdate(_, project *nexus.RuntimeprojectRuntimeProject) {
	slog.Debug("project update event received", "project", project.DisplayName())
	if !project.Spec.Deleted {
		return
	}
	defer deleteActiveWatcher(project)

	updateWatcherStatus(project, activeWatcher.StatusIndicationInProgress, "Deleting edge clusters")

	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	err := tdm.cleanupProject(ctx, project)

	if err != nil {
		slog.Error("cleanup of project cluster resources failed: %w", "error", err)
		updateWatcherStatus(project, activeWatcher.StatusIndicationError, "Error cleaning up cluster resources for project")
		return
	}

	updateWatcherStatus(project, activeWatcher.StatusIndicationIdle, "Successfully cleaned up project")
}

func (tdm *TenancyDatamodel) cleanupProject(ctx context.Context, project *nexus.RuntimeprojectRuntimeProject) error {
	projectId := string(project.UID)

	// Delete all clusters
	err := tdm.k8s.DeleteClusters(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to delete clusters: %w", err)
	}

	// Delete all templates
	err = tdm.k8s.DeleteTemplates(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to delete templates: %w", err)
	}

	// Delete namespace
	err = tdm.k8s.DeleteNamespace(ctx, projectId)
	if err != nil {
		return fmt.Errorf("failed to delete project namespace: %w", err)
	}

	return nil
}
