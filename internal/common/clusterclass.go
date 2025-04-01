// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func GetClusterClass(name types.NamespacedName) capiv1beta1.ClusterClass {
	return capiv1beta1.ClusterClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: capiv1beta1.ClusterClassSpec{
			ControlPlane: capiv1beta1.ControlPlaneClass{
				LocalObjectTemplate: capiv1beta1.LocalObjectTemplate{
					Ref: &corev1.ObjectReference{
						APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
						Name:       name.Name,
					},
				},
				MachineInfrastructure: &capiv1beta1.LocalObjectTemplate{
					Ref: &corev1.ObjectReference{
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
						Name:       fmt.Sprintf("%s-controlplane", name.Name),
					},
				},
				MachineHealthCheck: &capiv1beta1.MachineHealthCheckClass{
					UnhealthyConditions: []capiv1beta1.UnhealthyCondition{
						{Status: corev1.ConditionUnknown, Timeout: metav1.Duration{Duration: 300 * time.Second}, Type: corev1.NodeReady},
						{Status: corev1.ConditionFalse, Timeout: metav1.Duration{Duration: 300 * time.Second}, Type: corev1.NodeReady},
					},
				},
			},
			Infrastructure: capiv1beta1.LocalObjectTemplate{
				Ref: &corev1.ObjectReference{
					APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					Name:       name.Name,
				},
			},
		},
	}
}
