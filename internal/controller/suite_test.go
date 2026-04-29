// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	// +kubebuilder:scaffold:imports

	// Imports for CAPI resources
	kthreesbootstrapv1beta2 "github.com/k3s-io/cluster-api-k3s/bootstrap/api/v1beta2"
	kthreescpv1beta2 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	kubeadmbootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	kubeadmcp "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta1"
	dockerv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

type moduleDownload struct {
	Dir string `json:"Dir"`
}

func getModuleDir(modulePath string) string {
	cmd := exec.Command("go", "mod", "download", "-json", modulePath)
	cmd.Dir = filepath.Join("..", "..")

	output, err := cmd.Output()
	if err != nil {
		panic(fmt.Sprintf("failed to resolve module %s: %v", modulePath, err))
	}

	var download moduleDownload
	if err := json.Unmarshal(output, &download); err != nil {
		panic(fmt.Sprintf("failed to parse module metadata for %s: %v", modulePath, err))
	}
	if download.Dir == "" {
		panic(fmt.Sprintf("module %s resolved without a directory", modulePath))
	}

	return download.Dir
}

func buildCRDPaths() []string {
	capiDir := getModuleDir("sigs.k8s.io/cluster-api")
	capiTestDir := getModuleDir("sigs.k8s.io/cluster-api/test")
	intelDir := getModuleDir("github.com/open-edge-platform/cluster-api-provider-intel")
	k3sDir := getModuleDir("github.com/k3s-io/cluster-api-k3s")

	paths := []string{
		filepath.Join("..", "..", "config", "crd", "bases"),
		filepath.Join(capiDir, "controlplane", "kubeadm", "config", "crd", "bases"),
		filepath.Join(capiDir, "config", "crd", "bases"),
		filepath.Join(capiTestDir, "infrastructure", "docker", "config", "crd", "bases"),
		filepath.Join(intelDir, "config", "crd", "bases"),
		// K3s control plane and bootstrap provider CRDs
		filepath.Join(k3sDir, "controlplane", "config", "crd", "bases"),
		filepath.Join(k3sDir, "bootstrap", "config", "crd", "bases"),
	}

	return paths
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error

	err = clusterv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// ---- DOCKER INFRASTRUCTURE PROVIDER  ----
	// Add scheme for Docker infrastructure provider
	err = dockerv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// ---- INTEL INFRASTRUCTURE PROVIDER  ----
	// Add scheme for Intel infrastructure provider
	err = intelv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// ---- KUBEADM CONTROL PLANE PROVIDER ----
	// Add scheme for Kubeadm bootstrap provider
	err = kubeadmbootstrapv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Add scheme for Kubeadm control plane provider
	err = kubeadmcp.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// ---- K3S CONTROL PLANE PROVIDER ----
	// Add scheme for K3s bootstrap provider
	err = kthreesbootstrapv1beta2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Add scheme for K3s control plane provider
	err = kthreescpv1beta2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// ----  CAPI ----
	// Add scheme for Cluster API core resources
	err = capi.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")

	crdPaths := buildCRDPaths()
	fmt.Fprintf(GinkgoWriter, "CRD paths:\n")
	for i, path := range crdPaths {
		fmt.Fprintf(GinkgoWriter, "  [%d] %s\n", i, path)
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     crdPaths,
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
