.PHONY: deps
deps:
	@go mod tidy
	@go mod verify

.PHONY: promu
promu:
	@which promu || scripts/install-promu.sh

GOPATH := $(shell go env GOPATH)
PREFIX ?= $(GOPATH)/bin

.PHONY: build
build: deps promu
	@echo ">> building binaries in $(PREFIX)"
	@$(GOPATH)/bin/promu build --prefix $(PREFIX)

.PHONY: crossbuild
crossbuild: promu
	@echo ">> crossbuilding all binaries"
	@$(GOPATH)/bin/promu crossbuild -v

include .busybox-versions

DOCKER_ARCHS       ?= amd64 arm64
BUILD_DOCKER_ARCHS := $(addprefix docker-build-,$(DOCKER_ARCHS))

.PHONY: docker-build
docker-build: $(BUILD_DOCKER_ARCHS)
$(BUILD_DOCKER_ARCHS): docker-build-%:
	@docker build -t "prombench-linux-$*" \
		--build-arg BASE_DOCKER_SHA="$($*)" \
		--build-arg ARCH="$*" \
		-f Dockerfile.multi-arch .

.PHONY: update-busybox
update-busybox:
	@scripts/update-busybox.sh

.PHONY: clean
clean:
	@rm -rf ./.build/
