// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	computev1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/compute/v1"
	inventoryv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/inventory/v1"
	osv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/os/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
	"github.com/open-edge-platform/cluster-manager/v2/internal/inventory"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
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
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(&inventoryv1.GetResourceResponse{}, assert.AnError).Once()
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

	immutableHost := &computev1.HostResource{Instance: &computev1.InstanceResource{DesiredOs: &osv1.OperatingSystemResource{OsType: osv1.OsType_OS_TYPE_IMMUTABLE}}}
	mutableHost := &computev1.HostResource{Instance: &computev1.InstanceResource{DesiredOs: &osv1.OperatingSystemResource{OsType: osv1.OsType_OS_TYPE_MUTABLE}}}
	nilInstance := &computev1.HostResource{Instance: nil}
	desiredOsNil := &computev1.HostResource{Instance: &computev1.InstanceResource{DesiredOs: nil}}

	cases := []struct {
		name        string
		mock        func()
		expectedVal bool
		expectedErr error
	}{
		{
			name: "immutable OS type",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(immutableHost, nil).Once()
			},
			expectedVal: false, // Always return false for now as we don't support immutable EMT with pre-installed K3s packages yet
		},
		{
			name: "immutable OS type resource id",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(
					&inventoryv1.GetResourceResponse{Resource: &inventoryv1.Resource{Resource: &inventoryv1.Resource_Host{Host: immutableHost}}}, nil).Once()
			},
			expectedVal: false, // Always return false for now as we don't support immutable EMT with pre-installed K3s packages yet
		},
		{
			name: "mutable OS type",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(mutableHost, nil).Once()
			},
			expectedVal: false,
		},
		{
			name: "mutable OS type resource id",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(
					&inventoryv1.GetResourceResponse{Resource: &inventoryv1.Resource{Resource: &inventoryv1.Resource_Host{Host: mutableHost}}}, nil).Once()
			},
			expectedVal: false,
		},
		{
			name: "host instance nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nilInstance, nil).Once()
			},
			expectedVal: false,
			expectedErr: errors.New("host instance is nil"),
		},
		{
			name: "desired OS nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(desiredOsNil, nil).Once()
			},
			expectedVal: false,
			expectedErr: errors.New("host instance desired os is nil"),
		},
		{
			name: "error fetching host",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).Return(nil, assert.AnError).Once()
			},
			expectedVal: false,
			expectedErr: assert.AnError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mock()

			invClient, err := inventory.NewInventoryClientWithOptions(inventory.Options{})
			require.NoError(t, err)

			val, err := invClient.IsImmutable(context.Background(), "test_tenant_id", "test_host_uuid")
			assert.Equal(t, tc.expectedVal, val)
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

func TestWatchHosts_DeleteClusterOnDeauthorizedHost(t *testing.T) {
	mockClient := mocks.NewMockTenantAwareInventoryClient(t)
	mockK8sClient := k8s.NewMockK8sWrapperClient(t)

	inventory.GetInventoryClientFunc = func(ctx context.Context, cfg client.InventoryClientConfig) (client.TenantAwareInventoryClient, error) {
		return mockClient, nil
	}

	cases := []struct {
		name                string
		hostState           computev1.HostState
		machine             *capi.Machine
		getMachineError     error
		deleteClusterError  error
		expectDeleteCluster bool
		expectWarning       bool
	}{
		{
			name:      "deauthorized host with assigned cluster - successful deletion",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:      "deauthorized host with no assigned cluster",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "", // No cluster assigned
				},
			},
			expectDeleteCluster: false,
		},
		{
			name:                "deauthorized host - machine not found",
			hostState:           computev1.HostState_HOST_STATE_UNTRUSTED,
			getMachineError:     errors.New("machine not found"),
			expectDeleteCluster: false,
			expectWarning:       true,
		},
		{
			name:      "deauthorized host - cluster deletion fails",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			deleteClusterError:  errors.New("failed to delete cluster"),
			expectDeleteCluster: true,
			expectWarning:       true,
		},
		{
			name:                "authorized host - no cluster deletion",
			hostState:           computev1.HostState_HOST_STATE_ONBOARDED,
			expectDeleteCluster: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test host event
			testHost := &computev1.HostResource{
				ResourceId:   "host-12345678",                        // Valid format: host-[0-9a-f]{8}
				TenantId:     "123e4567-e89b-12d3-a456-426614174000", // Valid UUID
				Name:         "test-host",
				CurrentState: tc.hostState,
			}

			// Set up mocks based on test case
			if tc.hostState == computev1.HostState_HOST_STATE_UNTRUSTED {
				if tc.getMachineError != nil {
					mockK8sClient.On("GetMachineByHostID", mock.Anything, testHost.TenantId, testHost.ResourceId).
						Return(capi.Machine{}, tc.getMachineError).Once()
				} else if tc.machine != nil {
					mockK8sClient.On("GetMachineByHostID", mock.Anything, testHost.TenantId, testHost.ResourceId).
						Return(*tc.machine, nil).Once()

					if tc.expectDeleteCluster && tc.machine.Spec.ClusterName != "" {
						if tc.deleteClusterError != nil {
							mockK8sClient.On("DeleteCluster", mock.Anything, testHost.TenantId, tc.machine.Spec.ClusterName).
								Return(tc.deleteClusterError).Once()
						} else {
							mockK8sClient.On("DeleteCluster", mock.Anything, testHost.TenantId, tc.machine.Spec.ClusterName).
								Return(nil).Once()
						}
					}
				}
			}

			// Create inventory client with mocked k8s client
			invClient := inventory.NewTestInventoryClient(mockK8sClient, mockClient)

			// Create a channel to capture host events
			hostEvents := make(chan events.Event, 1)

			// Create the watch event that would come from the inventory service
			watchEvent := &client.WatchEvents{
				Event: &inventoryv1.SubscribeEventsResponse{
					EventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED, // Or CREATED
					Resource: &inventoryv1.Resource{
						Resource: &inventoryv1.Resource_Host{
							Host: testHost,
						},
					},
				},
			}

			// Simulate the event being received
			go func() {
				invClient.InjectEvent(watchEvent)
				invClient.CloseEvents() // Close to stop the watch loop
			}()

			// Start watching hosts
			invClient.WatchHosts(hostEvents)

			// Give some time for the goroutine to process
			time.Sleep(100 * time.Millisecond)

			// Verify all expectations were met
			mockK8sClient.AssertExpectations(t)

			// If we expect a regular event (not just deletion), check it was sent
			if tc.hostState != computev1.HostState_HOST_STATE_UNTRUSTED {
				select {
				case event := <-hostEvents:
					assert.NotNil(t, event)
				case <-time.After(100 * time.Millisecond):
					t.Error("Expected to receive a host event")
				}
			}
		})
	}
}
