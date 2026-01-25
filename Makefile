# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Runtime image for PromptKit runtime container
RUNTIME_IMG ?= omnia-runtime:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

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

.PHONY: setup
setup: ## Set up development environment (install git hooks)
	@echo "Installing git hooks..."
	@ln -sf ../../hack/pre-commit .git/hooks/pre-commit
	@chmod +x hack/pre-commit
	@echo "Git hooks installed successfully"

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./api/..." paths="./cmd/..." paths="./internal/..." paths="./pkg/..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./api/..." paths="./cmd/..." paths="./internal/..." paths="./pkg/..."

.PHONY: manifests-all
manifests-all: manifests manifests-ee ## Generate manifests for core and enterprise.

.PHONY: generate-all
generate-all: generate generate-ee ## Generate code for core and enterprise.

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: generate-proto
generate-proto: protoc-gen-go protoc-gen-go-grpc ## Generate Go code from proto files.
	@echo "Generating Go code from proto files..."
	@mkdir -p pkg/runtime/v1 pkg/tools/v1
	PATH="$(LOCALBIN):$$PATH" "$(PROTOC)" \
		--go_out=. \
		--go_opt=module=github.com/altairalabs/omnia \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/altairalabs/omnia \
		api/proto/runtime/v1/runtime.proto \
		api/proto/tools/v1/tools.proto
	@echo "Proto generation complete."

.PHONY: generate-proto-ts
generate-proto-ts: ## Generate TypeScript types from proto files.
	@echo "Generating TypeScript types from proto files..."
	cd dashboard && npm run generate:proto
	@echo "TypeScript proto generation complete."

.PHONY: update-schema
update-schema: ## Fetch latest PromptPack schema for embedded fallback.
	@./hack/update-schema.sh

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	# Exclude e2e tests (require Kind cluster)
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-junit
test-junit: manifests generate fmt vet setup-envtest gotestsum ## Run tests with JUnit XML output.
	# Exclude e2e tests (require Kind cluster)
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" \
		$(GOTESTSUM) --junitfile test-results.xml --format testdox -- \
		$$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-integration
test-integration: manifests generate fmt vet ## Run integration tests (facade-runtime gRPC communication).
	go test -tags=integration ./test/integration/... -v -timeout 5m

.PHONY: test-integration-run
test-integration-run: ## Run a specific integration test by name (use TEST=TestName).
	go test -tags=integration ./test/integration/... -v -run $(TEST) -timeout 5m

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= omnia-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v -timeout 20m
	$(MAKE) cleanup-test-e2e

.PHONY: test-e2e-manager
test-e2e-manager: setup-test-e2e manifests generate fmt vet ## Run only Manager-labeled e2e tests.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter=manager -timeout 20m
	$(MAKE) cleanup-test-e2e

.PHONY: test-e2e-crds
test-e2e-crds: setup-test-e2e manifests generate fmt vet ## Run only CRDs-labeled e2e tests.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v -ginkgo.label-filter=crds -timeout 20m
	$(MAKE) cleanup-test-e2e

.PHONY: test-e2e-junit
test-e2e-junit: setup-test-e2e manifests generate fmt vet ## Run e2e tests with JUnit XML output.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test -tags=e2e ./test/e2e/ -v -ginkgo.v \
		-ginkgo.junit-report=e2e-results.xml \
		-ginkgo.show-node-events \
		-ginkgo.poll-progress-after=30s \
		-timeout 20m
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Release

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	helm lint charts/omnia

.PHONY: helm-template
helm-template: ## Test Helm chart template rendering
	helm template omnia charts/omnia --debug

.PHONY: helm-package
helm-package: ## Package Helm chart locally
	helm dependency update charts/omnia
	mkdir -p dist
	helm package charts/omnia --destination dist/

.PHONY: helm-push
helm-push: helm-package ## Package and push Helm chart to GHCR (requires VERSION)
ifndef VERSION
	$(error VERSION is required. Usage: make helm-push VERSION=0.2.0)
endif
	@echo "Pushing omnia-$(VERSION).tgz to GHCR..."
	helm push dist/omnia-$(VERSION).tgz oci://ghcr.io/altairalabs/charts

.PHONY: release-dry-run
release-dry-run: ## Dry run of release process (local validation)
	@echo "==> Validating Chart.yaml"
	@helm lint charts/omnia
	@echo "==> Building Docker images"
	@$(MAKE) docker-build
	@echo "==> Packaging Helm chart"
	@$(MAKE) helm-package
	@echo "==> Building documentation"
	@cd docs && npm ci && npm run build
	@echo "Release dry run complete"

.PHONY: version-bump
version-bump: ## Update chart version (requires VERSION)
ifndef VERSION
	$(error VERSION is required. Usage: make version-bump VERSION=0.2.0)
endif
	@echo "Current version: $$(yq eval '.version' charts/omnia/Chart.yaml)"
	@yq eval -i ".version = \"$(VERSION)\"" charts/omnia/Chart.yaml
	@yq eval -i ".appVersion = \"$(VERSION)\"" charts/omnia/Chart.yaml
	@echo "Updated to $(VERSION)"

##@ Documentation

.PHONY: docs-install
docs-install: ## Install documentation dependencies
	cd docs && npm ci

.PHONY: docs-build
docs-build: ## Build documentation site
	cd docs && npm run build

.PHONY: docs-dev
docs-dev: ## Run documentation site in development mode
	cd docs && npm run dev

.PHONY: docs-serve
docs-serve: docs-install ## Build docs and serve locally (preview)
	cd docs && npm run build && npm run preview

##@ Dashboard

DASHBOARD_IMG ?= omnia-dashboard:latest

.PHONY: dashboard-install
dashboard-install: ## Install dashboard dependencies
	cd dashboard && npm ci

.PHONY: dashboard-dev
dashboard-dev: ## Run dashboard in development mode
	cd dashboard && npm run dev

.PHONY: dashboard-build
dashboard-build: ## Build dashboard for production
	cd dashboard && npm run build

.PHONY: dashboard-lint
dashboard-lint: ## Run ESLint on dashboard
	cd dashboard && npm run lint

.PHONY: dashboard-typecheck
dashboard-typecheck: ## Run TypeScript type checking on dashboard
	cd dashboard && npm run typecheck

.PHONY: dashboard-check
dashboard-check: dashboard-lint dashboard-typecheck ## Run all dashboard checks (lint + typecheck)

.PHONY: docker-build-dashboard
docker-build-dashboard: ## Build docker image for the dashboard
	$(CONTAINER_TOOL) build -t ${DASHBOARD_IMG} ./dashboard

.PHONY: sync-chart-crds
sync-chart-crds: manifests manifests-ee ## Sync CRDs from config/crd/bases to charts/omnia/crds
	# Copy core CRDs (excluding enterprise arena CRDs which are conditional templates)
	cp config/crd/bases/omnia.altairalabs.ai_agentruntimes.yaml charts/omnia/crds/
	cp config/crd/bases/omnia.altairalabs.ai_promptpacks.yaml charts/omnia/crds/
	cp config/crd/bases/omnia.altairalabs.ai_providers.yaml charts/omnia/crds/
	cp config/crd/bases/omnia.altairalabs.ai_toolregistries.yaml charts/omnia/crds/
	cp config/crd/bases/omnia.altairalabs.ai_workspaces.yaml charts/omnia/crds/
	# Sync enterprise CRDs to conditional templates (wrapped with enterprise.enabled check)
	@echo "Syncing enterprise CRDs to conditional templates..."
	@for f in config/crd/bases/omnia.altairalabs.ai_arena*.yaml; do \
		base=$$(basename $$f); \
		echo "{{- if .Values.enterprise.enabled }}" > charts/omnia/templates/enterprise/$$base; \
		cat $$f >> charts/omnia/templates/enterprise/$$base; \
		echo "{{- end }}" >> charts/omnia/templates/enterprise/$$base; \
	done

.PHONY: generate-dashboard-types
generate-dashboard-types: sync-chart-crds ## Generate TypeScript types from CRD schemas
	node scripts/generate-dashboard-types.js

.PHONY: generate-dashboard-api
generate-dashboard-api: ## Generate TypeScript API client from OpenAPI spec
	cd dashboard && npm run generate:api

.PHONY: generate-all
generate-all: manifests generate generate-proto generate-proto-ts sync-chart-crds generate-dashboard-types generate-dashboard-api ## Run all code generation
	@echo "All code generation complete."

##@ Local Development (Tilt)

.PHONY: dev
dev: ## Start local development cluster with Tilt
	tilt up

.PHONY: dev-ollama
dev-ollama: ## Start local development with Ollama enabled (local LLM)
	@echo "Starting Tilt with Ollama enabled..."
	@echo "Note: Ollama requires 8GB+ RAM and will pull llava:7b (~4GB) on first run"
	ENABLE_OLLAMA=true tilt up

.PHONY: dev-down
dev-down: ## Stop local development cluster
	tilt down

.PHONY: dev-demo
dev-demo: ## Start development with demo mode (Ollama + vision agent)
	@echo "Starting Tilt with demo mode..."
	ENABLE_OLLAMA=true tilt up

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-runtime
build-runtime: fmt vet ## Build runtime binary.
	go build -o bin/runtime ./cmd/runtime

##@ Enterprise Edition

.PHONY: build-arena-controller
build-arena-controller: manifests-ee generate-ee fmt vet ## Build Arena controller binary (Enterprise).
	go build -o bin/arena-controller ./ee/cmd/omnia-arena-controller

.PHONY: build-arena-worker
build-arena-worker: fmt vet ## Build Arena worker binary (Enterprise).
	go build -o bin/arena-worker ./ee/cmd/arena-worker

.PHONY: manifests-ee
manifests-ee: controller-gen ## Generate CRDs for Enterprise types.
	"$(CONTROLLER_GEN)" rbac:roleName=arena-manager-role crd webhook paths="./ee/api/..." paths="./ee/internal/..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate-ee
generate-ee: controller-gen ## Generate code for Enterprise types.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate-ee.go.txt" paths="./ee/api/..." paths="./ee/internal/..." paths="./ee/pkg/..."

.PHONY: docker-build-arena-controller
docker-build-arena-controller: ## Build docker image for Arena controller (Enterprise).
	$(CONTAINER_TOOL) build -t arena-controller:latest -f ee/Dockerfile.arena-controller .

.PHONY: docker-build-arena-worker
docker-build-arena-worker: ## Build docker image for Arena worker (Enterprise).
	$(CONTAINER_TOOL) build -t arena-worker:latest -f ee/Dockerfile.arena-worker .

.PHONY: build-all
build-all: build build-runtime build-arena-controller build-arena-worker ## Build all binaries (core + enterprise).

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-build-runtime
docker-build-runtime: ## Build docker image for the runtime (PromptKit).
	go mod vendor
	$(CONTAINER_TOOL) build -t ${RUNTIME_IMG} -f Dockerfile.runtime .
	rm -rf vendor

.PHONY: docker-push-runtime
docker-push-runtime: ## Push docker image for the runtime.
	$(CONTAINER_TOOL) push ${RUNTIME_IMG}

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
	- $(CONTAINER_TOOL) buildx create --name omnia-builder
	$(CONTAINER_TOOL) buildx use omnia-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm omnia-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
PROTOC ?= protoc
PROTOC_GEN_GO ?= $(LOCALBIN)/protoc-gen-go
PROTOC_GEN_GO_GRPC ?= $(LOCALBIN)/protoc-gen-go-grpc
GOTESTSUM ?= $(LOCALBIN)/gotestsum

## Tool Versions
KUSTOMIZE_VERSION ?= v5.7.1
CONTROLLER_TOOLS_VERSION ?= v0.19.0
PROTOC_GEN_GO_VERSION ?= v1.36.6
PROTOC_GEN_GO_GRPC_VERSION ?= v1.5.1
GOTESTSUM_VERSION ?= v1.13.0

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.5.0
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
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
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
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: protoc-gen-go
protoc-gen-go: $(PROTOC_GEN_GO) ## Download protoc-gen-go locally if necessary.
$(PROTOC_GEN_GO): $(LOCALBIN)
	$(call go-install-tool,$(PROTOC_GEN_GO),google.golang.org/protobuf/cmd/protoc-gen-go,$(PROTOC_GEN_GO_VERSION))

.PHONY: protoc-gen-go-grpc
protoc-gen-go-grpc: $(PROTOC_GEN_GO_GRPC) ## Download protoc-gen-go-grpc locally if necessary.
$(PROTOC_GEN_GO_GRPC): $(LOCALBIN)
	$(call go-install-tool,$(PROTOC_GEN_GO_GRPC),google.golang.org/grpc/cmd/protoc-gen-go-grpc,$(PROTOC_GEN_GO_GRPC_VERSION))

.PHONY: gotestsum
gotestsum: $(GOTESTSUM) ## Download gotestsum locally if necessary.
$(GOTESTSUM): $(LOCALBIN)
	$(call go-install-tool,$(GOTESTSUM),gotest.tools/gotestsum,$(GOTESTSUM_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
