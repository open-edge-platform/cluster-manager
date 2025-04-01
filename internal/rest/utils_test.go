// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"testing"
	"time"

	"github.com/open-edge-platform/cluster-manager/pkg/api"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestGetClusterLifecyclePhase(t *testing.T) {
	fixedTime := time.Now()
	tests := map[string]struct {
		cluster        *capi.Cluster
		expectedStatus *api.GenericStatus
	}{
		"no conditions": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{}}},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONUNSPECIFIED),
				Message:   ptr("Condition not found"),
				Timestamp: ptr(uint64(0)),
			},
		},
		"pending": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}},
					Phase:      string(capi.ClusterPhasePending),
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONINPROGRESS),
				Message:   ptr("pending"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"deleting": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}},
					Phase:      string(capi.ClusterPhaseDeleting),
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONINPROGRESS),
				Message:   ptr("deleting"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"provisioned": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}},
					Phase:      string(capi.ClusterPhaseProvisioned),
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONINPROGRESS),
				Message:   ptr("provisioned"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"failed": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{
							Status:             corev1.ConditionFalse,
							Reason:             "SomeReason",
							Message:            "SomeMessage",
							LastTransitionTime: metav1.Time{Time: fixedTime},
						},
					},
					Phase: string(capi.ClusterPhaseFailed),
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONERROR),
				Message:   ptr("failed"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"unknown": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}},
					Phase:      string(capi.ClusterPhaseUnknown),
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONUNSPECIFIED),
				Message:   ptr("unknown"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			status, _ := getClusterLifecyclePhase(tc.cluster)
			assert.Equal(t, tc.expectedStatus.Indicator, status.Indicator)
			assert.Equal(t, tc.expectedStatus.Message, status.Message)
			assert.EqualValues(t, *tc.expectedStatus.Timestamp, *status.Timestamp)
		})
	}
}

func TestGetNodeHealth(t *testing.T) {
	fixedTime := time.Now()

	tests := map[string]struct {
		cluster        *capi.Cluster
		machines       []unstructured.Unstructured
		expectedStatus *api.GenericStatus
	}{
		"all machines healthy": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}}}},
			machines: []unstructured.Unstructured{
				{Object: map[string]interface{}{"status": map[string]interface{}{"phase": string(capi.MachinePhaseRunning), "conditions": []map[string]interface{}{{"type": string(capi.MachineHealthCheckSucceededCondition), "status": string(corev1.ConditionTrue)}}}}},
			},
			expectedStatus: &api.GenericStatus{Indicator: ptr(api.STATUSINDICATIONIDLE), Message: ptr("nodes are healthy"), Timestamp: ptr(uint64(fixedTime.Unix()))},
		},
		"some machines unhealthy": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}}}},
			machines: []unstructured.Unstructured{
				{Object: map[string]interface{}{"status": map[string]interface{}{"phase": string(capi.MachinePhaseRunning), "conditions": []map[string]interface{}{{"type": string(capi.MachineHealthCheckSucceededCondition), "status": string(corev1.ConditionFalse), "message": "NotHealthy"}}}}},
			},
			expectedStatus: &api.GenericStatus{Indicator: ptr(api.STATUSINDICATIONERROR), Message: ptr("nodes are unhealthy (1/1);[: NotHealthy]"), Timestamp: ptr(uint64(fixedTime.Unix()))},
		},
		"machines in provisioning phase": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{{LastTransitionTime: metav1.Time{Time: fixedTime}}}}},
			machines: []unstructured.Unstructured{
				{Object: map[string]interface{}{"status": map[string]interface{}{"phase": string(capi.MachinePhaseProvisioning)}}},
			},
			expectedStatus: &api.GenericStatus{Indicator: ptr(api.STATUSINDICATIONINPROGRESS), Message: ptr("node(s) health unknown (0/1);[: Provisioning]"), Timestamp: ptr(uint64(fixedTime.Unix()))},
		},
		"no cluster conditions": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{}}},
			machines: []unstructured.Unstructured{
				{Object: map[string]interface{}{"status": map[string]interface{}{"phase": string(capi.MachinePhaseRunning), "conditions": []map[string]interface{}{{"type": string(capi.MachineHealthCheckSucceededCondition), "status": string(corev1.ConditionTrue)}}}}},
			},
			expectedStatus: &api.GenericStatus{Indicator: ptr(api.STATUSINDICATIONIDLE), Message: ptr("nodes are healthy"), Timestamp: ptr(uint64(0))},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			status := getNodeHealth(tc.cluster, tc.machines)
			assert.Equal(t, tc.expectedStatus.Indicator, status.Indicator)
			assert.Equal(t, tc.expectedStatus.Message, status.Message)
			assert.EqualValues(t, *tc.expectedStatus.Timestamp, *status.Timestamp)
		})
	}
}

func TestGetComponentReady(t *testing.T) {
	fixedTime := time.Now()

	conditionType := capi.ReadyCondition
	readyMessage := "ready"
	notReadyMessage := "not ready"
	unknownMessage := "unknown"
	notFoundMessage := "Condition not found"

	tests := map[string]struct {
		cluster        *capi.Cluster
		expectedStatus *api.GenericStatus
	}{
		"no conditions": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{}}},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONUNSPECIFIED),
				Message:   ptr(notFoundMessage),
				Timestamp: ptr(uint64(0)),
			},
		},
		"condition not found": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ControlPlaneReadyCondition, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONUNSPECIFIED),
				Message:   ptr(notFoundMessage),
				Timestamp: ptr(uint64(0)),
			},
		},
		"condition true": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONIDLE),
				Message:   ptr(readyMessage),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"condition false": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: corev1.ConditionFalse, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONINPROGRESS),
				Message:   ptr(notReadyMessage + ";"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"condition unknown": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: corev1.ConditionUnknown, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONERROR),
				Message:   ptr(unknownMessage),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"unexpected condition status": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: "UnexpectedStatus", LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONERROR),
				Message:   ptr(unknownMessage),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			status := getComponentReady(tc.cluster, conditionType, readyMessage, notReadyMessage, unknownMessage, notFoundMessage)
			assert.Equal(t, tc.expectedStatus.Indicator, status.Indicator)
			assert.Equal(t, tc.expectedStatus.Message, status.Message)
			assert.EqualValues(t, *tc.expectedStatus.Timestamp, *status.Timestamp)
		})
	}
}

func TestGetProviderStatus(t *testing.T) {
	fixedTime := time.Now()

	tests := map[string]struct {
		cluster        *capi.Cluster
		expectedStatus *api.GenericStatus
	}{
		"no conditions": {
			cluster: &capi.Cluster{Status: capi.ClusterStatus{Conditions: []capi.Condition{}}},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONUNSPECIFIED),
				Message:   ptr("condition not found"),
				Timestamp: ptr(uint64(0)),
			},
		},
		"condition not found": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ControlPlaneReadyCondition, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONUNSPECIFIED),
				Message:   ptr("condition not found"),
				Timestamp: ptr(uint64(0)),
			},
		},
		"condition true": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONIDLE),
				Message:   ptr("ready"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"condition false": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: corev1.ConditionFalse, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONINPROGRESS),
				Message:   ptr("not ready;"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
		"condition unknown": {
			cluster: &capi.Cluster{
				Status: capi.ClusterStatus{
					Conditions: []capi.Condition{
						{Type: capi.ReadyCondition, Status: corev1.ConditionUnknown, LastTransitionTime: metav1.Time{Time: fixedTime}},
					},
				},
			},
			expectedStatus: &api.GenericStatus{
				Indicator: ptr(api.STATUSINDICATIONERROR),
				Message:   ptr("unknown"),
				Timestamp: ptr(uint64(fixedTime.Unix())),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			status := getProviderStatus(tc.cluster)
			assert.Equal(t, tc.expectedStatus.Indicator, status.Indicator)
			assert.Equal(t, tc.expectedStatus.Message, status.Message)
			assert.EqualValues(t, *tc.expectedStatus.Timestamp, *status.Timestamp)
		})
	}
}
