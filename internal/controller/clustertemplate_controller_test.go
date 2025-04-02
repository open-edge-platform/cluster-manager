// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rke2cpv1beta1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmcpv1beta1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	dockerv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"

	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
)

var _ = Describe("ClusterTemplate Controller for Kubeadm CP and Docker Infra", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		clustertemplate := &clusterv1alpha1.ClusterTemplate{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterTemplate")
			err := k8sClient.Get(ctx, typeNamespacedName, clustertemplate)
			if err != nil && errors.IsNotFound(err) {
				resource := &clusterv1alpha1.ClusterTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: clusterv1alpha1.ClusterTemplateSpec{
						ControlPlaneProviderType: "kubeadm",
						InfraProviderType:        "docker",
						KubernetesVersion:        "v1.30.6",
						ClusterConfiguration:     "{\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta1\",\"kind\":\"KubeadmControlPlaneTemplate\",\"metadata\":{\"name\":\"kubeadm-control-plane-template-v0.1.0\"},\"spec\":{\"template\":{\"spec\":{\"kubeadmConfigSpec\":{\"clusterConfiguration\":{\"apiServer\":{\"certSANs\":[\"localhost\",\"127.0.0.1\",\"0.0.0.0\",\"host.docker.internal\"]}},\"initConfiguration\":{\"nodeRegistration\":{}},\"joinConfiguration\":{\"nodeRegistration\":{}},\"postKubeadmCommands\":[\"kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml\"]}}}}}",
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
		})

		AfterEach(func() {
			resource := &clusterv1alpha1.ClusterTemplate{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("validating the ClusterClass Reference is set")
			Expect(resource.Status.ClusterClassRef).NotTo(BeNil())

			By("validating the finalizer is present")
			Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeTrue())

			By("Cleanup the specific resource instance ClusterTemplate")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Reconciling the deleted resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("validating the finalizer is removed")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeFalse())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, clustertemplate)).To(Succeed())
			Expect(clustertemplate.Status.Ready).To(BeTrue())

			By("validating the DockerMachineTemplate is created")
			err = k8sClient.Get(ctx, types.NamespacedName{
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

			By("validating the ClusterClass is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &capiv1beta1.ClusterClass{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("ClusterTemplate Controller for RKE2 CP and Docker Infra", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		clustertemplate := &clusterv1alpha1.ClusterTemplate{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterTemplate")
			err := k8sClient.Get(ctx, typeNamespacedName, clustertemplate)
			if err != nil && errors.IsNotFound(err) {
				resource := &clusterv1alpha1.ClusterTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: clusterv1alpha1.ClusterTemplateSpec{
						ControlPlaneProviderType: "rke2",
						InfraProviderType:        "docker",
						KubernetesVersion:        "v1.30.6+rke2r1",
						ClusterConfiguration:     "{\"kind\":\"RKE2ControlPlaneTemplate\",\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta1\",\"spec\":{\"template\":{\"spec\":{\"serverConfig\":{\"cni\":\"calico\",\"disableComponents\":{\"kubernetesComponents\":[\"cloudController\"]}},\"nodeDrainTimeout\":\"2m\",\"rolloutStrategy\":{\"type\":\"RollingUpdate\",\"rollingUpdate\":{\"maxSurge\":1}}}}}}",
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
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &clusterv1alpha1.ClusterTemplate{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("validating the finalizer is present")
			Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeTrue())

			By("Cleanup the specific resource instance ClusterTemplate")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Reconciling the deleted resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("validating the finalizer is removed")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeFalse())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, clustertemplate)).To(Succeed())
			Expect(clustertemplate.Status.Ready).To(BeTrue())

			By("validating the DockerMachineTemplate is created")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      fmt.Sprintf("%s-controlplane", typeNamespacedName.Name),
				Namespace: typeNamespacedName.Namespace,
			}, &dockerv1beta1.DockerMachineTemplate{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the Prerequisites ConfigMap is created")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      fmt.Sprintf("%s-rke2-class-lb-config", typeNamespacedName.Name),
				Namespace: typeNamespacedName.Namespace,
			}, &corev1.ConfigMap{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the DockerClusterTemplate is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &dockerv1beta1.DockerClusterTemplate{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the RKE2ControlPlaneTemplate is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &rke2cpv1beta1.RKE2ControlPlaneTemplate{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the ClusterClass is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &capiv1beta1.ClusterClass{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("ClusterTemplate Controller for RKE2 CP and Intel Infra", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		clustertemplate := &clusterv1alpha1.ClusterTemplate{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterTemplate")
			err := k8sClient.Get(ctx, typeNamespacedName, clustertemplate)
			if err != nil && errors.IsNotFound(err) {
				resource := &clusterv1alpha1.ClusterTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: clusterv1alpha1.ClusterTemplateSpec{
						ControlPlaneProviderType: "rke2",
						InfraProviderType:        "intel",
						KubernetesVersion:        "v1.30.6+rke2r1",
						ClusterConfiguration:     "{\"kind\":\"RKE2ControlPlaneTemplate\",\"apiVersion\":\"controlplane.cluster.x-k8s.io/v1beta1\",\"spec\":{\"template\":{\"spec\":{\"serverConfig\":{\"cni\":\"calico\",\"disableComponents\":{\"kubernetesComponents\":[\"cloudController\"]}},\"nodeDrainTimeout\":\"2m\",\"rolloutStrategy\":{\"type\":\"RollingUpdate\",\"rollingUpdate\":{\"maxSurge\":1}}}}}}",
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
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &clusterv1alpha1.ClusterTemplate{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("validating the finalizer is present")
			Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeTrue())

			By("Cleanup the specific resource instance ClusterTemplate")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Reconciling the deleted resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("validating the finalizer is removed")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(controllerutil.ContainsFinalizer(resource, clusterv1alpha1.ClusterTemplateFinalizer)).To(BeFalse())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterTemplateReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, clustertemplate)).To(Succeed())
			Expect(clustertemplate.Status.Ready).To(BeTrue())

			By("validating the IntelMachineTemplate is created")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      fmt.Sprintf("%s-controlplane", typeNamespacedName.Name),
				Namespace: typeNamespacedName.Namespace,
			}, &intelv1alpha1.IntelMachineTemplate{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the IntelClusterTemplate is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &intelv1alpha1.IntelClusterTemplate{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the RKE2ControlPlaneTemplate is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &rke2cpv1beta1.RKE2ControlPlaneTemplate{})
			Expect(err).NotTo(HaveOccurred())

			By("validating the ClusterClass is created")
			err = k8sClient.Get(ctx, typeNamespacedName, &capiv1beta1.ClusterClass{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
