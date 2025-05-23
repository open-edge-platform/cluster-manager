// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/controller"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	webhookclusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/internal/webhook/v1alpha1"

	// +kubebuilder:scaffold:imports

	// Imports for CAPI resources
	intelv1alpha1 "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	rke2bootstrapv1beta1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	rke2cpv1beta1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmbootstrapv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kubeadmcp "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	kthreesbootstrapv1beta2 "github.com/k3s-io/cluster-api-k3s/bootstrap/api/v1beta2"
	kthreescpv1beta2 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	dockerv1beta1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(clusterv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	// ---- DOCKER INFRASTRUCTURE PROVIDER  ----
	// Add scheme for Docker infrastructure provider
	utilruntime.Must(dockerv1beta1.AddToScheme(scheme))

	// ---- INTEL INFRASTRUCTURE PROVIDER  ----
	// Add scheme for Intel infrastructure provider
	utilruntime.Must(intelv1alpha1.AddToScheme(scheme))

	// ---- KUBEADM CONTROL PLANE PROVIDER ----
	// Add scheme for Kubeadm bootstrap provider
	utilruntime.Must(kubeadmbootstrapv1beta1.AddToScheme(scheme))

	// Add scheme for Kubeadm control plane provider
	utilruntime.Must(kubeadmcp.AddToScheme(scheme))

	// ---- RKE2 CONTROL PLANE PROVIDER ----
	// Add scheme for RKE2 bootstrap provider
	utilruntime.Must(rke2bootstrapv1beta1.AddToScheme(scheme))

	// Add scheme for RKE2 control plane provider
	utilruntime.Must(rke2cpv1beta1.AddToScheme(scheme))

	// ---- K3s CONTROL PLANE PROVIDER ----
	// Add scheme for K3s bootstrap provider
	utilruntime.Must(kthreesbootstrapv1beta2.AddToScheme(scheme))

	// Add scheme for K3s control plane provider
	utilruntime.Must(kthreescpv1beta2.AddToScheme(scheme))
	// ----  CAPI ----
	// Add scheme for Cluster API core resources
	utilruntime.Must(capi.AddToScheme(scheme))
}

// version injected at build time
var version string

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var webhookCertPath string
	var enableWebhook bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.BoolVar(&enableWebhook, "webhook-enabled", false,
		"enables validating webhook for the cluster template")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
		Port:    9443,
	})

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization

		// TODO(user): If CertDir, CertName, and KeyName are not specified, controller-runtime will automatically
		// generate self-signed certificates for the metrics server. While convenient for development and testing,
		// this setup is not recommended for production.
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5f4b93b4.edge-orchestrator.intel.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager", "version", version)
		os.Exit(1)
	}

	if err = (&controller.ClusterTemplateReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterTemplate")
		os.Exit(1)
	}
	if enableWebhook {
		setupLog.Info("enabling webhook for ClusterTemplate")
		if err := (&webhookclusterv1alpha1.ClusterTemplateCustomValidator{Client: mgr.GetClient()}).SetupClusterTemplateWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Unable to create webhook", "webhook", "ClusterClass")
			os.Exit(1)
		}
	}
	// creates an index to match clusters with a specific cluster class
	if err := mgr.GetCache().IndexField(context.Background(), &capi.Cluster{},
		core.ClusterInstances,
		webhookclusterv1alpha1.IndexClusterInstances,
	); err != nil {
		setupLog.Error(err, "unable to create index", "index", "ClusterByClusterClassRef")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	setupLog.Info("starting manager", "version", version)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
