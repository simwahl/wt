.PHONY: test clean

TEST_DIR := /tmp/wt-test-$$$$
SAVED_WT_ROOT := $(WT_ROOT)

test:
	@echo "Setting up test environment..."
	@mkdir -p $(TEST_DIR)
	@export WT_ROOT=$(TEST_DIR) && \
		./wt-test.sh; \
		TEST_EXIT=$$?; \
		rm -rf $(TEST_DIR); \
		exit $$TEST_EXIT

clean:
	rm -rf /tmp/wt-test-*
