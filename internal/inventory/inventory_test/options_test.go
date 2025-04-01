// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory_test

import (
	"sync"
	"testing"

	"github.com/open-edge-platform/cluster-manager/internal/inventory"
	"github.com/stretchr/testify/assert"
)

func TestOptionsBuilder(t *testing.T) {
	cases := []struct {
		name              string
		waitGroup         *sync.WaitGroup
		inventoryAddress  string
		enableTracing     bool
		enableMetrics     bool
		expectedWaitGroup *sync.WaitGroup
		expectedAddress   string
		expectedTracing   bool
		expectedMetrics   bool
	}{
		{
			name:              "all options set",
			waitGroup:         &sync.WaitGroup{},
			inventoryAddress:  "localhost:8080",
			enableTracing:     true,
			enableMetrics:     true,
			expectedWaitGroup: &sync.WaitGroup{},
			expectedAddress:   "localhost:8080",
			expectedTracing:   true,
			expectedMetrics:   true,
		},
		{
			name:              "only address set",
			waitGroup:         nil,
			inventoryAddress:  "localhost:9090",
			enableTracing:     false,
			enableMetrics:     false,
			expectedWaitGroup: nil,
			expectedAddress:   "localhost:9090",
			expectedTracing:   false,
			expectedMetrics:   false,
		},
		{
			name:              "tracing and metrics enabled",
			waitGroup:         nil,
			inventoryAddress:  "",
			enableTracing:     true,
			enableMetrics:     true,
			expectedWaitGroup: nil,
			expectedAddress:   "",
			expectedTracing:   true,
			expectedMetrics:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builder := inventory.NewOptionsBuilder().
				WithWaitGroup(tc.waitGroup).
				WithInventoryAddress(tc.inventoryAddress).
				WithTracing(tc.enableTracing).
				WithMetrics(tc.enableMetrics)

			options := builder.Build()

			assert.Equal(t, tc.expectedWaitGroup, options.WaitGroup())
			assert.Equal(t, tc.expectedAddress, options.InventoryAddress())
			assert.Equal(t, tc.expectedTracing, options.TracingEnabled())
			assert.Equal(t, tc.expectedMetrics, options.MetricsEnabled())
		})
	}
}
