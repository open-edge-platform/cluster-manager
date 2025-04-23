// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package rest

import (
	"context"
	"fmt"

	"log/slog"

	"github.com/open-edge-platform/cluster-manager/v2/internal/convert"
	"github.com/open-edge-platform/cluster-manager/v2/internal/core"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

// (GET /v2/clusters/{nodeId}/clusterdetail)
func (s *Server) GetV2ClustersNodeIdClusterdetail(ctx context.Context, request api.GetV2ClustersNodeIdClusterdetailRequestObject) (api.GetV2ClustersNodeIdClusterdetailResponseObject, error) {
	activeProjectID := request.Params.Activeprojectid.String()

	clusterDetails, err := s.getClusterDetails(ctx, activeProjectID, request.NodeId)
	if err != nil {
		return api.GetV2ClustersNodeIdClusterdetail404JSONResponse{
			N404NotFoundJSONResponse: api.N404NotFoundJSONResponse{
				Message: ptr(err.Error()),
			},
		}, nil
	}
	labels := labels.Filter(convert.MapAnyToString(*clusterDetails.Labels))
	unstructuredLabels := convert.MapStringToAny(labels)
	clusterDetails.Labels = &unstructuredLabels
	return api.GetV2ClustersNodeIdClusterdetail200JSONResponse(clusterDetails), nil
}

func (s *Server) getClusterDetails(ctx context.Context, activeProjectID, nodeId string) (api.ClusterDetailInfo, error) {
	// retrieve cluster using node id and populate detail info with it
	// get capi machine object using ID and link it to cluster
	unstructuredMachines, err := s.k8sclient.ListCached(ctx, core.MachineResourceSchema, activeProjectID, v1.ListOptions{})
	if unstructuredMachines == nil || len(unstructuredMachines.Items) == 0 {
		slog.Error("failed to get machine", "namespace", activeProjectID, "ID", nodeId, "error", err)
		return api.ClusterDetailInfo{}, fmt.Errorf("machine not found")
	}
	if err != nil {
		slog.Error("failed to get machine", "namespace", activeProjectID, "ID", nodeId, "error", err)
		return api.ClusterDetailInfo{}, err
	}

	machines, err := convertUnsructuredtoMachine(unstructuredMachines)
	if err != nil {
		slog.Error("failed to convert machine to structured object", "namespace", activeProjectID, "ID", nodeId, "error", err)
		return api.ClusterDetailInfo{}, err
	}
	for _, machine := range machines {
		nodeRef := machine.Status.NodeRef
		if nodeRef != nil && string(nodeRef.UID) == nodeId {
			return s.getCluster(ctx, activeProjectID, machine.Spec.ClusterName)
		}
	}

	return api.ClusterDetailInfo{}, fmt.Errorf("machine not found")

}

func convertUnsructuredtoMachine(unstructuredMachines *unstructured.UnstructuredList) ([]*capi.Machine, error) {
	machines := make([]*capi.Machine, 0)
	for _, unstructuredMachine := range unstructuredMachines.Items {
		machine := &capi.Machine{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredMachine.Object, machine); err != nil {
			return nil, err
		}
		machines = append(machines, machine)
	}
	return machines, nil
}
