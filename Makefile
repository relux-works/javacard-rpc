# javacard-rpc — top-level Makefile
#
# Targets:
#   build-codegen   Build the jcrpc-gen CLI
#   generate        Generate counter example packages from TOML IDL
#   build-bridge    Build the jCardSim TCP bridge
#   build-applet    Build the counter example applet
#   test-applet     Run jCardSim-backed applet tests for the counter example
#   run-bridge      Start the bridge with counter applet loaded
#   run-example     Build and run the Swift E2E CLI
#   run-kotlin-example Build and run the Kotlin/JVM E2E CLI
#   test-cap        Convert a generated stream applet to a verified CAP
#   release-check   Run the full test suite and required CAP conversion gate
#   e2e             Full one-shot pipeline: generate → build → bridge → run Swift + Kotlin E2E
#   clean           Remove build artifacts

CODEGEN_DIR   := codegen
CODEGEN_BIN   := $(CODEGEN_DIR)/jcrpc-gen
EXAMPLE_DIR   := examples/counter
CLI_DIR       := $(EXAMPLE_DIR)/cli
KOTLIN_CLI_DIR := $(EXAMPLE_DIR)/kotlin-cli
GEN_DIR       := $(EXAMPLE_DIR)/generated

.PHONY: build-codegen generate build-bridge build-applet build-cli build-kotlin-cli test-applet test-cap release-check run-bridge run-example run-kotlin-example e2e clean

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

build-kotlin-cli: generate
	cd $(KOTLIN_CLI_DIR) && gradle build

test-applet:
	cd $(EXAMPLE_DIR)/applet && ./gradlew test -q

# --- Run ---

run-bridge: build-bridge build-applet
	$(EXAMPLE_DIR)/run-bridge.sh

run-example: build-cli
	cd $(CLI_DIR) && swift run

run-kotlin-example: build-kotlin-cli
	cd $(KOTLIN_CLI_DIR) && gradle run

# --- E2E ---

e2e:
	cd $(EXAMPLE_DIR) && ./run-e2e.sh

# --- Test ---

test-codegen:
	cd $(CODEGEN_DIR) && go test ./...

test: test-codegen test-applet

test-cap:
	@test -n "$(JCRPC_ANT_JAVACARD_JAR)" || { echo "JCRPC_ANT_JAVACARD_JAR is required" >&2; exit 2; }
	@test -n "$(JCRPC_JCKIT_DIR)" || { echo "JCRPC_JCKIT_DIR is required" >&2; exit 2; }
	@command -v ant >/dev/null 2>&1 || { echo "ant is required" >&2; exit 2; }
	cd $(CODEGEN_DIR) && go test . -run TestGeneratedJavaStreamPackageConvertsToCAP -count=1 -v

release-check: test test-cap

# --- Clean ---

clean:
	rm -f $(CODEGEN_BIN)
	rm -rf $(GEN_DIR)
	cd $(CLI_DIR) && rm -rf .build
	cd $(KOTLIN_CLI_DIR) && rm -rf build .gradle
	cd bridge && ./gradlew clean -q 2>/dev/null || true
	cd $(EXAMPLE_DIR)/applet && ./gradlew clean -q 2>/dev/null || true
