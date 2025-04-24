// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	backoff "github.com/cenkalti/backoff/v4"
	intelProvider "github.com/open-edge-platform/cluster-api-provider-intel/api/v1alpha1"
	ct "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/config"
	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	dockerProvider "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
)

const (
	// certain errors are expected to be transient and we should retry the operation (e.g. when the cluster object has been modified)
	retryInterval    = 250 * time.Millisecond // retryInterval is the time to wait between retries
	maxRetries       = 12                     // maxRetries is the maximum number of retries
	rateLimiterQPS   = "RATE_LIMITER_QPS"
	rateLimiterBurst = "RATE_LIMITER_BURST"
	defaultQPS       = 30
	defaultBurst     = 100
)

var ErrDefaultTemplateNotFound = fmt.Errorf("default template not found")
var ErrClusterNotFound = fmt.Errorf("cluster not found")

// K8s object schemas
var (
	templateResourceGroup   = "edge-orchestrator.intel.com"
	templateResourceVersion = "v1alpha1"
	templateResourceKind    = "clustertemplates"

	clusterResourceSchema = schema.GroupVersionResource{
		Group:    "cluster.x-k8s.io",
		Version:  "v1beta1",
		Resource: "clusters",
	}
	templateResourceSchema = schema.GroupVersionResource{
		Group:    templateResourceGroup,
		Version:  templateResourceVersion,
		Resource: templateResourceKind,
	}

	bindingsResourceSchema = schema.GroupVersionResource{
		Group:    intelProvider.GroupVersion.Group,
		Version:  intelProvider.GroupVersion.Version,
		Resource: "intelmachinebindings",
	}

	machineResourceSchema = schema.GroupVersionResource{
		Group:    "cluster.x-k8s.io",
		Version:  "v1beta1",
		Resource: "machines",
	}

	IntelMachineResourceSchema = schema.GroupVersionResource{
		Group:    intelProvider.GroupVersion.Group,
		Version:  intelProvider.GroupVersion.Version,
		Resource: "intelmachines",
	}

	DockerMachineResourceSchema = schema.GroupVersionResource{
		Group:    "infrastructure.cluster.x-k8s.io",
		Version:  "v1beta1",
		Resource: "dockermachines",
	}
)

type Client struct {
	Dyn dynamic.Interface
}

func New(opts ...func(*Client)) (*Client, error) {
	client := &Client{}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

func WithInClusterConfig() func(*Client) {
	return func(cli *Client) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			panic(fmt.Errorf("failed to get user home directory: %w", err)) // unrecoverable error
		}

		kubeConfigPath := fmt.Sprintf("%s/.kube/config", homeDir)
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			panic(fmt.Errorf("failed to build config from kubeconfig: %w", err)) // unrecoverable error
		}
		// cfg, err := rest.InClusterConfig()
		// if err != nil {
		// 	panic(fmt.Errorf("failed to get in cluster config: %w", err)) // unrecoverable error
		// }

		qpsValue, burstValue, err := getRateLimiterParams()
		if err != nil {
			slog.Warn("unable to get rate limiter params; using default values", "error", err)
		}
		slog.Info("rate limiter params", "qps", qpsValue, "burst", burstValue)

		cfg.QPS = float32(qpsValue)
		cfg.Burst = int(burstValue)

		cli.Dyn, err = dynamic.NewForConfig(cfg)
		if err != nil {
			panic(fmt.Errorf("failed to create dynamic clientSet: %w", err)) // unrecoverable error
		}
	}
}

func WithDynamicClient(dyn dynamic.Interface) func(*Client) {
	return func(cli *Client) {
		cli.Dyn = dyn
	}
}

func WithFakeClient() func(*Client) {
	return func(cli *Client) {
		cli.Dyn = fake.NewSimpleDynamicClient(runtime.NewScheme())
	}
}

// NewClient is OBSOLETE and should not be used, use New() instead
// TODO: refactor multitenancy to use New() instead of NewClient()
func NewClient(cfg *config.Config) (*Client, error) {
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("failed to get cluster config", "error", err)
		return nil, err
	}

	client, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		slog.Error("failed to create dynamic clientSet", "error", err)
		return nil, err
	}

	return &Client{client}, nil
}

// NewClientFake is OBSOLETE and should not be used, use New(WitFakeClient()) instead
// TODO: refactor multitenancy to use New(WithFakeClient()) instead of NewClientFake()
func NewClientFake() *Client {
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())

	return &Client{client}
}

// CreateNamespace creates a new namespace with the given name
func (cli *Client) CreateNamespace(ctx context.Context, name string) error {
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	namespaceInfo := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	namespaceObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&namespaceInfo)
	if err != nil {
		return fmt.Errorf("failed to convert namespace to unstructured object: %w", err)
	}

	namespace := unstructured.Unstructured{Object: namespaceObject}

	_, err = cli.Dyn.Resource(namespaceRes).Create(ctx, &namespace, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// CreateTemplate creates a new template object in the given namespace
func (cli *Client) CreateTemplate(ctx context.Context, namespace string, template *ct.ClusterTemplate) error {
	templateObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&template)
	if err != nil {
		return fmt.Errorf("failed to convert templete to unstructured object: %w", err)
	}

	templateManifest := &unstructured.Unstructured{Object: templateObject}
	_, err = cli.Dyn.Resource(templateResourceSchema).Namespace(namespace).Create(ctx, templateManifest, metav1.CreateOptions{})
	return err
}

// CreateCluster creates a new cluster object in the given namespace
func (cli *Client) CreateCluster(ctx context.Context, namespace string, cluster capi.Cluster) (string, error) {
	unstructuredCluster, err := convert.ToUnstructured(cluster)
	if err != nil {
		return "", err
	}

	clusterCreationResponse, err := cli.Dyn.Resource(clusterResourceSchema).Namespace(namespace).Create(ctx, unstructuredCluster, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return clusterCreationResponse.GetName(), nil
}

// DeleteClusters deletes all clusters in the given namespace
func (cli *Client) DeleteClusters(ctx context.Context, namespace string) error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return cli.Dyn.Resource(clusterResourceSchema).Namespace(namespace).DeleteCollection(ctx, deleteOptions, metav1.ListOptions{})
}

// DeleteTemplates deletes all templates in the given namespace
func (cli *Client) DeleteTemplates(ctx context.Context, namespace string) error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return cli.Dyn.Resource(templateResourceSchema).Namespace(namespace).DeleteCollection(ctx, deleteOptions, metav1.ListOptions{})
}

// DeleteNamespace deletes the namespace with the given name
func (cli *Client) DeleteNamespace(ctx context.Context, namespace string) error {
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return cli.Dyn.Resource(namespaceRes).Delete(ctx, namespace, deleteOptions)
}

// CreateClusterLabels creates new labels on the cluster object in the given namespace
func (cli *Client) CreateClusterLabels(ctx context.Context, namespace string, clusterName string, newLabels map[string]string) error {
	if newLabels == nil {
		return nil
	}

	return createLabels(ctx, cli, namespace, clusterResourceSchema, clusterName, newLabels)
}

// CreateTemplateLabels creates new labels on the template object in the given namespace
func (cli *Client) CreateTemplateLabels(ctx context.Context, namespace string, templateName string, newLabels map[string]string) error {
	if newLabels == nil {
		return nil
	}

	return createLabels(ctx, cli, namespace, templateResourceSchema, templateName, newLabels)
}

// createLabels creates new labels on the resource object in the given namespace
// It retries on transient "the object has been modified" error, which is expected when the cluster object was updated by another process after we fetched it
// It returns an error if the operation fails after all retries
func createLabels(ctx context.Context, cli *Client, namespace string, resourceSchema schema.GroupVersionResource, resourceName string, newLabels map[string]string) error {
	transientError := func(err error) bool {
		tryAgainErrPattern := "the object has been modified; please apply your changes to the latest version and try again"
		return strings.Contains(err.Error(), tryAgainErrPattern)
	}

	transaction := func() error {
		resource, err := cli.Dyn.Resource(resourceSchema).Namespace(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return backoff.Permanent(err)
		}
		resource.SetLabels(labels.Merge(resource.GetLabels(), newLabels))
		if _, err = cli.Dyn.Resource(resourceSchema).Namespace(namespace).Update(ctx, resource, metav1.UpdateOptions{}); err != nil {
			if transientError(err) {
				return err // retry on transient error
			}
			return backoff.Permanent(err)
		}
		return nil
	}

	return backoff.Retry(transaction, backoff.WithMaxRetries(backoff.NewConstantBackOff(retryInterval), maxRetries))
}

// DefaultTemplate returns the default template in the given namespace
func (cli *Client) DefaultTemplate(ctx context.Context, namespace string) (ct.ClusterTemplate, error) {
	var template ct.ClusterTemplate

	listOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%v", labels.DefaultLabelKey, labels.DefaultLabelVal)}
	unstructuredClusterTemplatesList, err := cli.Dyn.Resource(templateResourceSchema).Namespace(namespace).List(ctx, listOptions)
	if err != nil {
		return template, err
	}
	if len(unstructuredClusterTemplatesList.Items) > 1 {
		return template, fmt.Errorf("multiple default templates found")
	}
	if len(unstructuredClusterTemplatesList.Items) == 0 {
		return template, ErrDefaultTemplateNotFound
	}
	err = convert.FromUnstructured(unstructuredClusterTemplatesList.Items[0], &template)
	if err != nil {
		return template, err
	}

	// Check if the template has the default label key equal to true
	if val, ok := template.Labels[labels.DefaultLabelKey]; !ok || val != labels.DefaultLabelVal {
		return template, ErrDefaultTemplateNotFound
	}

	return template, nil
}

// Templates returns all templates in the given namespace
func (cli *Client) Templates(ctx context.Context, namespace string) ([]ct.ClusterTemplate, error) {
	var templates []ct.ClusterTemplate

	unstructuredClusterTemplatesList, err := cli.Dyn.Resource(templateResourceSchema).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return templates, err
	}

	for _, item := range unstructuredClusterTemplatesList.Items {
		var template ct.ClusterTemplate
		err = convert.FromUnstructured(item, &template)
		if err != nil {
			return templates, err
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// Template returns the template with the given name in the given namespace
func (cli *Client) Template(ctx context.Context, namespace, name string) (ct.ClusterTemplate, error) {
	var template ct.ClusterTemplate

	unstructuredTemplate, err := cli.Dyn.Resource(templateResourceSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return template, err
	}

	err = convert.FromUnstructured(*unstructuredTemplate, &template)
	if err != nil {
		return template, err
	}

	return template, nil
}

// GetCluster returns the cluster with the given name in the given namespace
func (cli *Client) GetCluster(ctx context.Context, namespace, name string) (*capi.Cluster, error) {
	var cluster capi.Cluster

	unstructuredCluster, err := cli.Dyn.Resource(clusterResourceSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrClusterNotFound
		}
		return nil, err
	}

	err = convert.FromUnstructured(*unstructuredCluster, &cluster)
	if err != nil {
		return nil, err
	}

	return &cluster, nil
}

// GetMachines returns the machine with the given name in the given namespace for the given cluster
func (cli *Client) GetMachines(ctx context.Context, namespace, clusterName string) ([]capi.Machine, error) {
	var machines []capi.Machine

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%v", clusterName)}
	unstructuredMachinesList, err := cli.Dyn.Resource(machineResourceSchema).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return machines, err
	}

	for _, item := range unstructuredMachinesList.Items {
		var machine capi.Machine
		err = convert.FromUnstructured(item, &machine)
		if err != nil {
			return machines, err
		}
		machines = append(machines, machine)
	}

	return machines, nil
}

// CreateMachineBinding creates a new machine binding object in the given namespace
func (cli *Client) CreateMachineBinding(ctx context.Context, namespace string, binding intelProvider.IntelMachineBinding) error {
	unstructuredBinding, err := convert.ToUnstructured(binding)
	if err != nil {
		return err
	}

	_, err = cli.Dyn.Resource(bindingsResourceSchema).Namespace(namespace).Create(ctx, unstructuredBinding, metav1.CreateOptions{})
	return err
}

// IntelMachines returns all IntelMachine objects in the given namespace for the given cluster
func (cli *Client) IntelMachines(ctx context.Context, namespace, clusterName string) ([]intelProvider.IntelMachine, error) {
	return providerMachines[intelProvider.IntelMachine](ctx, cli, namespace, clusterName, IntelMachineResourceSchema)
}

// DockerMachines returns all DockerMachine objects in the given namespace for the given cluster
func (cli *Client) DockerMachines(ctx context.Context, namespace, clusterName string) ([]dockerProvider.DockerMachine, error) {
	return providerMachines[dockerProvider.DockerMachine](ctx, cli, namespace, clusterName, DockerMachineResourceSchema)
}

// providerMachines returns the provider machines in the given namespace for the given cluster
func providerMachines[T any](ctx context.Context, cli *Client, namespace, clusterName string, providerSchema schema.GroupVersionResource) ([]T, error) {
	var machines []T

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%v", clusterName)}
	unstructuredMachinesList, err := cli.Dyn.Resource(providerSchema).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return machines, err
	}

	for _, item := range unstructuredMachinesList.Items {
		var machine T
		err = convert.FromUnstructured(item, &machine)
		if err != nil {
			return machines, err
		}
		machines = append(machines, machine)
	}

	return machines, nil
}

// IntelMachine returns the IntelMachine with the given name in the given namespace for the given cluster
func (cli *Client) IntelMachine(ctx context.Context, namespace, providerMachineName string) (intelProvider.IntelMachine, error) {
	return providerMachine[intelProvider.IntelMachine](ctx, cli, namespace, providerMachineName, IntelMachineResourceSchema)
}

// DockerMachine returns the DockerMachine with the given name in the given namespace for the given cluster
func (cli *Client) DockerMachine(ctx context.Context, namespace, providerMachineName string) (dockerProvider.DockerMachine, error) {
	return providerMachine[dockerProvider.DockerMachine](ctx, cli, namespace, providerMachineName, DockerMachineResourceSchema)
}

// providerMachine returns the provider machine with the given name in the given namespace for the given cluster
func providerMachine[T any](ctx context.Context, cli *Client, namespace, providerMachineName string, providerSchema schema.GroupVersionResource) (T, error) {
	var machine T

	unstructuredMachine, err := cli.Dyn.Resource(providerSchema).Namespace(namespace).Get(ctx, providerMachineName, metav1.GetOptions{})
	if err != nil {
		return machine, err
	}

	err = convert.FromUnstructured(*unstructuredMachine, &machine)
	if err != nil {
		return machine, err
	}

	return machine, nil
}

func getRateLimiterParams() (float64, int64, error) {
	qps := os.Getenv(rateLimiterQPS)
	qpsValue, err := strconv.ParseFloat(qps, 32)
	if err != nil {
		return defaultQPS, defaultBurst, err
	}
	burst := os.Getenv(rateLimiterBurst)
	burstValue, err := strconv.ParseInt(burst, 10, 32)
	if err != nil {
		return defaultQPS, defaultBurst, err
	}
	return qpsValue, burstValue, nil
}
