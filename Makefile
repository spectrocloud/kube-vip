SHELL := /bin/sh

# The name of the executable (default is current directory name)
TARGET := kube-vip
.DEFAULT_GOAL := $(TARGET)

# Fips Flags
FIPS_ENABLE ?= ""

BUILDER_GOLANG_VERSION ?= 1.22
BUILD_ARGS = --build-arg CRYPTO_LIB=${FIPS_ENABLE} --build-arg BUILDER_GOLANG_VERSION=${BUILDER_GOLANG_VERSION}

RELEASE_LOC := release
ifeq ($(FIPS_ENABLE),yes)
  CGO_ENABLED := 1
  RELEASE_LOC := release-fips
endif

# Fips Flags
FIPS_ENABLE ?= ""

BUILDER_GOLANG_VERSION ?= 1.22
BUILD_ARGS = --build-arg CRYPTO_LIB=${FIPS_ENABLE} --build-arg BUILDER_GOLANG_VERSION=${BUILDER_GOLANG_VERSION}

RELEASE_LOC := release
ifeq ($(FIPS_ENABLE),yes)
  CGO_ENABLED := 1
  RELEASE_LOC := release-fips
endif

# These will be provided to the target
VERSION := v0.6.4
SPECTRO_VERSION ?= 4.0.0-dev
BUILD := `git rev-parse HEAD`

# Operating System Default (LINUX)
TARGETOS=linux

# Use linker flags to provide version/build settings to the target
LDFLAGS=-ldflags "-s -w -X=main.Version=$(VERSION) -X=main.Build=$(BUILD) -extldflags -static"
ifeq ($(FIPS_ENABLE),yes)
  LDFLAGS=-ldflags "-s -w -X=main.Version=$(VERSION) -X=main.Build=$(BUILD) -linkmode=external  -extldflags -static"
endif
DOCKERTAG ?= $(VERSION)
REPOSITORY ?= plndr

IMAGE_NAME := kube-vip
REGISTRY ?= gcr.io/spectro-dev-public/$(USER)/${RELEASE_LOC}
IMG_TAG ?= v0.6.4-spectro-${SPECTRO_VERSION}
IMG ?= ${REGISTRY}/${IMAGE_NAME}:${IMG_TAG}

RELEASE_REGISTRY := gcr.io/spectro-images-public/release/kube-vip
RELEASE_CONTROLLER_IMG := $(RELEASE_REGISTRY)/$(IMAGE_NAME)

IMAGE_NAME := kube-vip
REGISTRY ?= gcr.io/spectro-dev-public/$(USER)/${RELEASE_LOC}
IMG_TAG ?= v0.4.0-spectro-${SPECTRO_VERSION}
IMG ?= ${REGISTRY}/${IMAGE_NAME}:${IMG_TAG}

RELEASE_REGISTRY := gcr.io/spectro-images-public/release/kube-vip
RELEASE_CONTROLLER_IMG := $(RELEASE_REGISTRY)/$(IMAGE_NAME)

.PHONY: all build clean install uninstall fmt simplify check run e2e-tests

all: check install

$(TARGET):
	@go build $(LDFLAGS) -o $(TARGET)

build: $(TARGET)
	@true

clean:
	@rm -f $(TARGET)

install:
	@echo Building and Installing project
	@go install $(LDFLAGS)

uninstall: clean
	@rm -f $$(which ${TARGET})

fmt:
	@gofmt -l -w ./...

demo:
	@cd demo
	@docker buildx build  --platform linux/amd64,linux/arm64,linux/arm/v7,linux/ppc64le,linux/s390x --push -t ${IMG} .
	@echo New Multi Architecture Docker image created
	@cd ..

## Remote (push of images)
# This build a local docker image (x86 only) for quick testing

dockerx86Dev:
	@-rm ./kube-vip
	@docker buildx build  --platform linux/amd64 --push -t $(REPOSITORY)/$(TARGET):dev .
	@echo New single x86 Architecture Docker image created

dockerx86Iptables:
	@-rm ./kube-vip
	@docker buildx build  --platform linux/amd64 -f ./Dockerfile_iptables --push -t $(REPOSITORY)/$(TARGET):dev .
	@echo New single x86 Architecture Docker image created

dockerx86:
	@-rm ./kube-vip
	@docker buildx build --platform linux/amd64 --push -t ${IMG} .
	@echo New single x86 Architecture Docker image created

release-dockerx86:
	@-rm ./kube-vip
	@docker buildx build --platform linux/amd64 --push -t ${RELEASE_CONTROLLER_IMG} .
	@echo New single x86 Architecture Docker image created

docker:
	@-rm ./kube-vip
	@docker buildx build --build-arg CRYPTO_LIB=${FIPS_ENABLE} ${BUILD_ARGS} --push  --platform linux/amd64 --platform linux/arm64 -t ${IMG} .
	@echo New Multi Architecture Docker image created

## Local (docker load of images)
# This will build a local docker image (x86 only), use make dockerLocal for all architectures
dockerx86Local:
	@-rm ./kube-vip
	@docker buildx build  --platform linux/amd64 --load -t $(REPOSITORY)/$(TARGET):$(DOCKERTAG) .
	@echo New Multi Architecture Docker image created

dockerx86Action:
	@-rm ./kube-vip
	@docker buildx build  --platform linux/amd64 --load -t $(REPOSITORY)/$(TARGET):action .
	@echo New Multi Architecture Docker image created

dockerx86ActionIPTables:
	@-rm ./kube-vip
	@docker buildx build  --platform linux/amd64 -f ./Dockerfile_iptables --load -t $(REPOSITORY)/$(TARGET):action .
	@echo New Multi Architecture Docker image created

dockerLocal:
	@-rm ./kube-vip
	@docker buildx build  --platform linux/amd64,linux/arm64,linux/arm/v7,linux/ppc64le,linux/s390x --load -t $(REPOSITORY)/$(TARGET):$(DOCKERTAG) .
	@echo New Multi Architecture Docker image created

simplify:
	@gofmt -s -l -w ./...

check:
	go mod tidy
	test -z "$(git status --porcelain)"
	test -z $(shell gofmt -l main.go | tee /dev/stderr) || echo "[WARN] Fix formatting issues with 'make fmt'"
	golangci-lint run
	go vet ./...

run: install
	@$(TARGET)

manifests:
	@make build
	@mkdir -p ./docs/manifests/$(VERSION)/
	@./kube-vip manifest pod --interface eth0 --vip 192.168.0.1 --arp --leaderElection --controlplane --services > ./docs/manifests/$(VERSION)/kube-vip-arp.yaml
	@./kube-vip manifest pod --interface eth0 --vip 192.168.0.1 --arp --leaderElection --controlplane --services --enableLoadBalancer > ./docs/manifests/$(VERSION)/kube-vip-arp-lb.yaml
	@./kube-vip manifest pod --interface eth0 --vip 192.168.0.1 --bgp --controlplane --services > ./docs/manifests/$(VERSION)/kube-vip-bgp.yaml
	@./kube-vip manifest daemonset --interface eth0 --vip 192.168.0.1 --arp --leaderElection --controlplane --services --inCluster > ./docs/manifests/$(VERSION)/kube-vip-arp-ds.yaml
	@./kube-vip manifest daemonset --interface eth0 --vip 192.168.0.1 --arp --leaderElection --controlplane --services --inCluster --enableLoadBalancer > ./docs/manifests/$(VERSION)/kube-vip-arp-ds-lb.yaml
	@./kube-vip manifest daemonset --interface eth0 --vip 192.168.0.1 --bgp --leaderElection --controlplane --services --inCluster > ./docs/manifests/$(VERSION)/kube-vip-bgp-ds.yaml
	@./kube-vip manifest daemonset --interface eth0 --vip 192.168.0.1 --bgp --leaderElection --controlplane --services --inCluster --provider-config /etc/cloud-sa/cloud-sa.json > ./docs/manifests/$(VERSION)/kube-vip-bgp-em-ds.yaml
	@-rm ./kube-vip

unit-tests:
	go test ./...

integration-tests:
	go test -tags=integration,e2e -v ./pkg/etcd

e2e-tests:
	E2E_IMAGE_PATH=$(REPOSITORY)/$(TARGET):$(DOCKERTAG) go run github.com/onsi/ginkgo/v2/ginkgo --tags=e2e -v -p ./testing/e2e ./testing/e2e/etcd

service-tests:
	E2E_IMAGE_PATH=$(REPOSITORY)/$(TARGET):$(DOCKERTAG) go run ./testing/e2e/services -Services

trivy: dockerx86ActionIPTables
	docker run -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:0.47.0 \
		image  \
		--format table \
		--exit-code  1 \
		--ignore-unfixed \
		--vuln-type  'os,library' \
		--severity  'CRITICAL,HIGH'  \
		$(REPOSITORY)/$(TARGET):action

