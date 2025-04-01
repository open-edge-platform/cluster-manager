// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package convert

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestAnyToUnstructured(t *testing.T) {
	t.Run("Convert CAPI Cluster to Unstructured", func(t *testing.T) {
		cluster := capi.Cluster{
			Spec: capi.ClusterSpec{
				Topology: &capi.Topology{
					Version: "v1.20.0",
				},
			},
		}

		unstructuredObj, err := ToUnstructured(cluster)
		require.NoError(t, err, "AnyToUnstructured() error = %v, want nil", err)
		require.NotNil(t, unstructuredObj, "AnyToUnstructured() returned nil, want not nil")

		// Check if the unstructured object has the expected fields
		spec, found, err := unstructured.NestedMap(unstructuredObj.Object, "spec")
		require.NoError(t, err, "NestedMap() error = %v, want nil", err)
		require.True(t, found, "NestedMap() found = %v, want true", found)
		require.Equal(t, "v1.20.0", spec["topology"].(map[string]any)["version"], "Cluster version = %v, want %v", spec["topology"].(map[string]interface{})["version"], "v1.20.0")
	})

	t.Run("Convert Invalid Object to Unstructured", func(t *testing.T) {
		invalidObject := make(chan int) // Channels cannot be converted to unstructured

		unstructuredObj, err := ToUnstructured(invalidObject)
		require.Error(t, err, "AnyToUnstructured() error = nil, want non-nil")
		require.Nil(t, unstructuredObj, "AnyToUnstructured() returned non-nil, want nil")
	})
}

func TestMapStringToAny(t *testing.T) {
	t.Run("Table driven tests for MapStringToAny", func(t *testing.T) {
		tests := []struct {
			name     string
			input    map[string]string
			expected map[string]string
		}{
			{
				name: "Convert map[string]string to map[string]any",
				input: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				expected: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			{
				name:     "Convert empty map[string]string to map[string]any",
				input:    map[string]string{},
				expected: map[string]string{},
			},
			{
				name:     "Convert nil map[string]string to map[string]any",
				input:    nil,
				expected: map[string]string{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				converted := MapStringToAny(tt.input)
				require.Equal(t, tt.expected, MapAnyToString(converted), "MapStringToAny() returned %v, want %v", MapAnyToString(converted), tt.expected)
			})
		}
	})
}

func TestMapAnyToString(t *testing.T) {
	t.Run("Table driven tests for MapAnyToString", func(t *testing.T) {
		tests := []struct {
			name     string
			input    map[string]any
			expected map[string]any
		}{
			{
				name: "Convert map[string]any to map[string]string",
				input: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
				expected: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
			{
				name:     "Convert empty map[string]any to map[string]string",
				input:    map[string]any{},
				expected: map[string]any{},
			},
			{
				name:     "Convert nil map[string]any to map[string]string",
				input:    nil,
				expected: map[string]any{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				converted := MapAnyToString(tt.input)
				require.Equal(t, tt.expected, MapStringToAny(converted), "MapAnyToString() returned %v, want %v", MapStringToAny(converted), tt.expected)
			})
		}
	})
}
