// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package multitenancy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	activeWatcher "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectactivewatcher.edge-orchestrator.intel.com/v1"
	watcherv1 "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/apis/projectwatcher.edge-orchestrator.intel.com/v1"
	nexus "github.com/open-edge-platform/orch-utils/tenancy-datamodel/build/nexus-client"
)

func (t *TenancyDatamodel) addProjectWatcher() error {
	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	_, err := t.nexus.TenancyMultiTenancy().Config().AddProjectWatchers(ctx, &watcherv1.ProjectWatcher{ObjectMeta: metav1.ObjectMeta{Name: appName}})
	if nexus.IsAlreadyExists(err) {
		slog.Warn("project watcher already exists", "error", err)
		return nil
	}

	slog.Info("registered as a project watcher", "app", appName)
	return err
}

func (t *TenancyDatamodel) deleteProjectWatcher() error {
	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	err := t.nexus.TenancyMultiTenancy().Config().DeleteProjectWatchers(ctx, appName)
	if nexus.IsNotFound(err) {
		slog.Warn("project watcher does not exist", "error", err)
		return nil
	}

	slog.Info("deregistered as a project watcher", "app", appName)
	return err
}

func verifySynced(handler cache.ResourceEventHandlerRegistration) error {
	var attempts int
	if err := backoff.Retry(func() error {
		attempts++
		if handler.HasSynced() {
			return nil
		}
		return fmt.Errorf("resource event handler has not yet synced after %d attempts", attempts)
	}, backoff.NewExponentialBackOff(
		backoff.WithMaxInterval(5*time.Second),
		backoff.WithMaxElapsedTime(2*time.Minute))); err != nil {
		return err
	}

	slog.Debug("resource event handler successfully synced")
	return nil
}

func updateWatcherStatus(project *nexus.RuntimeprojectRuntimeProject, status activeWatcher.ActiveWatcherStatus, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	watcher, err := project.GetActiveWatchers(ctx, appName)
	if err != nil || watcher == nil {
		slog.Error("failed to get active watcher, unable to set status to new status", "status", status)
		return
	}

	watcher.Spec = activeWatcher.ProjectActiveWatcherSpec{
		StatusIndicator: status,
		Message:         fmt.Sprint(msg),
		TimeStamp:       safeUnixTime(),
	}

	if err = watcher.Update(ctx); err != nil {
		slog.Error("error updating active watcher status to new status", "status", status)
	}
}

func deleteActiveWatcher(project *nexus.RuntimeprojectRuntimeProject) {
	ctx, cancel := context.WithTimeout(context.Background(), nexusContextTimeout)
	defer cancel()

	if err := project.DeleteActiveWatchers(ctx, appName); err != nil {
		if nexus.IsChildNotFound(err) {
			slog.Warn("app does not watch project", "app", appName, "project_name", project.DisplayName(), "project_id", string(project.UID))
			return
		}

		slog.Error("error deleting watcher for project", "app", appName, "project_name", project.DisplayName(), "project_id", string(project.UID))
		return
	}

	slog.Debug("deleted watcher for project", "app", appName, "project_name", project.DisplayName(), "project_id", string(project.UID))
}

func safeUnixTime() uint64 {
	t := time.Now().Unix()
	if t < 0 {
		return 0
	}

	return uint64(t)
}
