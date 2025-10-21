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
	v1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
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
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	dockerProvider "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"k8s.io/client-go/dynamic/dynamicinformer"
    "k8s.io/client-go/tools/cache"
	k8sLabels "k8s.io/apimachinery/pkg/labels"
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

type Client interface {
    StartInformers(ctx context.Context, resources []schema.GroupVersionResource) error
    GetCached(ctx context.Context, resourceSchema schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
    ListCached(ctx context.Context, resourceSchema schema.GroupVersionResource, namespace string, listOptions metav1.ListOptions) (*unstructured.UnstructuredList, error)
    CreateNamespace(ctx context.Context, name string) error
    DeleteNamespace(ctx context.Context, namespace string) error
    CreateCluster(ctx context.Context, namespace string, cluster capi.Cluster) (string, error)
    DeleteClusters(ctx context.Context, namespace string) error
    GetCluster(ctx context.Context, namespace, name string) (*capi.Cluster, error)
    GetMachines(ctx context.Context, namespace, clusterName string) ([]capi.Machine, error)
    CreateMachineBinding(ctx context.Context, namespace string, binding intelProvider.IntelMachineBinding) error
    IntelMachines(ctx context.Context, namespace, clusterName string) ([]intelProvider.IntelMachine, error)
    DockerMachines(ctx context.Context, namespace, clusterName string) ([]dockerProvider.DockerMachine, error)
    IntelMachine(ctx context.Context, namespace, providerMachineName string) (intelProvider.IntelMachine, error)
    DockerMachine(ctx context.Context, namespace, providerMachineName string) (dockerProvider.DockerMachine, error)
	GetMachineByHostID(ctx context.Context, namespace, hostID string) (capi.Machine, error)
	DeleteCluster(ctx context.Context, namespace string, clusterName string) error
	SetMachineLabels(ctx context.Context, namespace string, machineName string, newUserLabels map[string]string) error
	WithInClusterConfig() *ManagerClient
	CreateSecret(ctx context.Context, namespace string, name string, data map[string][]byte) error
	RemoveTemplateLabels(ctx context.Context, namespace string, templateName string, labelKeys ...string) error
	AddTemplateLabels(ctx context.Context, namespace string, templateName string, newLabels map[string]string) error
	HasTemplate(ctx context.Context, namespace, templateName string) bool
	GetClusterTemplate(ctx context.Context, namespace, templateName string) (*v1alpha1.ClusterTemplate, error)
	Templates(ctx context.Context, namespace string) ([]v1alpha1.ClusterTemplate, error)
	DeleteTemplates(ctx context.Context, namespace string) error
    DefaultTemplate(ctx context.Context, namespace string) (v1alpha1.ClusterTemplate, error)
	CreateTemplate(ctx context.Context, namespace string, template *v1alpha1.ClusterTemplate) error
	Template(ctx context.Context, namespace, name string) (v1alpha1.ClusterTemplate, error)
	Dynamic() dynamic.Interface
	SetClusterLabels(ctx context.Context, namespace string, clusterName string, newUserLabels map[string]string) error
}


type ManagerClient struct {
    Dyn       dynamic.Interface
    Informers dynamicinformer.DynamicSharedInformerFactory
}

// New creates a new Client with optional configurations.
func New(opts ...func(*ManagerClient)) *ManagerClient {
    client := &ManagerClient{}
    for _, opt := range opts {
        opt(client)
    }
	if client.Dyn == nil {
		return client
	}
    // initialize the dynamic informer factory
    client.Informers = dynamicinformer.NewDynamicSharedInformerFactory(client.Dyn, 10*time.Minute)

    // start informers (docker machines not in use yet)
    if err := client.StartInformers(context.Background(), []schema.GroupVersionResource{
        clusterResourceSchema,
        templateResourceSchema,
		bindingsResourceSchema,
		machineResourceSchema,
		IntelMachineResourceSchema,
    }); err != nil {
		slog.Warn("failed to start informers. This may result in cache errors", "error", err)
        return nil
    }

    return client
}

// Dynamic allows access to the dynamic client for write operations
func (cli *ManagerClient) Dynamic() dynamic.Interface {
	return cli.Dyn
}
// StartInformers starts all informers and waits for their caches to sync.
func (cli *ManagerClient) StartInformers(ctx context.Context, resources []schema.GroupVersionResource) error {
    slog.Info("starting informers")
	if cli.Dyn == nil {
		return fmt.Errorf("dynamic client is not initialized")
	}
    // create informers for the specified resources
    syncFuncs := []cache.InformerSynced{}
    for _, resource := range resources {
		slog.Info("starting informer for resource", "resource", resource)
        informer := cli.Informers.ForResource(resource).Informer()
        go informer.Run(ctx.Done())
        syncFuncs = append(syncFuncs, informer.HasSynced)
    }

    // wait for all caches to sync
    if !cache.WaitForCacheSync(ctx.Done(), syncFuncs...) {
        return fmt.Errorf("failed to sync caches")
    }
    slog.Info("informers started and caches synced")
    return nil
}

// GetCached retrieves an object from the informer cache or falls back to the API if not found.
func (cli *ManagerClient) GetCached(ctx context.Context, resourceSchema schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
    informer := cli.Informers.ForResource(resourceSchema).Informer()

    // wait for the cache to sync before accessing it
    if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
        return nil, fmt.Errorf("cache not synced for resource: %v", resourceSchema.Resource)
    }

    // attempt to retrieve the object from the cache
    key := name
    if namespace != "" {
        key = fmt.Sprintf("%s/%s", namespace, name)
    }
    obj, exists, err := informer.GetStore().GetByKey(key)
    if err != nil {
        return nil, fmt.Errorf("error retrieving object from cache: %w", err)
    }
    if !exists {
        slog.Info("cache miss, falling back to API", "key", key)
        return cli.Dyn.Resource(resourceSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
    }

    return obj.(*unstructured.Unstructured), nil
}

func (cli *ManagerClient) ListCached(ctx context.Context, resourceSchema schema.GroupVersionResource, namespace string, listOptions metav1.ListOptions) (*unstructured.UnstructuredList, error) {
    informer := cli.Informers.ForResource(resourceSchema).Informer()

    // Wait for the cache to sync before accessing it
    if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
        return nil, fmt.Errorf("cache not synced for resource: %v", resourceSchema.Resource)
    }

    // Attempt to retrieve the objects from the cache
    var filteredObjects []unstructured.Unstructured
    items := informer.GetStore().List()
    for _, item := range items {
        obj := item.(*unstructured.Unstructured)

        // Filter by namespace if provided
        if namespace != "" && obj.GetNamespace() != namespace {
            continue
        }

        // Apply label selector filtering
        if listOptions.LabelSelector != "" {
			parsedSelector, parseErr := metav1.ParseToLabelSelector(listOptions.LabelSelector)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid label selector: %w", parseErr)
			}
			selector, err := metav1.LabelSelectorAsSelector(parsedSelector)
			if err != nil {
				return nil, fmt.Errorf("failed to convert label selector: %w", err)
			}
            if err != nil {
                return nil, fmt.Errorf("invalid label selector: %w", err)
            }
            if !selector.Matches(k8sLabels.Set(obj.GetLabels())) {
                continue
            }
        }

        filteredObjects = append(filteredObjects, *obj)
    }

    // If no objects are found in the cache, fall back to the API
    if len(filteredObjects) == 0 {
        slog.Info("cache miss, falling back to API", "resource", resourceSchema.Resource, "namespace", namespace)
        unstructuredList, err := cli.Dyn.Resource(resourceSchema).Namespace(namespace).List(ctx, listOptions)
        if err != nil {
            return nil, fmt.Errorf("error retrieving objects from API: %w", err)
        }
        return unstructuredList, nil
    }

    // Convert the filtered objects into an UnstructuredList
    unstructuredList := &unstructured.UnstructuredList{
        Items: filteredObjects,
    }
    return unstructuredList, nil
}

func (c *ManagerClient) WithInClusterConfig() *ManagerClient {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("failed to get in-cluster config", "error", err)
		return nil
	}

	qpsValue, burstValue, err := getRateLimiterParams()
	if err != nil {
		slog.Warn("unable to get rate limiter params; using default values", "error", err)
	}
	slog.Debug("rate limiter params", "qps", qpsValue, "burst", burstValue)
	cfg.QPS = float32(qpsValue)
	cfg.Burst = int(burstValue)

	c.Dyn, err = dynamic.NewForConfig(cfg)
	if err != nil {
		slog.Error("failed to create dynamic client", "error", err)
		return nil
	}
	return c
}

func (c *ManagerClient) WithFakeClient() *ManagerClient {
	c.Dyn = fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "edge-orchestrator.intel.com", Version: "v1alpha1", Resource: "clustertemplates"}: "ClusterTemplateList",
		})
	return c
}

// CreateNamespace creates a new namespace with the given name
func (c *ManagerClient) CreateNamespace(ctx context.Context, name string) error {
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

	_, err = c.Dyn.Resource(namespaceRes).Create(ctx, &namespace, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (c *ManagerClient) CreateSecret(ctx context.Context, namespace string, name string, data map[string][]byte) error {
	secretRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
		Type: v1.SecretTypeOpaque,
	}

	secretObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	if err != nil {
		return fmt.Errorf("failed to convert secret to unstructured object: %w", err)
	}

	secretManifest := &unstructured.Unstructured{Object: secretObject}
	_, err = c.Dyn.Resource(secretRes).Namespace(namespace).Create(ctx, secretManifest, metav1.CreateOptions{})

	if err != nil && !errors.IsAlreadyExists(err) {
		slog.Error("failed to create secret", "namespace", namespace, "name", name, "error", err)
		return fmt.Errorf("failed to create secret %s in namespace %s: %w", name, namespace, err)
	}

	return nil
}

// CreateTemplate creates a new template object in the given namespace
func (cli *ManagerClient) CreateTemplate(ctx context.Context, namespace string, template *v1alpha1.ClusterTemplate) error {
	templateObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&template)
	if err != nil {
		return fmt.Errorf("failed to convert templete to unstructured object: %w", err)
	}

	templateManifest := &unstructured.Unstructured{Object: templateObject}
	_, err = cli.Dyn.Resource(templateResourceSchema).Namespace(namespace).Create(ctx, templateManifest, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// CreateCluster creates a new cluster object in the given namespace
func (c *ManagerClient) CreateCluster(ctx context.Context, namespace string, cluster capi.Cluster) (string, error) {
	unstructuredCluster, err := convert.ToUnstructured(cluster)
	if err != nil {
		return "", err
	}

	slog.Debug("creating cluster", "namespace", namespace, "cluster", unstructuredCluster)

	clusterCreationResponse, err := c.Dyn.Resource(clusterResourceSchema).Namespace(namespace).Create(ctx, unstructuredCluster, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return clusterCreationResponse.GetName(), nil
}

// DeleteClusters deletes all clusters in the given namespace
func (c *ManagerClient) DeleteClusters(ctx context.Context, namespace string) error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return c.Dyn.Resource(clusterResourceSchema).Namespace(namespace).DeleteCollection(ctx, deleteOptions, metav1.ListOptions{})
}

// DeleteCluster deletes a cluster with the given name in the given namespace
func (c *ManagerClient) DeleteCluster(ctx context.Context, namespace string, clusterName string) error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return c.Dyn.Resource(clusterResourceSchema).Namespace(namespace).Delete(ctx, clusterName, deleteOptions)
}

// DeleteTemplates deletes all templates in the given namespace
func (c *ManagerClient) DeleteTemplates(ctx context.Context, namespace string) error {
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return c.Dyn.Resource(templateResourceSchema).Namespace(namespace).DeleteCollection(ctx, deleteOptions, metav1.ListOptions{})
}

// DeleteNamespace deletes the namespace with the given name
func (c *ManagerClient) DeleteNamespace(ctx context.Context, namespace string) error {
	namespaceRes := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}

	return c.Dyn.Resource(namespaceRes).Delete(ctx, namespace, deleteOptions)
}

// SetClusterLabels overrides the labels of the cluster object in the given namespace
func (c *ManagerClient) SetClusterLabels(ctx context.Context, namespace string, clusterName string, newUserLabels map[string]string) error {
	if newUserLabels == nil {
		return nil
	}

	return modifyLabels(ctx, c, namespace, clusterResourceSchema, clusterName, func(cluster *unstructured.Unstructured) {
		cluster.SetLabels(labels.Merge(labels.SystemLabels(cluster.GetLabels()), newUserLabels))
	})
}

// SetMachineLabels overrides the labels of the machine object in the given namespace
func (c *ManagerClient) SetMachineLabels(ctx context.Context, namespace string, machineName string, newUserLabels map[string]string) error {
	if newUserLabels == nil {
		return nil
	}
	return modifyLabels(ctx, c, namespace, machineResourceSchema, machineName, func(machine *unstructured.Unstructured) {
		machine.SetLabels(labels.Merge(labels.SystemLabels(machine.GetLabels()), newUserLabels))
	})
}

// AddTemplateLabels appends new labels on the template object in the given namespace
func (c *ManagerClient) AddTemplateLabels(ctx context.Context, namespace string, templateName string, newLabels map[string]string) error {
	if newLabels == nil {
		return nil
	}

	return modifyLabels(ctx, c, namespace, templateResourceSchema, templateName, func(template *unstructured.Unstructured) {
		template.SetLabels(labels.Merge(template.GetLabels(), newLabels))
	})
}

func (c *ManagerClient) RemoveTemplateLabels(ctx context.Context, namespace string, templateName string, labelKeys ...string) error {
	if len(labelKeys) == 0 {
		return nil
	}

	return modifyLabels(ctx, c, namespace, templateResourceSchema, templateName, func(template *unstructured.Unstructured) {
		template.SetLabels(labels.Remove(template.GetLabels(), labelKeys...))
	})
}

// modifyLabels modifies the labels of the given resource in the given namespace
// It retries on transient "the object has been modified" error, which is expected when the cluster object was updated by another process after we fetched it
// It returns an error if the operation fails after all retries
func modifyLabels(ctx context.Context, c *ManagerClient, namespace string, resourceSchema schema.GroupVersionResource, resourceName string, op func(*unstructured.Unstructured)) error {
	transientError := func(err error) bool {
		tryAgainErrPattern := "the object has been modified; please apply your changes to the latest version and try again"
		return strings.Contains(err.Error(), tryAgainErrPattern)
	}

	transaction := func() error {
		resource, err := c.Dyn.Resource(resourceSchema).Namespace(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return backoff.Permanent(err)
		}
		op(resource)
		if _, err = c.Dyn.Resource(resourceSchema).Namespace(namespace).Update(ctx, resource, metav1.UpdateOptions{}); err != nil {
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
func (c *ManagerClient) DefaultTemplate(ctx context.Context, namespace string) (v1alpha1.ClusterTemplate, error) {
	var template v1alpha1.ClusterTemplate

	listOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%v", labels.DefaultLabelKey, labels.DefaultLabelVal)}
	unstructuredClusterTemplatesList, err := c.Dyn.Resource(templateResourceSchema).Namespace(namespace).List(ctx, listOptions)
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
func (c *ManagerClient) Templates(ctx context.Context, namespace string) ([]v1alpha1.ClusterTemplate, error) {
	var templates []v1alpha1.ClusterTemplate

	unstructuredClusterTemplatesList, err := c.Dyn.Resource(templateResourceSchema).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return templates, err
	}

	for _, item := range unstructuredClusterTemplatesList.Items {
		var template v1alpha1.ClusterTemplate
		err = convert.FromUnstructured(item, &template)
		if err != nil {
			return templates, err
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// Template returns the template with the given name in the given namespace
func (c *ManagerClient) Template(ctx context.Context, namespace, name string) (v1alpha1.ClusterTemplate, error) {
	var template v1alpha1.ClusterTemplate

	unstructuredTemplate, err := c.Dyn.Resource(templateResourceSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
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
func (c *ManagerClient) GetCluster(ctx context.Context, namespace, name string) (*capi.Cluster, error) {
	var cluster capi.Cluster

	unstructuredCluster, err := c.Dyn.Resource(clusterResourceSchema).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
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

// GetMachineByHostID returns the machine with the given host ID in the given namespace for the given cluster
func (c *ManagerClient) GetMachineByHostID(ctx context.Context, namespace, hostID string) (capi.Machine, error) {
	opts := metav1.ListOptions{}
	unstructuredMachinesList, err := c.Dyn.Resource(machineResourceSchema).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return capi.Machine{}, err
	}

	for _, item := range unstructuredMachinesList.Items {
		var machine capi.Machine
		err = convert.FromUnstructured(item, &machine)
		if err != nil {
			continue
		}

		if machine.Status.NodeRef != nil && (machine.Status.NodeRef.Name == hostID) {
			return machine, nil
		}
	}
	return capi.Machine{}, fmt.Errorf("machine with host ID %s not found", hostID)
}

// GetMachines returns the machine with the given name in the given namespace for the given cluster
func (c *ManagerClient) GetMachines(ctx context.Context, namespace, clusterName string) ([]capi.Machine, error) {
	var machines []capi.Machine

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%v", clusterName)}
	unstructuredMachinesList, err := c.Dyn.Resource(machineResourceSchema).Namespace(namespace).List(ctx, opts)
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

func (c *ManagerClient) GetClusterTemplate(ctx context.Context, namespace, templateName string) (*v1alpha1.ClusterTemplate, error) {
	var template v1alpha1.ClusterTemplate

	unstructuredClusterTemplate, err := c.Dyn.Resource(templateResourceSchema).Namespace(namespace).Get(ctx, templateName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	err = convert.FromUnstructured(*unstructuredClusterTemplate, &template)
	if err != nil {
		return nil, err
	}

	return &template, nil
}

func (c *ManagerClient) HasTemplate(ctx context.Context, namespace, templateName string) bool {
	_, err := c.Dyn.Resource(templateResourceSchema).Namespace(namespace).Get(ctx, templateName, metav1.GetOptions{})
	return err == nil
}

// CreateMachineBinding creates a new machine binding object in the given namespace
func (c *ManagerClient) CreateMachineBinding(ctx context.Context, namespace string, binding intelProvider.IntelMachineBinding) error {
	unstructuredBinding, err := convert.ToUnstructured(binding)
	if err != nil {
		return err
	}

	_, err = c.Dyn.Resource(bindingsResourceSchema).Namespace(namespace).Create(ctx, unstructuredBinding, metav1.CreateOptions{})
	return err
}

// IntelMachines returns all IntelMachine objects in the given namespace for the given cluster
func (c *ManagerClient) IntelMachines(ctx context.Context, namespace, clusterName string) ([]intelProvider.IntelMachine, error) {
	return providerMachines[intelProvider.IntelMachine](ctx, c, namespace, clusterName, IntelMachineResourceSchema)
}

// DockerMachines returns all DockerMachine objects in the given namespace for the given cluster
func (c *ManagerClient) DockerMachines(ctx context.Context, namespace, clusterName string) ([]dockerProvider.DockerMachine, error) {
	return providerMachines[dockerProvider.DockerMachine](ctx, c, namespace, clusterName, DockerMachineResourceSchema)
}

// providerMachines returns the provider machines in the given namespace for the given cluster
func providerMachines[T any](ctx context.Context, cli *ManagerClient, namespace, clusterName string, providerSchema schema.GroupVersionResource) ([]T, error) {
	var machines []T

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("cluster.x-k8s.io/cluster-name=%v", clusterName)}
	unstructuredMachinesList, err := cli.ListCached(ctx, providerSchema, namespace, opts)
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
func (c *ManagerClient) IntelMachine(ctx context.Context, namespace, providerMachineName string) (intelProvider.IntelMachine, error) {
	return providerMachine[intelProvider.IntelMachine](ctx, c, namespace, providerMachineName, IntelMachineResourceSchema)
}

// DockerMachine returns the DockerMachine with the given name in the given namespace for the given cluster
func (c *ManagerClient) DockerMachine(ctx context.Context, namespace, providerMachineName string) (dockerProvider.DockerMachine, error) {
	return providerMachine[dockerProvider.DockerMachine](ctx, c, namespace, providerMachineName, DockerMachineResourceSchema)
}

// providerMachine returns the provider machine with the given name in the given namespace for the given cluster
func providerMachine[T any](ctx context.Context, c *ManagerClient, namespace, providerMachineName string, providerSchema schema.GroupVersionResource) (T, error) {
	var machine T

	unstructuredMachine, err := c.Dyn.Resource(providerSchema).Namespace(namespace).Get(ctx, providerMachineName, metav1.GetOptions{})
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
