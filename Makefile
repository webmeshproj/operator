
# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/webmeshproj/operator:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.26.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL ?= /usr/bin/env bash -o pipefail
.SHELLFLAGS ?= -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

lint: ## Run linters.
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run

##@ Build

NAME        := operator
VERSION_PKG := github.com/webmeshproj/$(NAME)/controllers/version
VERSION     := $(shell git describe --tags --always --dirty)
COMMIT      := $(shell git rev-parse HEAD)
DATE        := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     ?= -s -w -extldflags=-static \
			   -X $(VERSION_PKG).Version=$(VERSION) \
			   -X $(VERSION_PKG).Commit=$(COMMIT) \
			   -X $(VERSION_PKG).BuildDate=$(DATE)
BUILD_TAGS  ?= osusergo,netgo,sqlite_omit_load_extension

ARCH      ?= $(shell go env GOARCH)
OS        ?= $(shell go env GOOS)
DIST      := $(CURDIR)/dist

.PHONY: build
build: fmt vet generate ## Build operator binary.
	go build \
		-tags "$(BUILD_TAGS)" \
		-ldflags "$(LDFLAGS)" \
		-o "$(DIST)/$(NAME)_$(OS)_$(ARCH)" \
		main.go

BUILD_IMAGE ?= ghcr.io/webmeshproj/operator-build:latest
build-image: ## Build the operator build image.
	docker buildx build -t $(BUILD_IMAGE) -f Dockerfile.build --load .

.PHONY: dist
dist: ## Build operator binaries for all platforms in the Docker build image.
	mkdir -p $(DIST)
	docker run --rm \
		-u $(shell id -u):$(shell id -g) \
		-v "$(CURDIR):/build" \
		-v "$(shell go env GOCACHE):/.cache/go-build" \
		-v "$(shell go env GOPATH):/go" \
		-e GOPATH=/go \
		-w /build \
		$(BUILD_IMAGE) make -j $(shell nproc) dist-operator SHELL=ash

dist-operator: ## Build operator binaries for all platforms.
	$(MAKE) \
		dist-operator-linux-amd64 \
		dist-operator-linux-arm64 \
		dist-operator-linux-arm

dist-operator-linux-amd64:
	$(call dist-build,linux,amd64,x86_64-linux-musl-gcc)

dist-operator-linux-arm64:
	$(call dist-build,linux,arm64,aarch64-linux-musl-gcc)

dist-operator-linux-arm:
	$(call dist-build,linux,arm,arm-linux-musleabihf-gcc)

define dist-build
	CGO_ENABLED=1 GOOS=$(1) GOARCH=$(2) CC=$(3) \
		go build \
			-tags "$(BUILD_TAGS)" \
			-ldflags "$(LDFLAGS)" \
			-trimpath \
			-o "$(DIST)/$(NAME)_$(1)_$(2)" \
			main.go
endef

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: docker-build
docker-build: dist ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.11.1

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

##@ Local Development

CLUSTER_NAME ?= webmesh-operator
KUBECONTEXT  ?= k3d-$(CLUSTER_NAME)
KUBECTL 	 ?= kubectl --context=$(KUBECONTEXT)

run-k3d: create-k3d import-k3d install-cert-manager

create-k3d:
	k3d cluster create $(CLUSTER_NAME) \
		--port "8443:8443@loadbalancer" \
		--port "51820:51820/udp@loadbalancer" \
		--k3s-arg "--disable=traefik@server:0" \
		--servers 1

import-k3d:
	k3d image import --cluster $(CLUSTER_NAME) $(IMG)

CERT_MANAGER_VERSION ?= v1.12.0
install-cert-manager:
	$(KUBECTL) apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_VERSION)/cert-manager.yaml

install-operator: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

uninstall-operator: kustomize
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete -f -

restart-operator:
	$(KUBECTL) --namespace webmesh-system \
		rollout restart deployment/operator-controller-manager

destroy-k3d:
	k3d cluster delete $(CLUSTER_NAME)

CONFIG ?= $(HOME)/.wmctl/config.yaml
get-config:
	$(KUBECTL) get secret mesh-sample-admin-config -o json \
		| jq '.data | map_values(@base64d)' \
		| jq -r '.["config.yaml"]' > $(CONFIG)