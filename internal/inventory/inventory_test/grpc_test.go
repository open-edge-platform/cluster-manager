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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta1"

	computev1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/compute/v1"
	inventoryv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/inventory/v1"
	osv1 "github.com/open-edge-platform/infra-core/inventory/v2/pkg/api/os/v1"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
	"github.com/open-edge-platform/cluster-manager/v2/internal/inventory"
	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
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

	immutableHost := &computev1.HostResource{Instance: &computev1.InstanceResource{Os: &osv1.OperatingSystemResource{OsType: osv1.OsType_OS_TYPE_IMMUTABLE}}}
	mutableHost := &computev1.HostResource{Instance: &computev1.InstanceResource{Os: &osv1.OperatingSystemResource{OsType: osv1.OsType_OS_TYPE_MUTABLE}}}
	nilInstance := &computev1.HostResource{Instance: nil}
	osNil := &computev1.HostResource{Instance: &computev1.InstanceResource{Os: nil}}

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
			name: "os nil",
			mock: func() {
				mockClient.EXPECT().GetHostByUUID(mock.Anything, mock.Anything, mock.Anything).Return(osNil, nil).Once()
			},
			expectedVal: false,
			expectedErr: errors.New("host instance os is nil"),
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
		eventKind           inventoryv1.SubscribeEventsResponse_EventKind
		machine             *capi.Machine
		fallbackMachine     *capi.Machine
		hostMetadata        string
		fallbackClusterName string
		cluster             *capi.Cluster
		getMachineError     error
		fallbackLookupError error
		getClusterError     error
		deleteClusterError  error
		expectDeleteCluster bool
		expectWarning       bool
	}{
		{
			name:      "deauthorized host with assigned cluster - successful deletion",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:      "deauthorized host with no assigned cluster",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
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
			eventKind:           inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
			getMachineError:     errors.New("machine not found"),
			fallbackLookupError: errors.New("machine not found"),
			expectDeleteCluster: false,
			expectWarning:       true,
		},
		{
			name:      "deauthorized host fallback machine lookup succeeds",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
			getMachineError: errors.New("machine not found"),
			fallbackMachine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:      "deauthorized host - cluster deletion fails",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			deleteClusterError:  errors.New("failed to delete cluster"),
			expectDeleteCluster: true,
			expectWarning:       true,
		},
		{
			name:      "deauthorized host assigned non auto-created cluster",
			hostState: computev1.HostState_HOST_STATE_UNTRUSTED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:      "deleted event with auto-created cluster deletes cluster",
			hostState: computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:      "deleted event with non auto-created linked cluster deletes cluster",
			hostState: computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind: inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED,
			machine: &capi.Machine{
				Spec: capi.MachineSpec{
					ClusterName: "test-cluster",
				},
			},
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:                "deleted event machine missing resolves derived cluster name",
			hostState:           computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind:           inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED,
			getMachineError:     errors.New("machine not found"),
			fallbackLookupError: errors.New("machine not found"),
			fallbackClusterName: "cluster-host-12345678",
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:                "deleted event machine missing resolves metadata cluster name",
			hostState:           computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind:           inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED,
			getMachineError:     errors.New("machine not found"),
			fallbackLookupError: errors.New("machine not found"),
			hostMetadata:        `[{"key":"cluster-name","value":"from-metadata"}]`,
			fallbackClusterName: "from-metadata",
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:                "deleted event machine found without cluster name resolves fallback",
			hostState:           computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind:           inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED,
			machine:             &capi.Machine{Spec: capi.MachineSpec{ClusterName: ""}},
			fallbackClusterName: "cluster-host-12345678",
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{labels.AutoCreatedLabelKey: "true"},
				},
			},
			expectDeleteCluster: true,
		},
		{
			name:                "deleted event inferred name skips cluster without auto-created label",
			hostState:           computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind:           inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED,
			getMachineError:     errors.New("machine not found"),
			fallbackLookupError: errors.New("machine not found"),
			fallbackClusterName: "cluster-host-12345678",
			cluster: &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"trusted-compute-compatible": "false"},
				},
			},
			expectDeleteCluster: false,
		},
		{
			name:                "authorized updated host - no cluster deletion",
			hostState:           computev1.HostState_HOST_STATE_ONBOARDED,
			eventKind:           inventoryv1.SubscribeEventsResponse_EVENT_KIND_UPDATED,
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
				Metadata:     tc.hostMetadata,
				CurrentState: tc.hostState,
			}

			// Set up mocks based on test case
			cleanupAttempted := tc.hostState == computev1.HostState_HOST_STATE_UNTRUSTED ||
				tc.eventKind == inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED

			if cleanupAttempted {
				var resolvedMachine *capi.Machine
				if tc.getMachineError != nil {
					mockK8sClient.On("GetMachineByHostID", mock.Anything, testHost.TenantId, testHost.ResourceId).
						Return(capi.Machine{}, tc.getMachineError).Once()

					if tc.fallbackMachine != nil {
						resolvedMachine = tc.fallbackMachine
						mockK8sClient.On("GetMachineByProviderHostID", mock.Anything, testHost.TenantId, testHost.ResourceId).
							Return(*tc.fallbackMachine, nil).Once()
					} else if tc.fallbackLookupError != nil {
						mockK8sClient.On("GetMachineByProviderHostID", mock.Anything, testHost.TenantId, testHost.ResourceId).
							Return(capi.Machine{}, tc.fallbackLookupError).Once()
					}
				} else if tc.machine != nil {
					resolvedMachine = tc.machine
					mockK8sClient.On("GetMachineByHostID", mock.Anything, testHost.TenantId, testHost.ResourceId).
						Return(*tc.machine, nil).Once()
				}

				clusterName := tc.fallbackClusterName
				if resolvedMachine != nil && resolvedMachine.Spec.ClusterName != "" {
					clusterName = resolvedMachine.Spec.ClusterName
				}

				if clusterName != "" {
					if tc.getClusterError != nil {
						mockK8sClient.On("GetCluster", mock.Anything, testHost.TenantId, clusterName).
							Return((*capi.Cluster)(nil), tc.getClusterError).Once()
					} else {
						mockK8sClient.On("GetCluster", mock.Anything, testHost.TenantId, clusterName).
							Return(tc.cluster, nil).Once()
					}

					if tc.expectDeleteCluster {
						mockK8sClient.On("DeleteClusterForCleanup", mock.Anything, testHost.TenantId, clusterName, tc.eventKind == inventoryv1.SubscribeEventsResponse_EVENT_KIND_DELETED).
							Return(tc.deleteClusterError).Once()
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
					EventKind: tc.eventKind,
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
