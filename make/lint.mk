.PHONY: lint
## Runs linters on Go code files and YAML files
lint: lint-go-code lint-yaml

.PHONY: lint-yaml
## runs yamllint on all yaml files
lint-yaml:
	$(Q)yamllint -c .yamllint ./

.PHONY: lint-go-code
# Checks the code with golangci-lint
lint-go-code:
ifeq (, $(shell which golangci-lint 2>/dev/null))
	$(error "golangci-lint not found in PATH. Please install it using instructions on https://golangci-lint.run/usage/install/#local-installation")
endif
	golangci-lint ${V_FLAG} run -c .golangci.yml
