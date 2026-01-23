.PHONY: test-py test-go clean

TEST_DIR := /tmp/wt-test-$$$$
SAVED_WT_ROOT := $(WT_ROOT)

test-py:
	@echo "Setting up test environment for Python..."
	@mkdir -p $(TEST_DIR)
	@export WT_ROOT=$(TEST_DIR) && \
		export WT_CMD="python3 wt.py" && \
		export WT_SKIP_PROMPTS=1 && \
		./wt-test.sh; \
		TEST_EXIT=$$?; \
		rm -rf $(TEST_DIR); \
		exit $$TEST_EXIT

test-go:
	@echo "Building Go binary..."
	@mkdir -p .out
	@go build -o .out/wt wt.go
	@echo "Setting up test environment for Go..."
	@mkdir -p $(TEST_DIR)
	@export WT_ROOT=$(TEST_DIR) && \
		export WT_CMD="./.out/wt" && \
		export WT_SKIP_PROMPTS=1 && \
		./wt-test.sh; \
		TEST_EXIT=$$?; \
		rm -rf $(TEST_DIR); \
		exit $$TEST_EXIT

clean:
	rm -rf /tmp/wt-test-*
	rm -f .out/wt
