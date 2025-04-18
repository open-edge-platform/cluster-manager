# Cluster Manager

## Table of Contents

- [Overview](#overview)
- [Get Started](#get-started)
- [Develop](#develop)
- [Contribute](#contribute)
- [License](#license)

## Overview

CM is a microservice within the Cluster Orchestration (CO) component. It serves as an intermediary layer
that bridges the gap between CO and Cluster-API (CAPI). It translates API calls for CO cluster
and cluster template resources into CAPI-compatible formats and implements features that are required
for CO services but not natively supported by CAPI such as Role-Based Access Control (RBAC)
and handling project creation and deletion events. CM is stateless. It stores all persistent data
in CAPI-specified Custom Resources (CRs) using annotations to store CO-specific information.

CM consists of four modules:

### Rest Server

This module is responsible for processing incoming REST API calls. It ensures that the request
is authenticated, authorized, and validated before forwarding the request to the appropriate
internal component for further processing.

### Tenant Controller

This module handles project creation and deletion events and manages the Kubernetes namespaces
that represent projects.

### Cluster Controller

This module handles cluster creation and deletion events and manages the CAPI Cluster and
IntelMachineBinding CRs that represent clusters.

### Template Controller

This module is a Kubernetes operator that manages ClusterTemplate CRs. It is responsible for
validating cluster templates and generating the corresponding CAPI resources such as
ControlPlaneTemplate and ClusterClass.

## Get Started

Instructions on how to build, install and test.

### Prerequisites

This code requires the following tools to be installed on your development machine:

- [Go\* programming language](https://go.dev) - check [Makefile](./Makefile) on usage
- [golangci-lint](https://github.com/golangci/golangci-lint) - check [Makefile](./Makefile)  on usage
- [mockery](github.com/vektra/mockery) - check [Makefile](./Makefile)  on usage
- Python\* programming language version 3.10 or later
- [gocover-cobertura](github.com/boumenot/gocover-cobertura) - check [Makefile](./Makefile)  on usage
- [Docker](https://docs.docker.com/engine/install/) to build containers
- [KinD](https://kind.sigs.k8s.io/docs/user/quick-start/) based cluster for end-to-end tests
- [Helm](https://helm.sh/docs/intro/install/) for install helm charts for end-to-end tests

### Build, Install and Test

The basic workflow to make changes to the code, verify those changes, and create a pull request (PR) is:

0. Edit and build the code with `make build` command

1. Run linters with `make lint` command

2. Run the unit tests with `make test-unit` command

3. Run the service tests

    - Create kind cluster with CAPI enabled with `make kind-create` command

    - Build the CM images and install them into the cluster with `make helm-install` command

    - Expose CM service to localhost with `make kind-expose-cm` command

    - Execute the service tests with `make test-service` command

## Develop

### APIs

CM supports the following REST APIs:

| API                                 | Method | Description                                                       |
|-------------------------------------|--------|-------------------------------------------------------------------|
| /v2/clusters                        | GET    | Get all clusters' information                                     |
| /v2/clusters                        | POST   | Create a cluster                                                  |
| /v2/clusters/{name}                 | GET    | Get the cluster {name} information                                |
| /v2/clusters/{name}                 | DELETE | Delete the cluster {name}                                         |
| /v2/clusters/{nodeId}/clusterdetail | GET    | Get cluster detailed information by {nodeId}                      |
| /v2/clusters/{name}/nodes           | PUT    | Update cluster {name} nodes                                       |
| /v2/clusters/{name}/nodes/{nodeId}  | DELETE | Delete the cluster {name} node {nodeId}                           |
| /v2/clusters/{name}/labels          | PUT    | Update cluster {name} labels                                      |
| /v2/clusters/{name}/template        | PUT    | Update the cluster {name} template                                |
| /v2/clusters/{name}/kubeconfigs     | GET    | Get the cluster's kubeconfig file by its name {name}              |
| /v2/healthz                         | GET    | Get the Cluster Manager REST API healthz status              |
| /v2/templates                       | GET    | Get all templates' information                                    |
| /v2/templates                       | POST   | Import templates                                                  |
| /v2/templates/{name}/{version}      | GET    | Get information on a specific template                            |
| /v2/templates/{name}/{version}      | DELETE | Delete a specific template                                        |
| /v2/templates/{name}/versions       | GET    | Get all versions of templates matching a particular template name |
| /v2/templates/{name}/default        | PUT    | Update this template as the default template                      |

### Developer Utilities

There are several convenience make targets to support developer activities, you can use help to
see a list of makefile targets. The following is a list of makefile targets that support developer
activities:

- `manifests`   Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
- `generate`    Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
- `fmt`         Run go fmt against code
- `vet`         Run go vet against code
- `test`        Run tests
- `mocks`       Generate mock files for unit test using mockery
- `coverage`    Generate test coverage report
- `test-e2e`    Run the e2e tests
- `lint`        Run linters
- `lint-fix`    Run golangci-lint linter and perform fixes
- `lint-config` Verify golangci-lint linter configuration

## Contribute

We welcome contributions from the community!
To contribute, please open a pull request to have your changes reviewed and merged into the main branch.
To learn how to contribute to the project, see the
[contributor's guide](https://docs.openedgeplatform.intel.com/edge-manage-docs/main/developer_guide/contributor_guide/index.html).
We encourage you to add appropriate unit tests and e2e tests if your contribution introduces a new feature.

The project will accept contributions through Pull-Requests (PRs).
PRs must be built successfully by the CI pipeline, pass linters verifications, and the unit tests.

## Community and Support

To learn more about the project, its community, and governance, visit the [Edge Orchestrator Community](https://github.com/open-edge-platform).
For support, start with
[Troubleshooting](https://docs.openedgeplatform.intel.com/edge-manage-docs/main/developer_guide/troubleshooting/index.html)
or [contact us](https://github.com/open-edge-platform/).

There are several convenience make targets to support developer activities, you can use help to see a list of makefile targets.

## License

Cluster Manager is licensed under [Apache 2.0 License](LICENSES/Apache-2.0.txt)
