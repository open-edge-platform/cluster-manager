// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmcpv1beta1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	dockerv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"

	kthreescpv1beta2 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
)

var _ = Describe("ClusterTemplate Controller", func() {
	const resourceName = "test-resource"
	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default",
	}

	DescribeTable("Reconcile ClusterTemplate resources",
		func(controlPlaneProviderType, infraProviderType, kubernetesVersion, clusterConfiguration string, validateResources func()) {
			clustertemplate := &clusterv1alpha1.ClusterTemplate{}

			By("creating the custom resource for the Kind ClusterTemplate")
			err := k8sClient.Get(ctx, typeNamespacedName, clustertemplate)
			if err != nil && errors.IsNotFound(err) {
				resource := &clusterv1alpha1.ClusterTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      typeNamespacedName.Name,
						Namespace: typeNamespacedName.Namespace,
					},
					Spec: clusterv1alpha1.ClusterTemplateSpec{
						ControlPlaneProviderType: controlPlaneProviderType,
						InfraProviderType:        infraProviderType,
						KubernetesVersion:        kubernetesVersion,
						ClusterConfiguration:     clusterConfiguration,
						ClusterNetwork: clusterv1alpha1.ClusterNetwork{
							Services: &clusterv1alpha1.NetworkRanges{
								CIDRBlocks: []string{"10.43.0.0/16"},
							},
							Pods: &clusterv1alpha1.NetworkRanges{
								CIDRBlocks: []string{"10.42.0.0/16"},
							},
						},
						ClusterLabels: map[string]string{
							"default-extension": "privileged",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("Reconciling the created resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, clustertemplate)).To(Succeed())
			Expect(clustertemplate.Status.Ready).To(BeTrue())

			// Different combinations of control-plane and infrastructure provider types
			// result in the creation of various resources. The validation function
			// must handle different struct types, which is why this logic was
			// moved to a separate function.
			validateResources()

			By("validating the ClusterClass is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &capiv1beta1.ClusterClass{})
			Expect(err).NotTo(HaveOccurred())

			resource := &clusterv1alpha1.ClusterTemplate{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("validating the ClusterClass Reference is set")
			Expect(resource.Status.ClusterClassRef).NotTo(BeNil())

			By("validating the finalizer is present")
			Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeTrue())

			By("Cleanup the specific resource instance ClusterTemplate")
			Expect(k8sClient.Delete(ctx, clustertemplate)).To(Succeed())

			By("Reconciling the deleted resource")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("validating the finalizer is removed")
			err = k8sClient.Get(ctx, typeNamespacedName, clustertemplate)
			if err == nil {
				Expect(controllerutil.ContainsFinalizer(clustertemplate, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeFalse())
			}
		},

		Entry("for Kubeadm CP and Docker Infra",
			"kubeadm", "docker", "v1.30.6",
			"{\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta1\",\"kind\":\"KubeadmControlPlaneTemplate\",\"metadata\":{\"name\":\"kubeadm-control-plane-template-v0.1.0\"},\"spec\":{\"template\":{\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"apiServer\":{\"certSANs\":[\"localhost\",\"127.0.0.1\",\"0.0.0.0\",\"host.docker.internal\"]}},\"initConfiguration\":{\"nodeRegistration\":{}},\"joinConfiguration\":{\"nodeRegistration\":{}},\"postKubeadmCommands\":[\"kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml\"]}}}}}",
			func() {
				By("validating the DockerMachineTemplate is created")
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-controlplane", typeNamespacedName.Name),
					Namespace: typeNamespacedName.Namespace,
				}, &dockerv1beta1.DockerMachineTemplate{})
				Expect(err).NotTo(HaveOccurred())

				By("validating the DockerClusterTemplate is created")
				err = k8sClient.Get(ctx, typeNamespacedName, &dockerv1beta1.DockerClusterTemplate{})
				Expect(err).NotTo(HaveOccurred())

				By("validating the KubeadmControlPlaneTemplate is created")
				err = k8sClient.Get(ctx, typeNamespacedName, &kubeadmcpv1beta1.KubeadmControlPlaneTemplate{})
				Expect(err).NotTo(HaveOccurred())
			},
		),

		Entry("for K3s CP and Docker Infra",
			"k3s", "docker", "v1.33.5+k3s1",
			"{\"kind\":\"KThreesControlPlaneTemplate\",\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta2\",\"spec\":{\"template\":{\"spec\":{\"kthreesConfigSpec\":{\"agentConfig\":{\"airGapped\":false},\"preK3sCommands\":[\"echo hello\"]}}}}}",
			func() {
				By("validating the DockerMachineTemplate is created")
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-controlplane", typeNamespacedName.Name),
					Namespace: typeNamespacedName.Namespace,
				}, &dockerv1beta1.DockerMachineTemplate{})
				Expect(err).NotTo(HaveOccurred())

				By("validating the DockerClusterTemplate is created")
				err = k8sClient.Get(ctx, typeNamespacedName, &dockerv1beta1.DockerClusterTemplate{})
				Expect(err).NotTo(HaveOccurred())

				By("validating the KThreesControlPlaneTemplate is created")
				err = k8sClient.Get(ctx, typeNamespacedName, &kthreescpv1beta2.KThreesControlPlaneTemplate{})
				Expect(err).NotTo(HaveOccurred())
			},
		),

		Entry("for K3s CP and Intel Infra",
			"k3s", "intel", "v1.33.5+k3s1",
			"{\"kind\":\"KThreesControlPlaneTemplate\",\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta2\",\"spec\":{\"template\":{\"spec\":{\"kthreesConfigSpec\":{\"agentConfig\":{\"airGapped\":false},\"preK3sCommands\":[\"echo hello\"]}}}}}",
			func() {
				By("validating the IntelMachineTemplate is created")
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      fmt.Sprintf("%s-controlplane", typeNamespacedName.Name),
					Namespace: typeNamespacedName.Namespace,
				}, &intelv1alpha1.IntelMachineTemplate{})
				Expect(err).NotTo(HaveOccurred())

				By("validating the IntelClusterTemplate is created")
				err = k8sClient.Get(ctx, typeNamespacedName, &intelv1alpha1.IntelClusterTemplate{})
				Expect(err).NotTo(HaveOccurred())

				By("validating the KThreesControlPlaneTemplate is created")
				err = k8sClient.Get(ctx, typeNamespacedName, &kthreescpv1beta2.KThreesControlPlaneTemplate{})
				Expect(err).NotTo(HaveOccurred())
			},
		),
	)
})
