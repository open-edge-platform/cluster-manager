// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package events_test

import (
	"context"
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
