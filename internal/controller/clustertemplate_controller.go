// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clustertemplatev1alpha1 "github.com/open-edge-platform/cluster-manager/v2/api/v1alpha1"
	"github.com/open-edge-platform/cluster-manager/v2/internal/common"
	capiProvider "github.com/open-edge-platform/cluster-manager/v2/internal/providers"
)

// ClusterTemplateReconciler reconciles a ClusterTemplate object
type ClusterTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=edge-orchestrator.intel.com,resources=clustertemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=edge-orchestrator.intel.com,resources=clustertemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=edge-orchestrator.intel.com,resources=clustertemplates/finalizers,verbs=update

// RBAC for CAPI components
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinetemplates,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockerclustertemplates,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=intelmachinetemplates,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=intelclustertemplates,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanetemplates,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch;create;delete

// RBAC for dependencies
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;delete

func (r *ClusterTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)

	// Fetch the ClusterTemplate instance
	// Fetch the ClusterTemplate instance
	clusterTemplate := &clustertemplatev1alpha1.ClusterTemplate{}
	err := r.Get(ctx, req.NamespacedName, clusterTemplate)
	if err != nil {
		if errors.IsNotFound(err) {
			// ignore error, the ClusterTemplate has been already deleted
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "failed to get ClusterTemplate", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, err
	}

	// Get the provider based on controlPlaneProviderType and infraProviderType
	provider := capiProvider.GetCapiProvider(clusterTemplate.Spec.ControlPlaneProviderType, clusterTemplate.Spec.InfraProviderType)
	if provider == nil {
		logger.Error(nil, "unsupported provider combination", "controlPlaneProviderType", clusterTemplate.Spec.ControlPlaneProviderType, "infraProviderType", clusterTemplate.Spec.InfraProviderType)
		return ctrl.Result{}, nil
	}

	namespacedName := types.NamespacedName{
		Name:      clusterTemplate.Name,
		Namespace: clusterTemplate.Namespace,
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(clusterTemplate, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := r.patchClusterTemplate(ctx, logger, namespacedName, patchHelper, clusterTemplate, provider); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle clusterTemplate deletion
	if !clusterTemplate.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, logger, clusterTemplate, namespacedName, provider)
	}

	// Handle non-deleted machines
	if err := r.reconcileClusterTemplate(ctx, logger, namespacedName, provider, clusterTemplate); err != nil {
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(clusterTemplate, clustertemplatev1alpha1.ClusterTemplateFinalizer) {
		logger.Info("Adding finalizer to ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		controllerutil.AddFinalizer(clusterTemplate, clustertemplatev1alpha1.ClusterTemplateFinalizer)
	}

	return ctrl.Result{}, nil
}

func (r *ClusterTemplateReconciler) reconcileClusterTemplate(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, provider capiProvider.Provider, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate) error {
	if err := r.reconcileControlPlaneTemplate(ctx, logger, namespacedName, provider, clusterTemplate); err != nil {
		return err
	}
	if err := r.reconcilePrerequisites(ctx, logger, namespacedName, provider, clusterTemplate); err != nil {
		return err
	}
	if err := r.reconcileControlPlaneMachineTemplate(ctx, logger, namespacedName, provider, clusterTemplate); err != nil {
		return err
	}
	if err := r.reconcileProviderClusterTemplate(ctx, logger, namespacedName, provider, clusterTemplate); err != nil {
		return err
	}
	if err := r.reconcileClusterClass(ctx, logger, namespacedName, provider, clusterTemplate); err != nil {
		return err
	}
	return nil
}

func (r *ClusterTemplateReconciler) reconcileControlPlaneTemplate(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, provider capiProvider.Provider, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate) error {
	err := provider.GetControlPlaneTemplate(ctx, r.Client, namespacedName)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating ControlPlaneTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		err = provider.CreateControlPlaneTemplate(ctx, r.Client, namespacedName, clusterTemplate.Spec.ClusterConfiguration)
		if err != nil {
			logger.Error(err, "failed to create ControlPlaneTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ControlPlaneTemplateCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
			return err
		}
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.ControlPlaneTemplateCondition)
	} else if err != nil {
		logger.Error(err, "failed to get ControlPlaneTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ControlPlaneTemplateCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
		return err
	} else {
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.ControlPlaneTemplateCondition)
	}
	return nil
}

func (r *ClusterTemplateReconciler) reconcilePrerequisites(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, provider capiProvider.Provider, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate) error {
	err := provider.GetPrerequisites(ctx, r.Client, namespacedName)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating prerequisites for cluster template", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		err = provider.CreatePrerequisites(ctx, r.Client, namespacedName)
		if err != nil {
			logger.Error(err, "failed to create prerequisites for cluster template", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.PrerequisitesCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
			return err
		}
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.PrerequisitesCondition)
	} else if err != nil {
		logger.Error(err, "failed to get prerequisites for cluster template", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.PrerequisitesCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
		return err
	} else {
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.PrerequisitesCondition)
	}
	return nil
}

func (r *ClusterTemplateReconciler) reconcileControlPlaneMachineTemplate(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, provider capiProvider.Provider, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate) error {
	err := provider.GetControlPlaneMachineTemplate(ctx, r.Client, namespacedName)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating ControlPlaneMachineTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		err = provider.CreateControlPlaneMachineTemplate(ctx, r.Client, namespacedName)
		if err != nil {
			logger.Error(err, "failed to create ControlPlaneMachineTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ControlPlaneMachineTemplateCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
			return err
		}
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.ControlPlaneMachineTemplateCondition)
	} else if err != nil {
		logger.Error(err, "failed to get ControlPlaneMachineTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ControlPlaneMachineTemplateCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
		return err
	} else {
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.ControlPlaneMachineTemplateCondition)
	}
	return nil
}

func (r *ClusterTemplateReconciler) reconcileProviderClusterTemplate(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, provider capiProvider.Provider, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate) error {
	err := provider.GetClusterTemplate(ctx, r.Client, namespacedName)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating provider's ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		err = provider.CreateClusterTemplate(ctx, r.Client, namespacedName)
		if err != nil {
			logger.Error(err, "failed to create provider's ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.InfraProviderClusterTemplateCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
			return err
		}
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.InfraProviderClusterTemplateCondition)
	} else if err != nil {
		logger.Error(err, "failed to get provider's ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.InfraProviderClusterTemplateCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
		return err
	} else {
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.InfraProviderClusterTemplateCondition)
	}
	return nil
}

func (r *ClusterTemplateReconciler) reconcileClusterClass(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, provider capiProvider.Provider, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate) error {
	cc := common.GetClusterClass(namespacedName)
	provider.AlterClusterClass(&cc)

	err := r.Get(ctx, namespacedName, &cc)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Adding ownership to ClusterClass", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		err = controllerutil.SetControllerReference(clusterTemplate, &cc, r.Scheme)
		if err != nil {
			logger.Error(err, "failed to set ownership to ClusterClass", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			return err
		}

		logger.Info("Creating ClusterClass", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		err = r.Create(ctx, &cc)
		if err != nil {
			logger.Error(err, "failed to create ClusterClass", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ClusterClassCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
			return err
		}
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.ClusterClassCondition)
	} else if err != nil {
		logger.Error(err, "failed to get ClusterClass", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ClusterClassCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
		return err
	} else {
		conditions.MarkTrue(clusterTemplate, clustertemplatev1alpha1.ClusterClassCondition)
	}
	return nil
}

func (r *ClusterTemplateReconciler) patchClusterTemplate(ctx context.Context, logger logr.Logger, namespacedName types.NamespacedName, patchHelper *patch.Helper, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate, provider capiProvider.Provider) error {
	logger.Info("Patching ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
	clusterTemplate.Status.Ready = false

	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(clusterTemplate,
		conditions.WithConditions(
			clustertemplatev1alpha1.PrerequisitesCondition,
			clustertemplatev1alpha1.ControlPlaneTemplateCondition,
			clustertemplatev1alpha1.ControlPlaneMachineTemplateCondition,
			clustertemplatev1alpha1.InfraProviderClusterTemplateCondition,
			clustertemplatev1alpha1.ClusterClassCondition,
		),
		conditions.WithStepCounterIf(clusterTemplate.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	// Set the ClusterClass reference if the ClusterTemplate's ClusterClass condition is ready and ClusterTemplate is not being deleted
	if conditions.IsTrue(clusterTemplate, clustertemplatev1alpha1.ClusterClassCondition) && clusterTemplate.ObjectMeta.DeletionTimestamp.IsZero() {
		cc := common.GetClusterClass(namespacedName)
		provider.AlterClusterClass(&cc)

		err := r.Get(ctx, namespacedName, &cc)
		if err != nil {
			logger.Error(err, "failed to get ClusterClass to set the reference", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
			conditions.MarkFalse(clusterTemplate, clustertemplatev1alpha1.ClusterClassCondition, err.Error(), capiv1beta1.ConditionSeverityInfo, "")
			return err
		}

		logger.Info("Setting ClusterClass reference", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		clusterTemplate.Status.ClusterClassRef = &corev1.ObjectReference{
			APIVersion: cc.APIVersion,
			Kind:       cc.Kind,
			Name:       cc.Name,
			Namespace:  cc.Namespace,
		}
	}

	// Set the Ready status based on the summary condition
	if conditions.IsTrue(clusterTemplate, capiv1beta1.ReadyCondition) {
		logger.Info("ClusterTemplate is ready", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
		clusterTemplate.Status.Ready = true
	}
	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		clusterTemplate,
		patch.WithOwnedConditions{Conditions: []capiv1beta1.ConditionType{
			capiv1beta1.ReadyCondition,
			clustertemplatev1alpha1.PrerequisitesCondition,
			clustertemplatev1alpha1.ControlPlaneTemplateCondition,
			clustertemplatev1alpha1.ControlPlaneMachineTemplateCondition,
			clustertemplatev1alpha1.InfraProviderClusterTemplateCondition,
			clustertemplatev1alpha1.ClusterClassCondition,
		}},
	)
}

func (r *ClusterTemplateReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, clusterTemplate *clustertemplatev1alpha1.ClusterTemplate, namespacedName types.NamespacedName, provider capiProvider.Provider) error {
	logger.Info("Running ClusterTemplate reconciliation delete for ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)

	if !controllerutil.ContainsFinalizer(clusterTemplate, clustertemplatev1alpha1.ClusterTemplateFinalizer) {
		logger.Info("No finalizer on clusterTemplate, skipping deletion reconciliation")
		// TODO: I think we should error here
		return nil
	}

	logger.Info("Deleting external dependencies for ClusterTemplate", "namespace", namespacedName.Namespace, "name", namespacedName.Name)
	if err := provider.DeletePrerequisites(ctx, r.Client, namespacedName); err != nil {
		logger.Error(err, "failed to delete external dependencies")
		return err
	}

	controllerutil.RemoveFinalizer(clusterTemplate, clustertemplatev1alpha1.ClusterTemplateFinalizer)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clustertemplatev1alpha1.ClusterTemplate{}).
		Named("clustertemplate").
		Complete(r)
}
