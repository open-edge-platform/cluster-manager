// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
)

var (
	ErrEmptyHostID    = errors.New("host id is empty")
	ErrEmptyProjectID = errors.New("project id is empty")
	ErrNilK8sClient   = errors.New("k8s client is nil")
	ErrNilLabels      = errors.New("labels are nil")
)

// HostEventBase contains common fields for all host events
type HostEventBase struct {
	EventBase
	HostId    string
	ProjectId string
}

// Validate checks if the common host event fields are valid
func (e HostEventBase) validate() error {
	if e.HostId == "" {
		return ErrEmptyHostID
	}
	if e.ProjectId == "" {
		return ErrEmptyProjectID
	}
	return nil
}

// HostUpdated is an event triggered when a host is updated in the Inventory
type HostUpdated struct {
	HostEventBase
	Labels map[string]string
	K8scli *k8s.Client
}

// HostCreated is an event triggered when a host is created in the Inventory
type HostCreated struct {
	HostEventBase
}

// HostDeleted is an event triggered when a host is deleted in the Inventory
type HostDeleted struct {
	HostEventBase
}

// Handle the HostCreated event
func (e HostCreated) Handle(ctx context.Context) error {
	slog.Info("HostCreated", "HostId", e.HostId, "ProjectId", e.ProjectId)
	return e.validate()
}

// Handle the HostDeleted event
func (e HostDeleted) Handle(ctx context.Context) error {
	slog.Info("HostDeleted", "HostId", e.HostId, "ProjectId", e.ProjectId)
	return e.validate()
}

// validateHostUpdated performs additional validation specific to HostUpdated
func (e HostUpdated) validateHostUpdated() error {
	if err := e.validate(); err != nil {
		return err
	}

	if e.K8scli == nil {
		return ErrNilK8sClient
	}

	if e.Labels == nil {
		return ErrNilLabels
	}

	return nil
}

// Handle the HostUpdate event
func (e HostUpdated) Handle(ctx context.Context) error {
	slog.Info("HostUpdate", "HostId", e.HostId, "ProjectId", e.ProjectId, "Labels", e.Labels)

	// Validate all fields
	if err := e.validateHostUpdated(); err != nil {
		return err
	}

	// Use the provided context instead of creating a new one
	timeoutCtx, cancel := context.WithTimeout(ctx, EventTimeout)
	defer cancel()

	m, err := e.K8scli.GetMachineByHostID(timeoutCtx, e.ProjectId, e.HostId)
	if err != nil {
		return fmt.Errorf("failed to get machine by host id: %w", err)
	}
	slog.Debug("found machine", "name", m.Name, "labels", m.Labels)

	err = e.K8scli.SetMachineLabels(timeoutCtx, e.ProjectId, m.Name, e.Labels)
	if err != nil {
		return fmt.Errorf("failed to set machine labels: %w", err)
	}

	slog.Info("updated machine labels", "name", m.Name, "labels", e.Labels)
	return nil
}
