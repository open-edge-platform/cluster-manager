package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
)

// HostUpdateEvent is an event that is triggered when a host is updated in the Inventory
type HostUpdate struct {
	HostId    string
	ProjectId string
	Labels    map[string]string
	K8scli    *k8s.Client
}

// HostCreatedEvent is an event that is triggered when a host is created in the Inventory
type HostCreated struct {
	HostId    string
	ProjectId string
}

// HostDeletedEvent is an event that is triggered when a host is deleted in the Inventory
type HostDeletedEvent struct {
	HostId    string
	ProjectId string
}

// default event timeout
const eventTimeout = 3 * time.Second

// Event is an interface that defines a method to handle events
type Event interface {
	handle() error
}

// Sink is a function that creates a channel to sink events and starts a goroutine to handle them
func Sink() chan<- Event {
	event := make(chan Event)

	go func() {
		slog.Debug("event sink started", "type", fmt.Sprintf("%T", event))
		for e := range event {
			if err := e.handle(); err != nil {
				slog.Error("failed to handle event", "event", e, "error", err)
			}
		}
		slog.Debug("event sink closed", "type", fmt.Sprintf("%T", event))
	}()

	return event
}

// HostCreated event handler
func (e HostCreated) handle() error {
	slog.Info("HostCreatedEvent", "HostId", e.HostId, "ProjectId", e.ProjectId)
	return nil
}

// HostDeletedEvent event handler
func (e HostDeletedEvent) handle() error {
	slog.Info("HostDeletedEvent", "HostId", e.HostId, "ProjectId", e.ProjectId)
	return nil
}

// HostUpdate event handler
func (e HostUpdate) handle() error {
	slog.Info("HostUpdateEvent", "HostId", e.HostId, "ProjectId", e.ProjectId, "Labels", e.Labels)

	ctx, cancel := context.WithTimeout(context.Background(), eventTimeout)
	defer cancel()

	m, err := e.K8scli.GetMachineByProviderID(ctx, e.ProjectId, e.HostId)
	if err != nil {
		return fmt.Errorf("failed to get machine by provider id: %w", err)
	}

	err = e.K8scli.SetMachineLabels(ctx, e.ProjectId, m.Name, e.Labels)
	if err != nil {
		return fmt.Errorf("failed to set machine labels: %w", err)
	}

	slog.Info("updated machine labels", "name", m.Name, "labels", e.Labels)
	return nil
}
