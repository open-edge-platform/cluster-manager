// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bufio"
	"context"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	rke2bootstrapv1beta1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	rke2cpv1beta1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmbootstrapv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kubeadmcp "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
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

// getModuleVersionFromGoMod parses go.mod to get module version
func getModuleVersionFromGoMod(modulePath string) string {
	goModPath := filepath.Join("..", "..", "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		panic(fmt.Sprintf("failed to open go.mod: %v", err))
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, modulePath) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				// Return the version (last field)
				return fields[len(fields)-1]
			}
		}
	}

	if err := scanner.Err(); err != nil {
		panic(fmt.Sprintf("error reading go.mod: %v", err))
	}

	panic(fmt.Sprintf("module %s not found in go.mod", modulePath))
}

// buildCRDPaths dynamically builds CRD paths using versions from go.mod
func buildCRDPaths() []string {
	capiVersion := getModuleVersionFromGoMod("sigs.k8s.io/cluster-api")
	intelVersion := getModuleVersionFromGoMod("github.com/open-edge-platform/cluster-api-provider-intel")

	modPath := filepath.Join(build.Default.GOPATH, "pkg", "mod")

	paths := []string{
		filepath.Join("..", "..", "config", "crd", "bases"),
		// TODO: usage of rke2 in tests to be removed
		filepath.Join(modPath, "github.com", "rancher", "cluster-api-provider-rke2@v0.21.0", "controlplane", "config", "crd", "bases"),
		filepath.Join(modPath, "sigs.k8s.io", "cluster-api@"+capiVersion, "controlplane", "kubeadm", "config", "crd", "bases"),
		filepath.Join(modPath, "sigs.k8s.io", "cluster-api@"+capiVersion, "config", "crd", "bases"),
		// note: cluster-api/test is a separate module with different path structure
		filepath.Join(modPath, "sigs.k8s.io", "cluster-api", "test@"+capiVersion, "infrastructure", "docker", "config", "crd", "bases"),
		filepath.Join(modPath, "github.com", "open-edge-platform", "cluster-api-provider-intel@"+intelVersion, "config", "crd", "bases"),
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

	// ---- RKE2 CONTROL PLANE PROVIDER ----
	// Add scheme for RKE2 bootstrap provider
	err = rke2bootstrapv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Add scheme for RKE2 control plane provider
	err = rke2cpv1beta1.AddToScheme(scheme.Scheme)
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
