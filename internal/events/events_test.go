// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
)

const paralellism = 10

func TestConcurrentEventHandling(t *testing.T) {
	ctx := context.Background()
	sink := events.NewSink(ctx)

	// Create output channels to track results
	outputs := make([]chan error, paralellism)
	for i := range outputs {
		outputs[i] = make(chan error, 1)
	}

	// Launch many events concurrently
	var wg sync.WaitGroup
	for i := range paralellism {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := events.DummyEvent{EventBase: events.EventBase{Out: outputs[i]}, ID: i}
			sink <- event
		}()
	}

	// Wait for all events to be sent
	wg.Wait()

	// Verify all events were processed
	for i, out := range outputs {
		select {
		case err := <-out:
			require.NoError(t, err, "Event %d failed", i)
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Timeout waiting for event %d", i)
		}
	}

	close(sink)
}

func TestNilOutputChannel(t *testing.T) {
	ctx := context.Background()
	sink := events.NewSink(ctx)

	// Send an event with a nil output channel
	event := events.DummyEvent{
		EventBase: events.EventBase{Out: nil},
		ID:        42,
	}

	// This should not panic
	sink <- event

	// Allow some time for processing
	time.Sleep(50 * time.Millisecond)

	close(sink)
	// Test passes if no panic occurs
}

func TestContextCancellation(t *testing.T) {
	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	sink := events.NewSink(ctx)

	// Send a couple of events before cancellation
	outputs := make([]chan error, 2)
	for i := range outputs {
		outputs[i] = make(chan error, 1)
		event := events.DummyEvent{
			EventBase: events.EventBase{Out: outputs[i]},
			ID:        i,
		}
		sink <- event
	}

	// Verify these events are processed normally
	for i, out := range outputs {
		select {
		case err := <-out:
			require.NoError(t, err, "Event %d failed", i)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for event %d", i)
		}
	}

	// Cancel the context
	cancel()

	// Try to send one more event after cancellation
	outputAfterCancel := make(chan error, 1)
	afterCancelEvent := events.DummyEvent{
		EventBase: events.EventBase{Out: outputAfterCancel},
		ID:        999,
	}

	// The event system should time out
	select {
	case sink <- afterCancelEvent:
		t.Fatal("Expected send to block after context cancellation")
	case <-time.After(100 * time.Millisecond):
		// This is expected - the event should not be processed
	}
}

func TestErrorPropagation(t *testing.T) {
	ctx := context.Background()
	sink := events.NewSink(ctx)

	// Create an event that will return an error
	output := make(chan error, 1)
	event := ErrorEvent{
		EventBase:     events.EventBase{Out: output},
		ErrorToReturn: errors.New("simulated error"),
	}

	// Send the event
	sink <- event

	// Verify the error is correctly propagated
	select {
	case err := <-output:
		require.Error(t, err)
		require.Equal(t, "simulated error", err.Error())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for error to be propagated")
	}

	close(sink)
}

// ErrorEvent is a test event that always returns a specific error
type ErrorEvent struct {
	events.EventBase
	ErrorToReturn error
}

func (e ErrorEvent) Handle(ctx context.Context) error {
	return e.ErrorToReturn
}
