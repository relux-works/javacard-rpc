# javacard-rpc — top-level Makefile
#
# Targets:
#   build-codegen   Build the jcrpc-gen CLI
#   generate        Generate counter example packages from TOML IDL
#   build-bridge    Build the jCardSim TCP bridge
#   build-applet    Build the counter example applet
#   run-bridge      Start the bridge with counter applet loaded
#   run-example     Build and run the Swift E2E CLI
#   e2e             Full pipeline: generate → build → bridge → run tests
#   clean           Remove build artifacts

CODEGEN_DIR   := codegen
CODEGEN_BIN   := $(CODEGEN_DIR)/jcrpc-gen
EXAMPLE_DIR   := examples/counter
CLI_DIR       := $(EXAMPLE_DIR)/cli
GEN_DIR       := $(EXAMPLE_DIR)/generated

.PHONY: build-codegen generate build-bridge build-applet run-bridge run-example e2e clean

# --- Build ---

build-codegen:
	cd $(CODEGEN_DIR) && go build -o jcrpc-gen ./cmd/jcrpc-gen

generate: build-codegen
	mkdir -p $(GEN_DIR)
	$(CODEGEN_BIN) --all --out-dir $(GEN_DIR) --verbose $(EXAMPLE_DIR)/counter.toml

build-bridge:
	cd bridge && ./gradlew build -q

build-applet:
	cd $(EXAMPLE_DIR)/applet && ./gradlew build -q

build-cli: generate
	cd $(CLI_DIR) && swift build

# --- Run ---

run-bridge: build-bridge build-applet
	$(EXAMPLE_DIR)/run-bridge.sh

run-example: build-cli
	cd $(CLI_DIR) && swift run

# --- E2E (run in two terminals or use & for bridge) ---

e2e: generate build-bridge build-applet build-cli
	@echo ""
	@echo "=== E2E ready ==="
	@echo "Terminal 1: make run-bridge"
	@echo "Terminal 2: make run-example"
	@echo ""

# --- Test ---

test-codegen:
	cd $(CODEGEN_DIR) && go test ./...

test: test-codegen

# --- Clean ---

clean:
	rm -f $(CODEGEN_BIN)
	rm -rf $(GEN_DIR)
	cd $(CLI_DIR) && rm -rf .build
	cd bridge && ./gradlew clean -q 2>/dev/null || true
	cd $(EXAMPLE_DIR)/applet && ./gradlew clean -q 2>/dev/null || true
