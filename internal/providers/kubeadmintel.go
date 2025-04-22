package controlplaneprovider

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kubeadmintel struct{}

// AlterClusterClass implements Provider.
func (k kubeadmintel) AlterClusterClass(cc *v1beta1.ClusterClass) {
}

// CreateClusterTemplate implements Provider.
func (k kubeadmintel) CreateClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// CreateControlPlaneMachineTemplate implements Provider.
func (k kubeadmintel) CreateControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// CreateControlPlaneTemplate implements Provider.
func (k kubeadmintel) CreateControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName, config string) error {
	return errors.New("not implemented")
}

// CreatePrerequisites implements Provider.
func (k kubeadmintel) CreatePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// DeletePrerequisites implements Provider.
func (k kubeadmintel) DeletePrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// GetClusterTemplate implements Provider.
func (k kubeadmintel) GetClusterTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// GetControlPlaneMachineTemplate implements Provider.
func (k kubeadmintel) GetControlPlaneMachineTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// GetControlPlaneTemplate implements Provider.
func (k kubeadmintel) GetControlPlaneTemplate(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}

// GetPrerequisites implements Provider.
func (k kubeadmintel) GetPrerequisites(ctx context.Context, c client.Client, name types.NamespacedName) error {
	return errors.New("not implemented")
}
