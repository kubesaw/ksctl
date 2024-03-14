.PHONY: clean
clean: clean-bin
	$(Q)-rm -rf ${V_FLAG} ./vendor
	$(Q)-rm -rf ($COV_DIR)
	$(Q)go clean ${X_FLAG} ./...

.PHONY: clean-bin
clean-bin:
	@rm -rf $(BIN_DIR) 2>/dev/null || true
