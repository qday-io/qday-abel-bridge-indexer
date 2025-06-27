BUILDDIR ?= $(CURDIR)/build
NAMESPACE := ghcr.io/qday-io
PROJECT := qday-abel-bridge-indexer
DOCKER_IMAGE := $(NAMESPACE)/$(PROJECT)
#COMMIT_HASH := $(shell git rev-parse --short=7 HEAD)
#DATE=$(shell date +%Y%m%d-%H%M%S)
DOCKER_TAG := 0.2.2
MODULES := $(wildcard api/*)
SYSTEM := $(shell uname -s)

###############################################################################
###                                  Build                                  ###
###############################################################################


BUILD_TARGETS := build install

build: BUILD_ARGS=-o $(BUILDDIR)/
build-linux:
	GOOS=linux GOARCH=amd64 LEDGER_ENABLED=false $(MAKE) build

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/


image-build:
	docker build -t ${DOCKER_IMAGE}:${DOCKER_TAG} .

image-build-linux:
	docker build --platform=linux/amd64 -t ${DOCKER_IMAGE}:${DOCKER_TAG} .
	
image-push:
	docker push --all-tags ${DOCKER_IMAGE}

image-list:
	docker images | grep ${DOCKER_IMAGE}

$(MOCKS_DIR):
	mkdir -p $(MOCKS_DIR)

distclean: clean tools-clean

clean:
	rm -rf \
    $(BUILDDIR)/ \
    artifacts/ \
    tmp-swagger-gen/

all: build

build-all: tools build lint test vulncheck

.PHONY: distclean clean build-all

proto:
	$(foreach module,$(MODULES),$(call protoc,$(module)))

define protoc
	echo "module: $(1)"
    protoc --version
	protoc -I.:${PROTO_INCLUDE}\
			--go_out=. --go_opt=paths=source_relative \
			--go-grpc_out=. --go-grpc_opt=paths=source_relative \
			--grpc-gateway_out=logtostderr=true:. \
			--grpc-gateway_opt paths=source_relative \
			--experimental_allow_proto3_optional \
			$(1)/*/*.proto
	# echo "end"
    protoc -I.:${PROTO_INCLUDE}\
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=logtostderr=true:. \
		--grpc-gateway_opt paths=source_relative \
		--experimental_allow_proto3_optional \
		--swagger_out=logtostderr=true:. \
		$(1)/*.proto
	# echo "end"
endef

proto-clean:
	rm -rvf api/protobuf/vo/*.pb.go
	rm -rvf api/protobuf/*.pb.go
	rm -rvf api/protobuf/*.pb.gw.go
	rm -rvf api/protobuf/*.swagger.json

###############################################################################
###                                Linting                                  ###
###############################################################################

lint:
	@@test -n "$$golangci-lint version | awk '$4 >= 1.42')"
	golangci-lint run --out-format=tab -n

format:
	find . -name '*.go' -type f -not -path "./vendor*" -not -path "*.git*"  -not -name '*.pb.go' -not -name '*.pb.gw.go' | xargs gofumpt -d -e -extra

lint-fix:
	golangci-lint run --fix --out-format=tab --issues-exit-code=0
.PHONY: lint lint-fix lint-py

format-fix:
	find . -name '*.go' -type f -not -path "./vendor*" -not -path "*.git*" -not -name '*.pb.go' -not -name '*.pb.gw.go' | xargs gofumpt -w -s
	find . -name '*.go' -type f -not -path "./vendor*" -not -path "*.git*"  -not -name '*.pb.go' -not -name '*.pb.gw.go' | xargs misspell -w
.PHONY: format

###############################################################################
###                           Tests                                         ###
###############################################################################

PACKAGES_UNIT=$(shell go list ./...)
TEST_PACKAGES=./...
TEST_TARGETS := test-unit
SKIP_TEST_METHOD='(^TestLocal)'

test:
ifneq (,$(shell which tparse 2>/dev/null))
	go test -skip=$(SKIP_TEST_METHOD)  -mod=readonly  -json $(ARGS) $(EXTRA_ARGS) $(TEST_PACKAGES)  | tparse
else

	go test -skip=$(SKIP_TEST_METHOD) -mod=readonly $(ARGS)   $(EXTRA_ARGS) $(TEST_PACKAGES)
endif

.PHONY: test $(TEST_TARGETS)


test-local:
ifneq (,$(shell which tparse 2>/dev/null))
	go test  -mod=readonly  -json $(ARGS) $(EXTRA_ARGS) $(TEST_PACKAGES)  | tparse
else
	go test  -mod=readonly $(ARGS)   $(EXTRA_ARGS) $(TEST_PACKAGES)
endif

.PHONY: test-local $(TEST_TARGETS)