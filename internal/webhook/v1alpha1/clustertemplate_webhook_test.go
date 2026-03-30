// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"

	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
)

var _ = Describe("ClusterTemplate Webhook", func() {
	var (
		obj       *clusterv1alpha1.ClusterTemplate
		oldObj    *clusterv1alpha1.ClusterTemplate
		validator ClusterTemplateCustomValidator
	)

	BeforeEach(func() {
		obj = &clusterv1alpha1.ClusterTemplate{}
		oldObj = &clusterv1alpha1.ClusterTemplate{}
		validator = ClusterTemplateCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	Context("When creating or updating ClusterTemplate under Validating Webhook", func() {
		It("Should deny templates with an invalid controlplane template or unsupported provider", func() {
			By("simulating an invalid unsupported provider")
			obj.Spec.ControlPlaneProviderType = "invalid-provider"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("invalid control plane provider"))

			By("simulating an invalid controlplane template")
			obj.Spec.ControlPlaneProviderType = "kubeadm"
			obj.Spec.ClusterConfiguration = "{invalid-spec}}"
			_, err = validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to convert cluster configuration"))

		})

		It("Should admit valid templates", func() {
			By("reading the kubeadm template from file")
			kubeadmTemplate := &clusterv1alpha1.ClusterTemplate{}
			kubeadmTemplateFile, err := os.ReadFile("../../../examples/cluster_v1alpha1_clustertemplate_kubeadm.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to read kubeadm template file")
			err = yaml.Unmarshal(kubeadmTemplateFile, kubeadmTemplate)
			Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal kubeadm template")

			By("validating the kubeadm template")
			_, err = validator.ValidateCreate(ctx, kubeadmTemplate)
			Expect(err).To(BeNil(), "Expected kubeadm template to be valid")

			By("reading the k3s template from file")
			k3sTemplate := &clusterv1alpha1.ClusterTemplate{}
			k3sTemplateFile, err := os.ReadFile("../../../examples/cluster_v1alpha1_clustertemplate_k3s.yaml")
			Expect(err).NotTo(HaveOccurred(), "Failed to read k3s template file")
			err = yaml.Unmarshal(k3sTemplateFile, k3sTemplate)
			Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal k3s template")

			By("validating the k3s template")
			_, err = validator.ValidateCreate(ctx, k3sTemplate)
			Expect(err).To(BeNil(), "Expected k3s template to be valid")
		})

		It("Should only allow deletion of ClusterTemplates not in use", func() {})

	})
})
