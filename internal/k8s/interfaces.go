// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"context"

	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

//go:generate mockery --name=K8sWrapperClient --output=. --filename=mock_k8s_wrapper_client.go

// K8sWrapperClient defines the interface for k8s client operations needed by other packages
type K8sWrapperClient interface {
	GetMachineByHostID(ctx context.Context, namespace, hostID string) (capi.Machine, error)
	DeleteCluster(ctx context.Context, namespace string, clusterName string) error
	SetMachineLabels(ctx context.Context, namespace string, machineName string, newUserLabels map[string]string) error
}
