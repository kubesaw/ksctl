GO_PACKAGE_ORG_NAME ?= kubesaw
GO_PACKAGE_REPO_NAME ?= ksctl
GO_PACKAGE_PATH ?= github.com/${GO_PACKAGE_ORG_NAME}/${GO_PACKAGE_REPO_NAME}

BIN_DIR := bin
.PHONY: build
## Build the operator
build: GO_COMMAND=build
build: GO_EXTRA_FLAGS=-o $(BIN_DIR)/
build: clean-bin run-go

.PHONY: install
## installs the binary executable
install: GO_COMMAND=install
install: run-go

run-go:
	$(Q)CGO_ENABLED=0 \
		go ${GO_COMMAND} ${V_FLAG} \
		-ldflags "-X ${GO_PACKAGE_PATH}/pkg/version.Commit=${GIT_COMMIT_ID} -X ${GO_PACKAGE_PATH}/pkg/version.BuildTime=${BUILD_TIME}" \
        ${GO_EXTRA_FLAGS} ${GO_PACKAGE_PATH}/cmd/ksctl/...

.PHONY: verify-dependencies
## Runs commands to verify after the updated dependecies of toolchain-common/API(go mod replace), if the repo needs any changes to be made
verify-dependencies: tidy vet build test lint

.PHONY: tidy
tidy: 
	go mod tidy

.PHONY: vet
vet:
	go vet ./...
	
