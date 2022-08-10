SHELL=/bin/bash
# Image URL to use all building/pushing image targets
VER_LABEL=$(shell ./get-label.bash)
BUILD_NUMBER ?= 1
IMG ?= platform9/luigi-plugins:$(VER_LABEL)-$(BUILD_NUMBER)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
SRCROOT = $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/)
BUILD_DIR :=$(SRCROOT)/bin
OS=$(shell go env GOOS)
ARCH=$(shell go env GOARCH)

$(BUILD_DIR):
	mkdir -p $@

all: manager

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager cmd/luigi-plugins/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./cmd/luigi-plugins/main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test
	docker build --network host . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen: pre-reqs
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

pre-reqs:
	curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v2.3.1/kubebuilder_2.3.1_$(OS)_$(ARCH).tar.gz | tar -xz -C /tmp/
	mv /tmp/kubebuilder_2.3.1_$(OS)_$(ARCH) /usr/local/kubebuilder
	export PATH=$PATH:/usr/local/kubebuilder/bin

img-test:
	docker run --rm  -v $(SRCROOT):/luigi -w /luigi golang:1.17.7-bullseye  bash -c "make test"

img-build: img-test $(BUILD_DIR)
	docker build --network host . -t ${IMG}
	echo ${IMG} > $(BUILD_DIR)/container-tag

img-build-push: img-build
	docker login
	docker push ${IMG}
	echo ${IMG} > $(BUILD_DIR)/container-tag
	
