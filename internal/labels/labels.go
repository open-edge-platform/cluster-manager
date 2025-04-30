// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package labels

import (
	"regexp"
	"strings"
)

const (
	DefaultLabelKey = "default"
	DefaultLabelVal = "true"

	PlatformPrefix               = "edge-orchestrator.intel.com"
	PrometheusMetricsUrlLabelKey = "prometheusMetricsURL"
	PrometheusMetricsSubdomain   = "metrics-node"
	TrustedComputeLabelKey       = "trusted-compute-compatible"
	capiDomainLabelKey           = "cluster.x-k8s.io"
	capiTopologyLabelKey         = "topology.cluster.x-k8s.io"
	extension                    = "default-extension"
)

var (
	systemPrefixes  = []string{PlatformPrefix, capiDomainLabelKey, capiTopologyLabelKey, PrometheusMetricsUrlLabelKey, TrustedComputeLabelKey, extension}
	labelKeyRegex   = regexp.MustCompile(`^(([A-Za-z0-9][-A-Za-z0-9_.]{0,250})?[A-Za-z0-9]\/)?([A-Za-z0-9][-A-Za-z0-9_.]{0,61})?[A-Za-z0-9]$`)
	labelValueRegex = regexp.MustCompile(`^([A-Za-z0-9][-A-Za-z0-9_.]{0,61})?[A-Za-z0-9]?$`)
)

func OverrideSystemPrefixes(prefixes []string) {
	systemPrefixes = prefixes
}

// Filter returns new map with only user defined labels
func Filter(clusterLabels map[string]string) map[string]string {
	f := map[string]string{}

	keep := func(s string) bool {
		for _, p := range systemPrefixes {
			if strings.HasPrefix(s, p) {
				return false
			}
		}
		return true
	}

	for key, value := range clusterLabels {
		if !keep(key) {
			continue
		}

		f[key] = value
	}
	return f
}

// Merge returns a new map with all input labels merged into one
func Merge(labels ...map[string]string) map[string]string {
	mergedLabels := make(map[string]string)
	for _, l := range labels {
		for k, v := range l {
			mergedLabels[k] = v
		}
	}
	return mergedLabels
}

// Valid verifies label format against https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
func Valid(labels map[string]string) bool {
	for k, v := range labels {
		if !labelKeyRegex.MatchString(k) || !labelValueRegex.MatchString(v) {
			return false
		}
	}
	return true
}
