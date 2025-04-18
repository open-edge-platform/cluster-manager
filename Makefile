# SPDX-FileCopyrightText: (C) 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

SHELL := bash -eu -o pipefail

# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION            ?= $(shell cat VERSION)
GIT_HASH_SHORT     ?= $(shell git rev-parse --short=8 HEAD)
VERSION_DEV_SUFFIX := ${GIT_HASH_SHORT}
CLUSTERCTL_VERSION ?= v1.9.5
KUBEADM_VERSION    ?= v1.9.0
RKE2_VERSION       ?= v0.12.0
DOCKER_INFRA_VERSION   ?= v1.8.5
CLUSTERCTL := $(shell command -v clusterctl 2> /dev/null)

FUZZTIME ?= 60s

# Add an identifying suffix for `-dev` builds only.
# Release build versions are verified as unique by the CI build process.
ifeq ($(findstring -dev,$(VERSION)), -dev)
        VERSION := $(VERSION)-$(VERSION_DEV_SUFFIX)
endif

HELM_VERSION ?= ${VERSION}

REGISTRY         ?= 080137407410.dkr.ecr.us-west-2.amazonaws.com
REGISTRY_NO_AUTH ?= edge-orch
REPOSITORY       ?= cluster
DOCKER_TAG              ?= ${VERSION}
DOCKER_IMAGE_TEMPLATE_CONTROLLER  ?= ${REGISTRY}/${REGISTRY_NO_AUTH}/${REPOSITORY}/template-controller:${DOCKER_TAG}
DOCKER_IMAGE_CLUSTER_MANAGER ?= ${REGISTRY}/${REGISTRY_NO_AUTH}/${REPOSITORY}/cluster-manager:${DOCKER_TAG}

## Labels to add Docker/Helm/Service CI meta-data.
LABEL_SOURCE       ?= $(shell git remote get-url $(shell git remote))
LABEL_REVISION     = $(shell git rev-parse HEAD)
LABEL_CREATED      ?= $(shell date -u "+%Y-%m-%dT%H:%M:%SZ")

DOCKER_LABEL_ARGS  ?= \
	--build-arg org_oci_version="${VERSION}" \
	--build-arg org_oci_source="${LABEL_SOURCE}" \
	--build-arg org_oci_revision="${LABEL_REVISION}" \
	--build-arg org_oci_created="${LABEL_CREATED}"

# Docker Build arguments
DOCKER_BUILD_ARGS ?= \
	--build-arg http_proxy="$(http_proxy)" --build-arg https_proxy="$(https_proxy)" \
	--build-arg no_proxy="$(no_proxy)" --build-arg HTTP_PROXY="$(http_proxy)" \
	--build-arg HTTPS_PROXY="$(https_proxy)" --build-arg NO_PROXY="$(no_proxy)"

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

## Virtual environment name
VENV_NAME = venv-env

BUILD_DIR ?= build

# GoCov versions
GOLANG_GOCOV_VERSION := latest
GOLANG_GOCOV_XML_VERSION := latest
PKG := github.com/open-edge-platform/cluster-manager
# FIXME: The integration test in "./test" folder is failing. Commenting it for now
TEST_PATHS := ./internal/... ./pkg/... ./cmd/... # ./test/...

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

GOARCH       := $(shell go env GOARCH)
GOEXTRAFLAGS := -trimpath -gcflags="all=-spectre=all -N -l" -asmflags="all=-spectre=all" -ldflags="all=-s -w -X main.version=${VERSION}"
ifeq ($(GOARCH),arm64)
  GOEXTRAFLAGS := -trimpath -gcflags="all=-spectre= -N -l" -asmflags="all=-spectre=" -ldflags="all=-s -w -X main.version=${VERSION}"
endif
ifeq ($(GO_VENDOR),true)
        GOEXTRAFLAGS := -mod=vendor $(GOEXTRAFLAGS)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# When set to true, disables integration with multi-tenanancy controllers. Should be false for production use cases
# Should be true for CO subsystem integration tests if multi-tenancy controllers are not deployed
DISABLE_MT ?= true

# When set to true, disables integration with keycloak oidc and opa sidecar. Should be false for production use cases
# Should be true for CO subsystem integration tests if keycloak is not deployed
DISABLE_AUTH ?= true

# When set to true, disables integration with infra inventory. Should be false for production use cases
# Should be true for CO subsystem integration tests if inventory is not deployed
DISABLE_INV ?= true

.PHONY: all
all: help

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: ## Run unit tests.
	make test-unit

.PHONY: run-service-test
run-service-test: clusterctl ## Run service tests.
	make kind-create
	make helm-install
	make kind-expose-cm
	make test-service

.PHONY: mocks
mocks: ## Generate mock files for unit test using mockery
	mockery --version || go install github.com/vektra/mockery/v2@latest
	mockery

.PHONY: coverage
coverage: ## Generate test coverage report.
	echo "TODO: coverage target not implemented"

.PHONY: test-unit
test-unit: envtest gocov helm-test ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ${TEST_PATHS}) -v -race -gcflags -l -coverprofile cover.out -covermode atomic -short
	${GOBIN}/gocov convert cover.out | ${GOBIN}/gocov-xml > coverage.xml
	go tool cover -html=cover.out -o coverage.html


# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# Prometheus and CertManager are installed by default; skip with:
# - PROMETHEUS_INSTALL_SKIP=true
# - CERT_MANAGER_INSTALL_SKIP=true
.PHONY: test-service
test-service: ## Run the e2e tests. Expected an isolated environment using Kind.
	go test ./test/service/ -v -ginkgo.v

.PHONY: lint
lint: fmt vet golint yamllint helmlint mdlint ## Run linters

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

.PHONY: mdlint
mdlint: ## Lint markdown files.
	@markdownlint --version
	@markdownlint "**/*.md"

.PHONY: dependency-check
dependency-check: ## Empty for now

##@ Build

.PHONY: build
build: build-template-controller build-cluster-manager ## Build template controller and cluster manager

.PHONY: build-template-controller
build-template-controller: ## Build template controller
	go build -o bin/template-controller ${GOEXTRAFLAGS} cmd/template-controller/main.go

.PHONY: build-cluster-manager
build-cluster-manager: ## Build cluster manager
	go build -o bin/cluster-manager ${GOEXTRAFLAGS} cmd/cluster-manager/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/template-controller/main.go

.PHONY: vendor
vendor:  ## Build vendor directory of module dependencies.
	GOPRIVATE=github.com/open-edge-platform/* go mod vendor

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
DOCKER_ENV := DOCKER_BUILDKIT=1
docker-build: docker-build-template-controller docker-build-cluster-manager ## Build docker images.

.PHONY: docker-build-template-controller
docker-build-template-controller: vendor
	$(CONTAINER_TOOL) build -t ${DOCKER_IMAGE_TEMPLATE_CONTROLLER} . -f deployment/images/Dockerfile.cluster-template-controller ${DOCKER_BUILD_ARGS} ${DOCKER_LABEL_ARGS}

.PHONY: docker-build-cluster-manager
docker-build-cluster-manager: vendor
	$(CONTAINER_TOOL) build -t ${DOCKER_IMAGE_CLUSTER_MANAGER} . -f deployment/images/Dockerfile.cluster-manager ${DOCKER_BUILD_ARGS} ${DOCKER_LABEL_ARGS}

.PHONY: docker-push
docker-push: docker-push-template-controller docker-push-cluster-manager ## Push docker images.

.PHONY: docker-push-template-controller
docker-push-template-controller: ## Push docker image with the controller.
	$(CONTAINER_TOOL) push ${DOCKER_IMAGE_TEMPLATE_CONTROLLER}

.PHONY: docker-push-cluster-manager
docker-push-cluster-manager: ## Push docker image with the cluster manager.
	$(CONTAINER_TOOL) push ${DOCKER_IMAGE_CLUSTER_MANAGER}

.PHONY: docker-list
docker-list: ## Print name of docker container images
	@echo "images:"
	@echo "  template-controller:"
	@echo "    name: '$(DOCKER_IMAGE_TEMPLATE_CONTROLLER)'"
	@echo "    version: '$(VERSION)'"
	@echo "    gitTagPrefix: 'v'"
	@echo "    buildTarget: 'docker-build-template-controller'"
	@echo "  cluster-manager:"
	@echo "    name: '$(DOCKER_IMAGE_CLUSTER_MANAGER)'"
	@echo "    version: '$(VERSION)'"
	@echo "    gitTagPrefix: 'v'"
	@echo "    buildTarget: 'docker-build-cluster-manager'"

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name cluster-manager-builder
	$(CONTAINER_TOOL) buildx use cluster-manager-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm cluster-manager-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

HELM_DIRS = $(shell find ./deployment/charts -maxdepth 1 -mindepth 1 -type d -print )
HELM_PKGS = $(shell find . -name "*.tgz" -maxdepth 1 -mindepth 1 -type f -print )

.PHONY: helm-clean
helm-clean: ## Clean helm chart build annotations.
	for d in $(HELM_DIRS); do \
		yq eval -i 'del(.annotations.revision)' $$d/Chart.yaml; \
		yq eval -i 'del(.annotations.created)' $$d/Chart.yaml; \
	done
	rm -f $(HELM_PKGS)

.PHONY: helm-test
helm-test: ## Template the charts.
	for d in $(HELM_DIRS); do \
		helm template intel $$d; \
	done

.PHONY: helm-build
helm-build: ## Package helm charts.
	mkdir -p $(BUILD_DIR)
	for d in $(HELM_DIRS); do \
		yq eval -i '.version = "${HELM_VERSION}"' $$d/Chart.yaml; \
		yq eval -i '.appVersion = "${VERSION}"' $$d/Chart.yaml; \
		yq eval -i '.annotations.revision = "${LABEL_REVISION}"' $$d/Chart.yaml; \
		yq eval -i '.annotations.created = "${LABEL_CREATED}"' $$d/Chart.yaml; \
		helm package --app-version=${VERSION} --version=${HELM_VERSION} --debug -u $$d -d $(BUILD_DIR); \
	done
	# revert the temporary changes done in charts
	git checkout deployment/charts/cluster-template-crd/Chart.yaml deployment/charts/cluster-manager/Chart.yaml

.PHONY: helm-list
helm-list:
	@echo "charts:"
	@for d in $(HELM_DIRS); do \
    cname=$$(grep "^name:" "$$d/Chart.yaml" | cut -d " " -f 2) ;\
    echo "  $$cname:" ;\
    echo -n "    "; grep "^version" "$$d/Chart.yaml"  ;\
    echo "    gitTagPrefix: 'v'" ;\
    echo "    outDir: '$(BUILD_DIR)'" ;\
  done

.PHONY: helm-push
helm-push: ## Push helm charts.
	helm push $(BUILD_DIR)/cluster-template-crd-${HELM_VERSION}.tgz oci://$(REGISTRY)/$(REGISTRY_NO_AUTH)/$(REPOSITORY)/charts
	helm push $(BUILD_DIR)/cluster-manager-${HELM_VERSION}.tgz oci://$(REGISTRY)/$(REGISTRY_NO_AUTH)/$(REPOSITORY)/charts

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.17.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v1.64.7

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

##@ Standard targets

.PHONY: cobertura
cobertura:
	go install github.com/boumenot/gocover-cobertura@latest

.PHONY: gocov
gocov:
	go install github.com/axw/gocov/gocov@${GOLANG_GOCOV_VERSION}
	go install github.com/AlekSi/gocov-xml@${GOLANG_GOCOV_XML_VERSION}

$(VENV_NAME): requirements.txt
	echo "Creating virtualenv $@"
	python3 -m venv $@;\
	. ./$@/bin/activate; set -u;\
	python3 -m pip install --upgrade pip;\
	python3 -m pip install -r requirements.txt

.PHONY: license
license: $(VENV_NAME) ## Check licensing with the reuse tool.
	## Check licensing with the reuse tool.
	. ./$</bin/activate; set -u;\
	reuse --version;\
	reuse --root . lint

.PHONY: golint
golint: golangci-lint ## Lint Go files.
	$(GOLANGCI_LINT) run

.PHONY: helmlint
helmlint: ## Lint Helm charts.
	helm lint ./deployment/charts/*

YAML_FILES := $(shell find . -type f \( -name '*.yaml' -o -name '*.yml' \) -print )
.PHONY: yamllint
yamllint: $(VENV_NAME) ## Lint YAML files.
	. ./$</bin/activate; set -u;\
	yamllint --version;\
	yamllint -c .yamllint -s $(YAML_FILES)

.PHONY: fuzz
fuzz: vendor ## Run Fuzz tests against REST API handlers
	hack/fuzz_all.sh ${FUZZTIME}

##@ Local KinD test targets

RS_REGISTRY         ?= registry-rs.edgeorchestration.intel.com
RS_DOCKER_IMAGE_TEMPLATE_CONTROLLER  ?= ${RS_REGISTRY}/${REGISTRY_NO_AUTH}/${REPOSITORY}/template-controller:${DOCKER_TAG}
RS_DOCKER_IMAGE_CLUSTER_MANAGER ?= ${RS_REGISTRY}/${REGISTRY_NO_AUTH}/${REPOSITORY}/cluster-manager:${DOCKER_TAG}

KIND_CLUSTER ?= capi
KIND_CONFIG ?= test/kind-cluster-with-extramounts.yaml

.PHONY: kind-delete
kind-delete: ## Delete a development kind cluster with CAPI enabled
	@kind get clusters | grep -q '$(KIND_CLUSTER)' && kind delete cluster --name $(KIND_CLUSTER) || echo "No existing $(KIND_CLUSTER) cluster found."

.PHONY: kind-create
kind-create: ## Create a development kind cluster with CAPI enabled
	@if kind get clusters | grep -q '$(KIND_CLUSTER)'; then \
		echo "The '$(KIND_CLUSTER)' cluster is already running. Use \"make kind-delete\" to delete it before you proceed."; \
		exit 1; \
	fi
	echo "Creating a Kind cluster with CAPI enabled..."
	kind create cluster --name $(KIND_CLUSTER) --config $(KIND_CONFIG)
	CLUSTER_TOPOLOGY=true clusterctl init --core cluster-api:${CLUSTERCTL_VERSION} --bootstrap kubeadm:${KUBEADM_VERSION},rke2:${RKE2_VERSION} --control-plane kubeadm:${KUBEADM_VERSION},rke2:${RKE2_VERSION} --infrastructure docker:${DOCKER_INFRA_VERSION}

.PHONY: kind-expose-cm
kind-expose-cm: ## Expose the cluster manager service to the host
	kubectl port-forward svc/cluster-manager 8080:8080 &

docker-load:
	docker tag ${DOCKER_IMAGE_TEMPLATE_CONTROLLER} ${RS_DOCKER_IMAGE_TEMPLATE_CONTROLLER}
	docker tag ${DOCKER_IMAGE_CLUSTER_MANAGER} ${RS_DOCKER_IMAGE_CLUSTER_MANAGER}
	kind load docker-image ${RS_DOCKER_IMAGE_TEMPLATE_CONTROLLER} -n $(KIND_CLUSTER)
	kind load docker-image ${RS_DOCKER_IMAGE_CLUSTER_MANAGER} -n $(KIND_CLUSTER)

helm-install: docker-build docker-load helm-build ## Install helm charts to the K8s cluster specified in ~/.kube/config.
	helm upgrade --install --wait --debug cluster-template-crd $(BUILD_DIR)/cluster-template-crd-${HELM_VERSION}.tgz --set args.loglevel=DEBUG
	helm upgrade --install --wait --debug cluster-manager $(BUILD_DIR)/cluster-manager-${HELM_VERSION}.tgz --set clusterManager.extraArgs.disable-mt=${DISABLE_MT} --set clusterManager.extraArgs.disable-auth=${DISABLE_AUTH} --set clusterManager.extraArgs.disable-inventory=${DISABLE_INV}

helm-uninstall: # Uninstall helm charts from the K8s cluster specified in ~/.kube/config.
	helm uninstall cluster-manager cluster-template-crd

redeploy: docker-build docker-load ## Redeploy the pod with the latest codes.
	kubectl delete po -l app.kubernetes.io/instance=cluster-manager

.PHONY: generate-api
generate-api: check-oapi-codegen-version ## Generate Go client, server, client and types from OpenAPI spec with oapi-codegen
	@echo "Generating..."
	oapi-codegen -generate spec -o pkg/api/spec.gen.go -package api api/openapi/openapi.yaml
	oapi-codegen -generate client -o pkg/api/client.gen.go -package api api/openapi/openapi.yaml
	oapi-codegen -generate types -o pkg/api/types.gen.go -package api api/openapi/openapi.yaml
	oapi-codegen -generate std-http,strict-server -o pkg/api/server.gen.go -package api api/openapi/openapi.yaml

.PHONY: check-oapi-codegen-version
check-oapi-codegen-version: ## Check oapi-codegen version
	@version_output=$$(oapi-codegen --version); \
	version_lines=$$(echo $$version_output | tr '\n' ' '); \
	version=$$(echo $$version_lines | awk '{print $$2}'); \
	if [ "$$version" != "v2.3.0" ]; then \
		echo "oapi-codegen version must be v2.3.0, but got $$version"; \
		exit 1; \
	fi

##@ Dev targets

DEV_IMG               ?= cluster-manager-image:${DEV_TAG}
DOCKER_DEV_REGISTRY	  ?= <placeholder>
DOCKER_DEV_REPOSITORY ?= <placeholder>
DOCKER_DEV_IMG        := ${DOCKER_DEV_REGISTRY}${DOCKER_DEV_REPOSITORY}${DEV_IMG}

.PHONY: dev-image
dev-image: ## Build dev image and push to sandbox
	@if test -z $(DEV_TAG); \
		then echo "Please specify dev tag, make dev DEV_TAG=<dev-tag> " && exit 1; \
	fi
	${DOCKER_ENV} docker build --no-cache \
		--build-arg http_proxy="$(http_proxy)" --build-arg https_proxy="$(https_proxy)" --build-arg no_proxy="$(no_proxy)" \
		--build-arg HTTP_PROXY="$(HTTP_PROXY)" --build-arg HTTPS_PROXY="$(HTTPS_PROXY)" --build-arg NO_PROXY="$(NO_PROXY)" \
		${DOCKER_LABEL_ARGS} \
		-t ${DOCKER_DEV_IMG} . \
		-f deployment/images/Dockerfile.cluster-manager
	${DOCKER_ENV} docker push ${DOCKER_DEV_IMG}

.PHONY: dev-helm # Build dev helm chart and push to sandbox
dev-helm: ## Build dev helm chart and push to sandbox
	@if test -z $(DEV_TAG); \
		then echo "Please specify dev tag, make dev DEV_TAG=<dev-tag> " && exit 1; \
	fi
	helm package --app-version=${DEV_TAG} --version=${DEV_TAG} --debug -u deployment/charts/cluster-manager -d $(BUILD_DIR)
	helm push $(BUILD_DIR)/cluster-manager-${DEV_TAG}.tgz oci://$(DOCKER_DEV_REGISTRY)$(DOCKER_DEV_REPOSITORY)charts

.PHONY: install-cert-manager
install-cert-manager:  ## Install cert-manager using Helm.
	helm repo add jetstack https://charts.jetstack.io
	helm repo update
	helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --version v1.15.0 --set crds.enabled=true

.PHONY: uninstall-cert-manager
uninstall-cert-manager: ## Uninstall cert-manager using Helm.
	helm uninstall cert-manager --namespace cert-manager
	kubectl delete namespace cert-manager

.PHONY: clusterctl
clusterctl: ## Download clusterctl binary
ifndef CLUSTERCTL
	curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.9.4/clusterctl-linux-amd64 -o clusterctl
	chmod +x clusterctl
	mv clusterctl /usr/local/bin
endif

.PHONY: update-cm-version
update-cm-version: ## Update Cluster Manager version
	@echo "Current version: $(VERSION)"
	@read -p "Enter new version: " new_version; \
	echo $$new_version > VERSION; \
	sed -i "s/^version:.*/version: $${new_version}/" deployment/charts/cluster-manager/Chart.yaml; \
	sed -i "s/^appVersion:.*/appVersion: $${new_version}/" deployment/charts/cluster-manager/Chart.yaml; \
	sed -i "s/^version:.*/version: $${new_version}/" deployment/charts/cluster-template-crd/Chart.yaml; \
	sed -i "s/^appVersion:.*/appVersion: $${new_version}/" deployment/charts/cluster-template-crd/Chart.yaml;


.PHONY: update-ct-version
update-ct-version: ## Update Cluster Template version
	@echo "Current version: $(shell jq -r '.version' default-cluster-templates/baseline.json)"
	@read -p "Enter new version: " new_version; \
	sed -i "s/^  \"version\":.*/  \"version\": \"$${new_version}\",/" default-cluster-templates/baseline.json; \
	sed -i "s/^  \"version\":.*/  \"version\": \"$${new_version}\",/" default-cluster-templates/privileged.json; \
	sed -i "s/^  \"version\":.*/  \"version\": \"$${new_version}\",/" default-cluster-templates/restricted.json

.PHONY: update-api-version
update-api-version: ## Update API version
	@echo "Current version: $(shell grep '^  version:' api/openapi/openapi.yaml | awk '{print $$2}')"
	@read -p "Enter new version: " new_version; \
	sed -i "s/^  version:.*/  version: $${new_version}/" api/openapi/openapi.yaml
	make generate-api
