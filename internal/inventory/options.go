// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package inventory

import (
	"sync"

	"github.com/open-edge-platform/cluster-manager/v2/internal/k8s"
)

type Options struct {
	wg               *sync.WaitGroup
	inventoryAddress string
	enableTracing    bool
	enableMetrics    bool
	k8sClient        k8s.K8sWrapperClient
}

func (o *Options) WaitGroup() *sync.WaitGroup {
	return o.wg
}

func (o *Options) InventoryAddress() string {
	return o.inventoryAddress
}

func (o *Options) TracingEnabled() bool {
	return o.enableTracing
}

func (o *Options) MetricsEnabled() bool {
	return o.enableMetrics
}

type optionsBuilder struct {
	options *Options
}

type OptionsBuilder interface {
	WithWaitGroup(wg *sync.WaitGroup) OptionsBuilder
	WithInventoryAddress(address string) OptionsBuilder
	WithTracing(enableTracing bool) OptionsBuilder
	WithMetrics(enableMetrics bool) OptionsBuilder
	WithK8sClient(client k8s.K8sWrapperClient) OptionsBuilder
	Build() Options
}

func NewOptionsBuilder() OptionsBuilder {
	return &optionsBuilder{
		options: &Options{},
	}
}

func (b *optionsBuilder) WithWaitGroup(wg *sync.WaitGroup) OptionsBuilder {
	b.options.wg = wg
	return b
}

func (b *optionsBuilder) WithInventoryAddress(address string) OptionsBuilder {
	b.options.inventoryAddress = address
	return b
}

func (b *optionsBuilder) WithTracing(enableTracing bool) OptionsBuilder {
	b.options.enableTracing = enableTracing
	return b
}

func (b *optionsBuilder) WithMetrics(enableMetrics bool) OptionsBuilder {
	b.options.enableMetrics = enableMetrics
	return b
}

func (b *optionsBuilder) WithK8sClient(client k8s.K8sWrapperClient) OptionsBuilder {
	b.options.k8sClient = client
	return b
}

func (b *optionsBuilder) Build() Options {
	return *b.options
}
