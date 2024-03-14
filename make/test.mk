############################################################
#
# (local) Tests
#
############################################################

.PHONY: test
## runs the tests without coverage and excluding E2E tests
test:
	@echo "running the tests without coverage and excluding E2E tests..."
	$(Q)go test ${V_FLAG} -parallel 1 -failfast ./...

############################################################
#
# OpenShift CI Tests with Coverage
#
############################################################

# Output directory for coverage information
COV_DIR = out/coverage

.PHONY: test-with-coverage
## runs the tests with coverage
test-with-coverage:
	@echo "running the tests with coverage..."
	@-mkdir -p $(COV_DIR)
	@-rm $(COV_DIR)/coverage.txt
	$(Q)go test -parallel=1 -vet off ${V_FLAG} -coverprofile=$(COV_DIR)/coverage.txt -covermode=atomic ./...
