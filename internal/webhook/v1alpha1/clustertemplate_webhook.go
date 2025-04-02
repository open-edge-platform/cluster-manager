// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	rke2cpv1beta1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	kubeadmcp "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// set up logging
var clustertemplatelog = logf.Log.WithName("clustertemplate-resource")

// SetupClusterTemplateWebhookWithManager registers the webhook for ClusterTemplate in the manager.
func (v *ClusterTemplateCustomValidator) SetupClusterTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&clusterv1alpha1.ClusterTemplate{}).
		WithValidator(v).
		Complete()
}

type ClusterTemplateCustomValidator struct {
	client.Client
}

var _ webhook.CustomValidator = &ClusterTemplateCustomValidator{}

// ValidateCreate validates a cluster template creation request
func (v *ClusterTemplateCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	clustertemplate, ok := obj.(*clusterv1alpha1.ClusterTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTemplate object but got %T", obj)
	}
	clustertemplatelog.Info("validation for ClusterTemplate upon creation", "name", clustertemplate.GetName())

	switch api.TemplateInfoControlplaneprovidertype(clustertemplate.Spec.ControlPlaneProviderType) {
	case api.Kubeadm:
		kubeadmControlPlaneTemplate := &kubeadmcp.KubeadmControlPlaneTemplate{}
		err := json.Unmarshal([]byte(clustertemplate.Spec.ClusterConfiguration), &kubeadmControlPlaneTemplate)
		if err != nil {
			slog.Error("invalid KubeadmControlPlaneTemplate", "error", err)
			return nil, fmt.Errorf("failed to convert cluster configuration: %w", err)
		}
	case api.Rke2:
		rke2ControlPlaneTemplate := &rke2cpv1beta1.RKE2ControlPlaneTemplate{}
		err := json.Unmarshal([]byte(clustertemplate.Spec.ClusterConfiguration), &rke2ControlPlaneTemplate)
		if err != nil {
			slog.Error("invalid RKE2ControlPlaneTemplate", "error", err)
			return nil, fmt.Errorf("failed to convert cluster configuration: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid control plane provider type: %s", clustertemplate.Spec.ControlPlaneProviderType)
	}
	return nil, nil
}

// ValidateUpdate validates modifications to ClusterTemplate
func (v *ClusterTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// read in both objects
	newTemplate, ok := newObj.(*clusterv1alpha1.ClusterTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTemplate object for the newObj but got %T", newObj)
	}
	oldTemplate, ok := oldObj.(*clusterv1alpha1.ClusterTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTemplate object for the oldObj but got %T", oldObj)
	}
	// ideally we'd just reject any updates to the ClusterTemplate but the controller makes updates to status etc
	// there doesn't seem to be any way to differentiate requests even through a passed context key
	if !reflect.DeepEqual(newTemplate.Spec, oldTemplate.Spec) {
		return nil, fmt.Errorf("clusterTemplate spec immutable")
	}
	clustertemplatelog.Info("validation for ClusterTemplate upon update", "name", newTemplate.GetName())
	return nil, nil

}

// ValidateDelete validates ClusterTemplate deletion request by checking if the template is in use
func (v *ClusterTemplateCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	clustertemplate, ok := obj.(*clusterv1alpha1.ClusterTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterTemplate object but got %T", obj)
	}
	clustertemplatelog.Info("validation for ClusterTemplate upon deletion", "name", clustertemplate.GetName())

	err := v.templateNotInUse(ctx, clustertemplate)

	return nil, err
}

// templateInUse checks if the ClusterTemplate is in use by any Cluster
func (v *ClusterTemplateCustomValidator) templateNotInUse(ctx context.Context, template *clusterv1alpha1.ClusterTemplate) error {
	// get the ClusterClass object
	clusterClassName := template.Name
	if template.Status.ClusterClassRef != nil {
		clusterClassName = template.Status.ClusterClassRef.Name
	}
	clusterClass := &capi.ClusterClass{}
	err := v.Client.Get(ctx, client.ObjectKey{
		Namespace: template.Namespace,
		Name:      clusterClassName,
	}, clusterClass)
	if err != nil {
		return fmt.Errorf("failed to get ClusterClass: %w", err)
	}

	// get clusters that reference the ClusterClass
	clusters := &capi.ClusterList{}
	err = v.Client.List(ctx, clusters,
		client.MatchingFields{
			"spec.topology.classRef": fmt.Sprintf("%s/%s", clusterClass.Namespace, clusterClass.Name),
		},
	)
	if err != nil {
		clustertemplatelog.Error(err, "failed to list clusters. treating template as in use")
		return fmt.Errorf("failed to list clusters: %w", err)
	}

	if len(clusters.Items) > 0 {
		return fmt.Errorf("clusterTemplate is in use")
	}
	return nil
}

// IndexClusterInstances returns the list of Cluster instances that reference the ClusterTemplate
func IndexClusterInstances(obj client.Object) []string {
	cluster, ok := obj.(*capi.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected Cluster but got a %T", obj))
	}
	if cluster.Spec.Topology != nil {
		key := cluster.GetClassKey()
		return []string{fmt.Sprintf("%s/%s", key.Namespace, key.Name)}
	}
	return nil
}
