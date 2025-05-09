// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package labels_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/open-edge-platform/cluster-manager/v2/internal/labels"
)

func TestUserLabels(t *testing.T) {
	clusterLabels := map[string]string{
		"edge-orchestrator.intel.com/cluster-id":  "cluster-479656a6",
		"edge-orchestrator.intel.com/clustername": "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"edge-orchestrator.intel.com/project-id":  "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
		"edge-orchestrator.intel.com/template":    "scaletest-v0.0.2",
		"clustername":                             "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"cpumanager":                              "true",
		"prometheusMetricsURL":                    "metrics-node.scale.espd.infra-host.com",
		"test-app":                                "enabled",
		"tests":                                   "scale",
		"cluster.x-k8s.io/cluster-name":           "demo-cluster",
		"default":                                 "true",
		"edge-orchestrator.intel.com/users-label": "user-value",
		"topology.cluster.x-k8s.io/owned":         "",
		"trusted-compute-compatible":              "true",
	}

	want := map[string]string{
		"clustername": "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"cpumanager":  "true",
		"test-app":    "enabled",
		"tests":       "scale",
		"default":     "true",
	}

	got := labels.UserLabels(clusterLabels)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("filter mismatch (-want +got):\n%s", diff)
	}
}

func TestSystemLabels(t *testing.T) {
	clusterLabels := map[string]string{
		"edge-orchestrator.intel.com/cluster-id":  "cluster-479656a6",
		"edge-orchestrator.intel.com/clustername": "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"edge-orchestrator.intel.com/project-id":  "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
		"edge-orchestrator.intel.com/template":    "scaletest-v0.0.2",
		"clustername":                             "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"cpumanager":                              "true",
		"prometheusMetricsURL":                    "metrics-node.scale.espd.infra-host.com",
		"test-app":                                "enabled",
		"tests":                                   "scale",
		"cluster.x-k8s.io/cluster-name":           "demo-cluster",
		"default":                                 "true",
		"edge-orchestrator.intel.com/users-label": "user-value",
		"topology.cluster.x-k8s.io/owned":         "",
		"trusted-compute-compatible":              "false",
	}

	want := map[string]string{
		"edge-orchestrator.intel.com/cluster-id":  "cluster-479656a6",
		"edge-orchestrator.intel.com/clustername": "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"edge-orchestrator.intel.com/project-id":  "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
		"edge-orchestrator.intel.com/template":    "scaletest-v0.0.2",
		"prometheusMetricsURL":                    "metrics-node.scale.espd.infra-host.com",
		"cluster.x-k8s.io/cluster-name":           "demo-cluster",
		"edge-orchestrator.intel.com/users-label": "user-value",
		"topology.cluster.x-k8s.io/owned":         "",
		"trusted-compute-compatible":              "false",
	}

	got := labels.SystemLabels(clusterLabels)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("filter mismatch (-want +got):\n%s", diff)
	}
}

func TestMerge(t *testing.T) {
	specLabels := map[string]string{
		"edge-orchestrator.intel.com/cluster-id": "cluster-479656a6",
		"cluster.x-k8s.io/cluster-name":          "demo-cluster",
		"edge-orchestrator.intel.com/project-id": "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
	}

	templateLabels := map[string]string{
		"default-extension": "baseline",
	}

	want := map[string]string{
		"edge-orchestrator.intel.com/cluster-id": "cluster-479656a6",
		"cluster.x-k8s.io/cluster-name":          "demo-cluster",
		"edge-orchestrator.intel.com/project-id": "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
		"default-extension":                      "baseline",
	}

	got := labels.Merge(specLabels, templateLabels)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("merge mismatch (-want +got):\n%s", diff)
	}
}

func TestRemove(t *testing.T) {
	clusterLabels := map[string]string{
		"edge-orchestrator.intel.com/cluster-id":  "cluster-479656a6",
		"edge-orchestrator.intel.com/clustername": "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"edge-orchestrator.intel.com/project-id":  "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
		"edge-orchestrator.intel.com/template":    "scaletest-v0.0.2",
		"clustername":                             "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"cpumanager":                              "true",
		"prometheusMetricsURL":                    "metrics-node.scale.espd.infra-host.com",
	}

	want := map[string]string{
		"edge-orchestrator.intel.com/cluster-id":  "cluster-479656a6",
		"edge-orchestrator.intel.com/clustername": "scale-0a9c889f-97a4-5a27-aa57-e82300a2b19c",
		"edge-orchestrator.intel.com/project-id":  "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
		"edge-orchestrator.intel.com/template":    "scaletest-v0.0.2",
		"cpumanager":                              "true",
	}

	got := labels.Delete(clusterLabels, "clustername", "prometheusMetricsURL")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("remove mismatch (-want +got):\n%s", diff)
	}
}

/*
// Valid verifies label format against https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set

Valid label keys have :
- two segments: an optional prefix and name, separated by a slash (/)
- the name segment is required and must be 63 characters or less, beginning and ending with an alphanumeric character ([a-z0-9A-Z]) with dashes (-), underscores (_), dots (.), and alphanumerics between
- The prefix is optional. If specified, the prefix must be a DNS subdomain: a series of DNS labels separated by dots (.), not longer than 253 characters in total, followed by a slash (/)

Valid label value:
- must be 63 characters or less (can be empty),
- unless empty, must begin and end with an alphanumeric character ([a-z0-9A-Z]),
- could contain dashes (-), underscores (_), dots (.), and alphanumerics between
*/

func TestValidSuccess(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
	}{
		{
			name: "multiple labels with prefixes and values",
			labels: map[string]string{
				"edge-orchestrator.intel.com/cluster-id": "cluster-479656a6",
				"cluster.x-k8s.io/cluster-name":          "demo-cluster",
				"edge-orchestrator.intel.com/project-id": "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
			},
		},
		{
			name: "multiple labels with values",
			labels: map[string]string{
				"cluster-id":   "cluster-479656a6",
				"cluster-name": "demo-cluster",
				"project-id":   "0ba9d4dc-c4c2-4904-b526-57ace6e76922",
			},
		},
		{
			name:   "empty labels",
			labels: map[string]string{},
		},
		{
			name: "single label with prefix and value",
			labels: map[string]string{
				"edge-orchestrator.intel.com/cluster-id": "cluster-479656a6",
			},
		},
		{
			name: "single label with value",
			labels: map[string]string{
				"cluster-id": "cluster-479656a6",
			},
		},
		{
			name: "single label with prefix and empty value",
			labels: map[string]string{
				"edge-orchestrator.intel.com/cluster-id": "",
			},
		},
		{
			name: "prometheus label",
			labels: map[string]string{
				"prometheusMetricsURL": "metrics-node.kind.internal",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !labels.Valid(tt.labels) {
				t.Errorf("labels should be valid")
			}
		})
	}
}

func TestValidFailure(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
	}{
		{
			name:   "too long name (more than 63 characters)",
			labels: map[string]string{strings.Repeat("a", 64): ""},
		},
		{
			name:   "too long prefix (more than 253 characters)",
			labels: map[string]string{strings.Repeat("a", 254) + "/name": ""},
		},
		{
			name:   "too long value (more than 63 characters)",
			labels: map[string]string{"name": strings.Repeat("a", 64)},
		},
		{
			name:   "invalid prefix",
			labels: map[string]string{"@invalid-prefix/name": ""},
		},
		{
			name:   "invalid name",
			labels: map[string]string{"prefix/invalid-name!": ""},
		},
		{
			name:   "invalid value",
			labels: map[string]string{"name": "invalid-value!"},
		},
		{
			name: "prefix without name",
			labels: map[string]string{
				"prefix/": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if labels.Valid(tt.labels) {
				t.Errorf("labels should be invalid")
			}
		})
	}
}
