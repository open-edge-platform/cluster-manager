// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events

import (
	"fmt"
	"log/slog"
	"time"
)

// default event timeout
const eventTimeout = 3 * time.Second

// Event is an interface that defines a method to handle events
type Event interface {
	handle() error
	output() chan<- error
}

// Sink is a function that creates a channel to sink events and starts a goroutine to handle them
func Sink() chan<- Event {
	event := make(chan Event)

	go func() {
		slog.Debug("event sink started", "type", fmt.Sprintf("%T", event))
		for e := range event {
			err := e.handle()
			if err != nil {
				slog.Error("failed to handle event", "event", e, "error", err)
			}
			if out := e.output(); out != nil {
				select {
				case out <- err:
				case <-time.After(eventTimeout):
					slog.Error("event output channel timed out", "event", e)
				}
			}
		}
		slog.Debug("event sink closed", "type", fmt.Sprintf("%T", event))
	}()

	return event
}
