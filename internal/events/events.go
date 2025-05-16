// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

// Package events provides a framework for defining, handling, and processing asynchronous events.
// It defines an Event interface for event handling, a base implementation for common functionality,
// and utilities for event processing with context-aware cancellation and timeout support.
// The package is designed to facilitate event-driven architectures by enabling decoupled event
// processing and result reporting via channels.
package events

import (
	"context"
	"log/slog"
	"time"
)

// EventTimeout defines the default timeout for event handling responses
const EventTimeout = 3 * time.Second

// Event is an interface that defines methods to handle events
type Event interface {
	// Handle processes the event and returns any error
	Handle(ctx context.Context) error

	// Output returns a channel for sending results back to the caller
	Output() chan<- error
}

// EventBase provides common functionality for events
type EventBase struct {
	Out chan<- error // channel to send error back to the caller
}

// Output returns the output channel for the event
func (e EventBase) Output() chan<- error {
	return e.Out
}

// NewSink creates a channel to receive events and starts a goroutine to process them
func NewSink(ctx context.Context) chan<- Event {
	events := make(chan Event)

	go processEvents(ctx, events)

	return events
}

// processEvents handles incoming events from the provided channel
func processEvents(ctx context.Context, events <-chan Event) {
	slog.Debug("event sink started")

	for {
		select {
		case <-ctx.Done():
			slog.Debug("event sink shutting down due to context cancellation")
			return
		case e, ok := <-events:
			if !ok {
				slog.Debug("event sink closed")
				return
			}

			handleEvent(ctx, e)
		}
	}
}

// handleEvent processes a single event and sends the result to the output channel
func handleEvent(ctx context.Context, e Event) {
	err := e.Handle(ctx)
	if err != nil {
		slog.Error("failed to handle event", "event", e, "error", err)
	}

	out := e.Output()
	if out == nil {
		return
	}

	// Send result with timeout
	sendCtx, cancel := context.WithTimeout(ctx, EventTimeout)
	defer cancel()

	select {
	case out <- err:
		// Successfully sent result
	case <-sendCtx.Done():
		slog.Error("event output channel timed out", "event", e, "timeout", EventTimeout)
	}
}
