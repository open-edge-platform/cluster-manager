<!---
  SPDX-FileCopyrightText: (C) 2025 Intel Corporation
  SPDX-License-Identifier: Apache-2.0
-->

# Cluster Manager: Functional Modules and Interactions

This document describes the functional modules that make up the Cluster Manager (CM) service
and explains how they interact with each other and with external systems.

## Table of Contents

- [Overview](#overview)
- [Binaries](#binaries)
- [Internal Modules](#internal-modules)
- [Module Interactions](#module-interactions)
- [External Integrations](#external-integrations)
- [Kubernetes Custom Resources](#kubernetes-custom-resources)

---

## Overview

CM is deployed as two separate binaries, each backed by a set of focused internal packages.

```
cluster-manager/
├── cmd/cluster-manager/        # Binary 1: REST API server
├── cmd/template-controller/    # Binary 2: Kubernetes operator
├── internal/                   # Shared implementation packages
│   ├── auth/
│   ├── cluster/
│   ├── common/
│   ├── config/
│   ├── controller/
│   ├── convert/
│   ├── core/
│   ├── events/
│   ├── inventory/
│   ├── k8s/
│   ├── labels/
│   ├── logger/
│   ├── metrics/
│   ├── middleware/
│   ├── mocks/
│   ├── multitenancy/
│   ├── pagination/
│   ├── providers/
│   ├── rest/
│   ├── template/
│   └── webhook/
├── api/v1alpha1/               # ClusterTemplate CRD definition
└── pkg/api/                    # OpenAPI-generated types and server interface
```

---

## Binaries

### cluster-manager (REST API Server)

**Location**: `cmd/cluster-manager/main.go`  
**Port**: 8080 (HTTP)

This is the primary user-facing service. It starts the following subsystems in order:

1. Parses runtime configuration (`config` package).
2. Initializes structured logging (`logger` package).
3. Optionally overrides system label prefixes (`labels` package).
4. Optionally starts the multi-tenancy watcher (`multitenancy` package).
5. Initializes a Kubernetes dynamic client (`k8s` package).
6. Builds the authenticator chain (`auth` package).
7. Initializes the inventory client (`inventory` package).
8. Starts the HTTP server with all middleware and handlers (`rest` package).

### template-controller (Kubernetes Operator)

**Location**: `cmd/template-controller/main.go`  
**Ports**: 8081 (health probes), 9443 (webhooks, TLS)

This is a controller-runtime based Kubernetes operator. It:

1. Registers all CAPI resource types (Docker, Intel, Kubeadm, RKE2, K3s) with its scheme.
2. Creates a controller-manager and registers the `ClusterTemplateReconciler` (`controller` package).
3. Optionally registers the validating webhook (`webhook` package).
4. Creates an index over `Cluster` resources keyed by `ClusterClass` reference.

---

## Internal Modules

### rest

**Path**: `internal/rest/`  
**Role**: REST API server implementation

This package owns the HTTP server lifecycle and all request handlers. It implements the
`api.StrictServerInterface` generated from the OpenAPI 3.0.3 specification so every endpoint is
type-safe and validated against the spec before a handler runs.

Key responsibilities:
- Build the middleware chain (logging → metrics → project validation → OpenAPI validation → auth).
- Dispatch validated requests to per-endpoint handler files
  (`getv2clusters.go`, `postv2clusters.go`, `deletev2clustersname.go`, etc.).
- Convert between REST API types and CAPI Kubernetes objects via the `convert` package.
- Interact with the Kubernetes API via the `k8s` package.
- Query host trust and immutability status via the `inventory` package.
- Expose a Prometheus metrics endpoint on the same port.

Public interfaces consumed by this package:
- `Authenticator` – validates bearer tokens (implemented by `auth`).
- `Inventory` – checks host properties (implemented by `inventory`).

### controller

**Path**: `internal/controller/`  
**Role**: Kubernetes reconciler for `ClusterTemplate` custom resources

`ClusterTemplateReconciler` is a standard controller-runtime reconciler. On every reconciliation
loop it:
1. Fetches the `ClusterTemplate` CR.
2. Validates Kubernetes version, provider types, and network configuration.
3. Selects the infrastructure/control-plane provider combination via the `providers` package.
4. Creates or updates CAPI resources: `ClusterClass`, `ControlPlaneTemplate`,
   `ControlPlaneMachineTemplate`, and the matching infrastructure templates.
5. Writes back `Ready` status and conditions to the `ClusterTemplate` CR.

### auth

**Path**: `internal/auth/`  
**Role**: Authentication and authorization framework

Provides a pluggable `Authenticator` interface with the following concrete strategies, selected
via runtime configuration:

| Strategy | Purpose | Key config |
|----------|---------|-----------|
| **OIDC** (`provider.go`) | Validates JWT Bearer tokens against a well-known OIDC endpoint | `OIDC_SERVER_URL` |
| **OPA** (`opa.go`) | Evaluates Rego policies for attribute-based access control | `OPA_ENABLED`, `OPA_PORT` |
| **Vault** (`vault.go`) | Obtains and caches tokens via Kubernetes auth method | K8s service-account |
| **Keycloak admin** (`keycloakadmin.go`) | Manages Keycloak users and roles for new projects | `KEYCLOAK_URL` |
| **ClusterAPI token** (`clusterapitoken.go`) | Generates short-lived kubeconfig JWTs for users | configurable TTL |
| **No-op** | Disables auth in development mode | — |

### multitenancy

**Path**: `internal/multitenancy/`  
**Role**: Project lifecycle management via the Nexus tenancy datamodel

Connects to the Nexus gRPC service and subscribes to project creation and deletion events.
When a project is created it:
1. Creates a Kubernetes namespace for the project.
2. Applies Pod Security Admission (PSA) configuration.
3. Creates default `ClusterTemplate` CRs from the embedded defaults (`template` package).
4. Registers the namespace with RBAC labels (`labels` package).

When a project is deleted it removes the namespace and all resources within it.

### k8s

**Path**: `internal/k8s/`  
**Role**: Kubernetes API wrapper

Provides a `Client` struct and the `K8sWrapperClient` interface that wraps the dynamic Kubernetes
client. This abstraction is used throughout the codebase to perform CRUD operations on:
- Namespaces
- CAPI `Cluster` and `ClusterClass` resources
- `ClusterTemplate` CRs
- Machine resources (`IntelMachine`, `DockerMachine`)
- `IntelMachineBinding` resources
- Kubernetes `Secret` objects

### convert

**Path**: `internal/convert/`  
**Role**: Type conversion between REST API and CAPI

`ClusterSpecToCAPICluster` translates a REST `ClusterSpec` into a CAPI `Cluster` object, resolving
the `ClusterClass` reference from the named template.  
`CAPIClusterToClusterInfo` performs the reverse, mapping CAPI `Cluster` status and associated
`Machine` objects back to the REST `ClusterInfo` response type.

### providers

**Path**: `internal/providers/`  
**Role**: Infrastructure provider abstractions

Defines the `Provider` interface and five concrete implementations representing the supported
control-plane × infrastructure combinations:

| File | Control-plane | Infrastructure |
|------|--------------|----------------|
| `k3sintel.go` | K3s | Intel |
| `k3sdocker.go` | K3s | Docker (testing) |
| `rke2intel.go` | RKE2 | Intel |
| `rke2docker.go` | RKE2 | Docker (testing) |
| `kubeadmdocker.go` | Kubeadm | Docker (testing) |

Each provider returns the CAPI template specifications that the `controller` package uses to
create `ClusterClass` and related resources.

### inventory

**Path**: `internal/inventory/`  
**Role**: gRPC client for the infra-core inventory service

Exposes two key queries used during cluster provisioning:
- `GetHostTrustedCompute` – returns whether a host has TPM / trusted compute enabled.
- `IsImmutable` – returns whether a host's configuration is locked.

Also subscribes to host events over a streaming gRPC watch and propagates them to the `events`
channel so other subsystems can react to host state changes.

Default address: `mi-inventory:50051` (configurable via `INVENTORY_ADDRESS`).

### middleware

**Path**: `internal/middleware/`  
**Role**: HTTP middleware chain

| Component | File | Responsibility |
|-----------|------|---------------|
| Logger | `logger.go` | Structured request/response logging |
| Metrics | `metrics.go` | Prometheus response-time histogram and HTTP-status counter |
| ProjectValidator | `project_validator.go` | Validates that the project namespace exists before routing the request |

### template

**Path**: `internal/template/`  
**Role**: Template file I/O

Reads default `ClusterTemplate` YAML files embedded in the binary (from
`default-cluster-templates/`) and provides them to the `multitenancy` package when a new project
namespace is initialised. Also reads Pod Security Admission configuration files.

### config

**Path**: `internal/config/`  
**Role**: Runtime configuration

Parses flags and environment variables into a `Config` struct. Key fields include:
- `DisableMultitenancy` – skips Nexus watcher startup.
- `SystemLabelsPrefixes` – overrides the label prefixes treated as system-managed.
- `DefaultTemplate` – name of the template to apply to new project namespaces.
- OIDC, OPA, Vault, and Keycloak URLs and ports.

### labels

**Path**: `internal/labels/`  
**Role**: Label namespace management

Maintains the set of label-key prefixes that are considered "system-managed" (i.e., not writable
by end users). Used by the `rest` package to separate user labels from system labels when
processing `PUT /v2/clusters/{name}/labels` requests.

### pagination

**Path**: `internal/pagination/`  
**Role**: API result pagination and filtering

Provides `SortFilter` helpers that apply offset/limit pagination, field-based sorting, and
substring filtering to in-memory slices of API objects. Used by `getv2clusters.go` and
`getv2templates.go`.

### metrics

**Path**: `internal/metrics/`  
**Role**: Prometheus metric definitions

Defines the application-level Prometheus collectors (response-time histogram, HTTP-status counter)
that are registered once at startup and updated by the `middleware` package on every request.

### logger

**Path**: `internal/logger/`  
**Role**: Structured logging initialisation

Configures the `slog` default logger as JSON or human-readable format based on the `Config`
value. Called once during startup by `cmd/cluster-manager/main.go`.

### events

**Path**: `internal/events/`  
**Role**: Internal event bus

Defines a minimal `Event` interface and an `EventPublisher` used by the `inventory` package to
broadcast host-state changes to subscribers within the same process.

### webhook

**Path**: `internal/webhook/v1alpha1/`  
**Role**: Validating admission webhook for `ClusterTemplate`

Implements a controller-runtime `CustomValidator` that is called by the Kubernetes API server
before a `ClusterTemplate` CR is admitted. It verifies that no `Cluster` resources still
reference a `ClusterTemplate` that is about to be deleted, preventing orphaned clusters.

### core

**Path**: `internal/core/`  
**Role**: Shared constants

Holds domain-wide constants such as the index field name used by the webhook to look up
`Cluster` resources by `ClusterClass` reference.

### common

**Path**: `internal/common/`  
**Role**: Shared helper functions

Provides utility functions shared across the `rest` and `controller` packages, for example
building `ClusterClass` names from template name and version.

### cluster

**Path**: `internal/cluster/`  
**Role**: Cluster-level query helpers

Thin helpers that retrieve the node list and template reference for a named cluster; used by
REST handlers that need cluster-detail information.

### mocks

**Path**: `internal/mocks/`  
**Role**: Test utilities

Contains a lightweight OIDC/auth mock server (`RunAuthServer`) used during service-level tests
and local development to simulate the authentication stack without a real identity provider.

---

## Module Interactions

### Request flow: cluster-manager REST API

```
HTTP request (port 8080)
        │
        ▼
middleware.Logger           – log request details
        │
        ▼
middleware.RequestDurationMetrics  – start Prometheus timer
        │
        ▼
middleware.ProjectValidator  – verify project namespace exists (→ k8s)
        │
        ▼
oapi-codegen validator       – validate against OpenAPI 3.0.3 spec
        │
        ▼
auth.Authenticator           – OIDC JWT validation + OPA policy check
        │
        ▼
rest.Server handler          – per-endpoint business logic
        ├─── k8s.Client          – read / write Kubernetes resources
        ├─── convert             – translate API ↔ CAPI types
        ├─── inventory.Client    – query host trust / immutability (gRPC)
        ├─── pagination          – sort / filter / paginate results
        └─── labels              – separate system vs user labels
        │
        ▼
JSON response
```

### Reconciliation flow: template-controller

```
ClusterTemplate CR created / updated
        │
        ▼
controller.ClusterTemplateReconciler.Reconcile()
        ├─── validate spec (version, provider enums, network CIDRs)
        ├─── providers.GetProvider()   – select K3s / RKE2 / Kubeadm × Intel / Docker
        ├─── k8s API – create or update:
        │        ├─ InfraProviderClusterTemplate
        │        ├─ ControlPlaneTemplate
        │        ├─ ControlPlaneMachineTemplate
        │        └─ ClusterClass
        └─── update ClusterTemplate.Status (Ready, Conditions)
```

### Project lifecycle flow: multitenancy watcher

```
Nexus gRPC event (project created)
        │
        ▼
multitenancy.TenancyDatamodel
        ├─── k8s.CreateNamespace()
        ├─── k8s.CreateSecret()          – PSA configuration
        ├─── template.ReadDefaultTemplates()
        └─── k8s.CreateTemplate()        – install default ClusterTemplates

Nexus gRPC event (project deleted)
        │
        ▼
multitenancy.TenancyDatamodel
        └─── k8s.DeleteNamespace()       – cascades to all resources
```

### Admission webhook flow: template-controller

```
DELETE ClusterTemplate (Kubernetes API server)
        │
        ▼
webhook.ClusterTemplateCustomValidator.ValidateDelete()
        └─── k8s index lookup – find Cluster objects referencing this ClusterClass
                  ├─ clusters found  →  reject DELETE (409 / validation error)
                  └─ none found      →  allow DELETE
```

---

## External Integrations

| System | Protocol | Purpose | Default address / config |
|--------|----------|---------|--------------------------|
| **OIDC server** | HTTPS | JWT signature validation and JWKS fetch | `OIDC_SERVER_URL` |
| **Keycloak** | HTTPS | User and role management for new projects | `KEYCLOAK_URL` |
| **OPA** | HTTP | Policy evaluation for attribute-based access control | `OPA_PORT` (default `8181`) |
| **HashiCorp Vault** | HTTPS | Short-lived token management via Kubernetes auth | K8s service account |
| **infra-core inventory** | gRPC | Host trust and immutability queries; host-event stream | `INVENTORY_ADDRESS` (default `mi-inventory:50051`) |
| **Nexus (orch-utils)** | gRPC | Project creation / deletion event subscription | In-cluster K8s service |
| **Kubernetes API server** | HTTPS | All cluster, template, machine, and namespace operations | In-cluster config |

---

## Kubernetes Custom Resources

### ClusterTemplate (CM-owned CRD)

`ClusterTemplate` is the primary CM-owned custom resource. Users create it to define a cluster
blueprint. The `template-controller` operator watches it and generates the downstream CAPI
resources.

```yaml
apiVersion: edge-orchestrator.intel.com/v1alpha1
kind: ClusterTemplate
metadata:
  name: <template-name>
  namespace: <project-namespace>
spec:
  controlPlaneProviderType: k3s | rke2 | kubeadm
  infraProviderType: intel | docker
  kubernetesVersion: v1.x.y
  clusterConfiguration: |   # provider-specific YAML
    ...
  clusterNetwork:
    services:
      cidrBlocks: [...]
    pods:
      cidrBlocks: [...]
  clusterLabels: {}
status:
  ready: true | false
  clusterClassRef:
    name: <template-name>
    namespace: <project-namespace>
  conditions: [...]
```

### CAPI resources generated by template-controller

| Resource kind | API group | Created by |
|---------------|-----------|-----------|
| `ClusterClass` | `cluster.x-k8s.io/v1beta1` | `controller` |
| `KubeadmControlPlaneTemplate` | `controlplane.cluster.x-k8s.io/v1beta1` | `controller` |
| `RKE2ControlPlaneTemplate` | `controlplane.cluster.x-k8s.io/v1beta1` | `controller` |
| `K3sControlPlaneTemplate` | `controlplane.cluster.x-k8s.io/v1beta2` | `controller` |
| `DockerClusterTemplate` | `infrastructure.cluster.x-k8s.io/v1beta1` | `controller` |
| `IntelClusterTemplate` | `infrastructure.cluster.x-k8s.io/v1alpha1` | `controller` |
| `DockerMachineTemplate` | `infrastructure.cluster.x-k8s.io/v1beta1` | `controller` |
| `IntelMachineTemplate` | `infrastructure.cluster.x-k8s.io/v1alpha1` | `controller` |

### CAPI resources managed by cluster-manager REST server

| Resource kind | API group | Managed via |
|---------------|-----------|------------|
| `Cluster` | `cluster.x-k8s.io/v1beta1` | `k8s.Client` |
| `IntelMachineBinding` | `infrastructure.cluster.x-k8s.io/v1alpha1` | `k8s.Client` |
