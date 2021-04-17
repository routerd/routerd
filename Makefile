# Copyright 2021 The routerd authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

SHELL=/bin/bash
.SHELLFLAGS=-euo pipefail -c

export CGO_ENABLED:=0

BRANCH=$(shell git rev-parse --abbrev-ref HEAD)
SHORT_SHA=$(shell git rev-parse --short HEAD)
VERSION?=${BRANCH}-${SHORT_SHA}
BUILD_DATE=$(shell date +%s)
IMAGE_ORG?=quay.io/routerd
MODULE=routerd.net/routerd
LD_FLAGS=-X $(MODULE)/internal/version.Version=$(VERSION) -X $(MODULE)/internal/version.Branch=$(BRANCH) -X $(MODULE)/internal/version.Commit=$(SHORT_SHA) -X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# -------
# Compile
# -------

all: \
	bin/linux_amd64/routerd \
	bin/linux_amd64/routerd-dhcp \
	bin/linux_amd64/routerd-dns

bin/linux_amd64/%: GOARGS = GOOS=linux GOARCH=amd64

bin/%: FORCE
	$(eval COMPONENT=$(shell basename $*))
	$(GOARGS) go build -ldflags "-w $(LD_FLAGS)" -o bin/$* cmd/$(COMPONENT)/main.go

FORCE:

clean:
	rm -rf bin/$*
.PHONY: clean

# ----------
# Deployment
# ----------

# Run against the configured Kubernetes cluster in ~/.kube/config or $KUBECONFIG
run: generate fmt vet manifests
	go run -ldflags "-w $(LD_FLAGS)" ./cmd/routerd/main.go -pprof-addr="localhost:8065"
.PHONY: run

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -
.PHONY: install

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -
.PHONY: uninstall

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMAGE_ORG}/routerd:${VERSION}
	kustomize build config/default | kubectl apply -f -
.PHONY: deploy

# Remove controller in the configured Kubernetes cluster in ~/.kube/config
remove: manifests
	cd config/manager && kustomize edit set image controller=${IMAGE_ORG}/routerd:${VERSION}
	kustomize build config/default | kubectl delete -f -
.PHONY: remove

# ----------
# Generators
# ----------

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) crd:crdVersions=v1 rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# -------------------
# Testing and Linting
# -------------------

test: generate fmt vet manifests
	CGO_ENABLED=1 go test -race -v ./...
.PHONY: test

ci-test: test
	hack/validate-directory-clean.sh
.PHONY: ci-test

fmt:
	go fmt ./...
.PHONY: fmt

vet:
	go vet ./...
.PHONY: vet

tidy:
	go mod tidy
.PHONY: tidy

verify-boilerplate:
	@go run hack/boilerplate/boilerplate.go \
		-boilerplate-dir hack/boilerplate/ \
		-verbose
.PHONY: verify-boilerplate

pre-commit-install:
	@echo "installing pre-commit hooks using https://pre-commit.com/"
	@pre-commit install
.PHONY: pre-commit-install

# ----------------
# Container Images
# ----------------

build-images: \
	build-image-radvd \
	build-image-routerd \
	build-image-routerd-dhcp \
	build-image-routerd-dns
.PHONY: build-images

push-images: \
	push-image-radvd \
	push-image-routerd \
	push-image-routerd-dhcp \
	push-image-routerd-dns
.PHONY: push-images

build-image-radvd:
	@mkdir -p bin/image/radvd
	@cp -a config/docker/radvd.Dockerfile bin/image/radvd/Dockerfile
	@echo building ${IMAGE_ORG}/radvd:${VERSION}
	@docker build -t ${IMAGE_ORG}/radvd:${VERSION} bin/image/radvd

.SECONDEXPANSION:
build-image-%: bin/linux_amd64/$$*
	@rm -rf bin/image/$*
	@mkdir -p bin/image/$*
	@cp -a bin/linux_amd64/$* bin/image/$*
	@cp -a config/docker/$*.Dockerfile bin/image/$*/Dockerfile
	@echo building ${IMAGE_ORG}/$*:${VERSION}
	@docker build -t ${IMAGE_ORG}/$*:${VERSION} bin/image/$*

push-image-%: build-image-$$*
	@docker push ${IMAGE_ORG}/$*:${VERSION}
	@echo pushed ${IMAGE_ORG}/$*:${VERSION}
