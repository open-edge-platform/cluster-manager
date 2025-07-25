// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	computev1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/compute/v1"
	osv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/os/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/inventory"
	mocks "github.com/open-edge-platform/cluster-manager/v2/internal/mocks/client"
	"github.com/open-edge-platform/infra-core/inventory/v2/pkg/client"
)

func TestNewInventoryClientWithOptions(t *testing.T) {
	cases := []struct {
		name        string
		options     inventory.Options
		clientFunc  func(ctx context.Context, cfg client.InventoryClientConfig) (client.TenantAwareInventoryClient, error)
		expectedErr error
	}{
		{
			name: "successful creation",
			options: inventory.NewOptionsBuilder().
				WithInventoryAddress("localhost:50051").
				WithTracing(true).
				WithMetrics(true).
				Build(),
			clientFunc: func(ctx context.Context, cfg client.InventoryClientConfig) (client.TenantAwareInventoryClient, error) {
				return nil, nil
			},
		},
		{
			name: "failed creation",
			options: inventory.NewOptionsBuilder().
				WithInventoryAddress("invalid_address").
				Build(),
			clientFunc: func(ctx context.Context, cfg client.InventoryClientConfig) (client.TenantAwareInventoryClient, error) {
				return nil, assert.AnError
			},
			expectedErr: assert.AnError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory.GetInventoryClientFunc = tc.clientFunc

			client, err := inventory.NewInventoryClientWithOptions(tc.options)
			if tc.expectedErr != nil {
				assert.Nil(t, client)
				assert.Equal(t, tc.expectedErr, err)
				return
			}

			assert.NotNil(t, client)
			assert.Nil(t, err)
		})
	}
}

func TestGetHostTrustedCompute(t *testing.T) {
	mockClient := mocks.NewMockTenantAwareInventoryClient(t)
	inventory.GetInventoryClientFunc = func(ctx context.Context, cfg client.InventoryClientConfig) (client.TenantAwareInventoryClient, error) {
		return mockClient, nil
	}

	cases := []struct {
		name        string
		mock        func()
		expectedVal bool
		expectedErr error
	}{
		{
			name: "trusted compute true",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{
					Instance: &computev1.InstanceResource{
						SecurityFeature: osv1.SecurityFeature_SECURITY_FEATURE_SECURE_BOOT_AND_FULL_DISK_ENCRYPTION,
					},
				}, nil).Once()
			},
			expectedVal: true,
		},
		{
			name: "trusted compute false",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{
					Instance: &computev1.InstanceResource{
						SecurityFeature: osv1.SecurityFeature_SECURITY_FEATURE_NONE,
					},
				}, nil).Once()
			},
			expectedVal: false,
		},
		{
			name: "host instance nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{}, nil).Once()
			},
			expectedErr: errors.New("host instance is nil"),
			expectedVal: false,
		},
		{
			name: "host resource nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
			},
			expectedErr: errors.New("empty host resource"),
		},
		{
			name: "error getting host",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
			},
			expectedErr: assert.AnError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mock()

			invClient, err := inventory.NewInventoryClientWithOptions(inventory.Options{})
			require.NoError(t, err)

			trustedCompute, err := invClient.GetHostTrustedCompute(context.Background(), "test_tenant_id", "test_host_uuid")
			assert.Equal(t, tc.expectedVal, trustedCompute)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestIsImmutable(t *testing.T) {
	mockClient := mocks.NewMockTenantAwareInventoryClient(t)
	inventory.GetInventoryClientFunc = func(ctx context.Context, cfg client.InventoryClientConfig) (client.TenantAwareInventoryClient, error) {
		return mockClient, nil
	}

	cases := []struct {
		name        string
		mock        func()
		expectedVal bool
		expectedErr error
	}{
		{
			name: "immutable OS type",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{
					Instance: &computev1.InstanceResource{
						DesiredOs: &osv1.OperatingSystemResource{
							OsType: osv1.OsType_OS_TYPE_IMMUTABLE,
						},
					},
				}, nil).Once()
			},
			// IsImmutable always return false for now
			expectedVal: false,
		},
		{
			name: "mutable OS type",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{
					Instance: &computev1.InstanceResource{
						DesiredOs: &osv1.OperatingSystemResource{
							OsType: osv1.OsType_OS_TYPE_MUTABLE,
						},
					},
				}, nil).Once()
			},
			expectedVal: false,
		},
		{
			name: "host instance nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{}, nil).Once()
			},
			expectedErr: errors.New("host instance is nil"),
			expectedVal: false,
		},
		{
			name: "current OS nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(&computev1.HostResource{
					Instance: &computev1.InstanceResource{},
				}, nil).Once()
			},
			expectedErr: errors.New("host instance current os is nil"),
			expectedVal: false,
		},
		{
			name: "error fetching host",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
			},
			expectedErr: assert.AnError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mock()

			invClient, err := inventory.NewInventoryClientWithOptions(inventory.Options{})
			require.NoError(t, err)

			readOnlyInstall, err := invClient.IsImmutable(context.Background(), "test_tenant_id", "test_host_uuid")
			assert.Equal(t, tc.expectedVal, readOnlyInstall)
			assert.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestJsonStringToMap(t *testing.T) {
	cases := []struct {
		name     string
		jsonStr  string
		expected map[string]string
	}{
		{
			name:    "real-world example",
			jsonStr: `[{"key":"host-label","value":"true"},{"key":"test-label","value":"true"}]`,
			expected: map[string]string{
				"host-label": "true",
				"test-label": "true",
			},
		},
		{
			name:    "real-world example 2",
			jsonStr: `[{"key":"cluster-name","value":""},{"key":"app-id","value":""}]`,
			expected: map[string]string{
				"cluster-name": "",
				"app-id":       "",
			},
		},
		{
			name:     "empty brackets string",
			jsonStr:  `[]`,
			expected: map[string]string{},
		},
		{
			name:     "empty json string",
			jsonStr:  "",
			expected: map[string]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := inventory.JsonStringToMap(tc.jsonStr)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
