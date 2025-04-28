// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/open-edge-platform/cluster-manager/v2/internal/auth"
)

const (
	// defaultMetricsPort is used to disable metrics
	defaultMetricsPort = -1
)

type Config struct {
	// DisableAuth disables authentication/authorization, should be false for production and true in integration without keycloak
	DisableAuth bool

	// DisableMultitenancy disables multi-tenancy integration, should be false for production and true in integration without multi-tenancy
	DisableMultitenancy bool

	// DisableInventory disables inventory integration, should be false for production and true in integration without infra-manager's inventory
	DisableInventory bool

	// DisableMetrics disables metrics, should be false for production and true in integration without prometheus
	DisableMetrics bool

	OidcUrl              string
	OpaEnabled           bool
	OpaPort              int
	LogLevel             int
	LogFormat            string
	SystemLabelsPrefixes []string
	ClusterDomain        string
	Username             string
	InventoryAddress     string
	MetricsPort          int
}

// ParseConfig parses the configuration from flags and environment variables
func ParseConfig() *Config {
	disableAuth := flag.Bool("disable-auth", false, "(optional) disable rest authentication/authorization")
	disableMt := flag.Bool("disable-mt", false, "(optional) disable multi-tenancy integration")
	disableInv := flag.Bool("disable-inventory", false, "(optional) disable inventory integration")
	logLevel := flag.Int("loglevel", 0, "(optional) log level [trace:-8|debug:-4|info:0|warn:4|error:8]")
	logFormat := flag.String("logformat", "json", "(optional) log format [json|human]")
	prefixes := flag.String("system-labels-prefixes", "", "(optional) comma separated list of system labels prefixes; if not provided, sane defaults are used")
	clusterDomain := flag.String("clusterdomain", "kind.internal", "(optional) cluster domain")
	userName := flag.String("username", "admin", "(optional) user")
	inventoryAddress := flag.String("inventory-endpoint", "mi-inventory:50051", "(optional) inventory address")
	metricsPort := flag.Int("metrics-port", defaultMetricsPort, "(optional) metrics port")
	flag.Parse()

	disableMetrics := false
	if *metricsPort == defaultMetricsPort {
		slog.Info("metrics port not set, disabling metrics")
		disableMetrics = true
	}

	cfg := &Config{
		DisableMetrics:      disableMetrics,
		DisableAuth:         *disableAuth,
		DisableMultitenancy: *disableMt,
		DisableInventory:    *disableInv,
		LogLevel:            *logLevel,
		LogFormat:           strings.ToLower(*logFormat),
		ClusterDomain:       *clusterDomain,
		Username:            *userName,
		InventoryAddress:    *inventoryAddress,
		MetricsPort:         *metricsPort,
	}

	if *prefixes != "" {
		cfg.SystemLabelsPrefixes = strings.Split(*prefixes, ",")
	}

	if !cfg.DisableAuth {
		cfg.OidcUrl = os.Getenv(auth.OidcUrlEnvVar)
	}

	opaEnabled, err := strconv.ParseBool(os.Getenv(auth.OpaEnabledEnvVar))
	if opaEnabled && err != nil {
		slog.Error("failed to parse opa enabled env var", "error", err)
		os.Exit(1)
	}

	cfg.OpaEnabled = opaEnabled
	if cfg.OpaEnabled {
		opaPort, err := strconv.Atoi(os.Getenv(auth.OpaPortEnvVar))
		if err != nil {
			slog.Error("opa port invalid")
			os.Exit(1)
		}

		cfg.OpaPort = opaPort
	}
	return cfg
}

// Validate the configuration
func (c *Config) Validate() error {
	if c.LogFormat != "" {
		validFormats := []string{"json", "human"}
		if !slices.Contains(validFormats, c.LogFormat) {
			slog.Error("invalid log format 'logformat' provided", "provided", c.LogFormat, "valid", validFormats)
			return fmt.Errorf("log format must be one of %v but got %v", validFormats, c.LogFormat)
		}
	}

	if !c.DisableAuth {
		if c.OidcUrl == "" {
			slog.Error("open id connect url 'oidcurl' is required to enable authentication")
			return fmt.Errorf("oidc url is required to enable authentication")
		}

		if _, err := url.ParseRequestURI(c.OidcUrl); err != nil {
			slog.Error("invalid open id connect url 'oidcurl' provided", "error", err)
			return fmt.Errorf("invalid oidc url provided: %w", err)
		}
	}

	if !c.DisableInventory && c.InventoryAddress == "" {
		slog.Error("inventory address is required to enable inventory integration")
		return fmt.Errorf("inventory address is required to enable inventory integration")
	}

	return nil
}
