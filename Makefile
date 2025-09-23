# ===== Go & Build Configuration =====
GO        ?= go
BIN_DIR   ?= bin
CMD_DIR   := cmd
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -X 'main.version=$(VERSION)'

# Binary targets
SERVER_BIN := $(BIN_DIR)/server
CLIENT_BIN := $(BIN_DIR)/client
LFD_BIN    := $(BIN_DIR)/lfd

# Source files
SERVER_SRC := $(CMD_DIR)/server/srunner.go
CLIENT_SRC := $(CMD_DIR)/client/crunner.go
LFD_SRC    := $(CMD_DIR)/lfd/lrunner.go

# ===== Phony Targets =====
.PHONY: all build clean fmt vet test run-server run-client run-lfd help

# Default target
all: build

# Build all binaries
build: $(SERVER_BIN) $(CLIENT_BIN) $(LFD_BIN)
	@echo "Build complete. Binaries in $(BIN_DIR)/"

# Build individual binaries
$(SERVER_BIN): $(SERVER_SRC)
	@mkdir -p $(BIN_DIR)
	@echo "Building server..."
	$(GO) build -ldflags="$(LDFLAGS)" -o $(SERVER_BIN) $(SERVER_SRC)

$(CLIENT_BIN): $(CLIENT_SRC)
	@mkdir -p $(BIN_DIR)
	@echo "Building client..."
	$(GO) build -ldflags="$(LDFLAGS)" -o $(CLIENT_BIN) $(CLIENT_SRC)

$(LFD_BIN): $(LFD_SRC)
	@mkdir -p $(BIN_DIR)
	@echo "Building lfd..."
	$(GO) build -ldflags="$(LDFLAGS)" -o $(LFD_BIN) $(LFD_SRC)

# Clean build artifacts and logs
clean:
	rm -rf $(BIN_DIR) logs run
	@echo "Cleaned build artifacts and logs"

# Format Go code
fmt:
	$(GO) fmt ./...

# Run static analysis
vet:
	$(GO) vet ./...

# Run tests
test:
	$(GO) test -race ./...



# Run individual components directly (use environment variables or pass your own args)
# If ARGS is not provided, the binary will use its default flag values
# Examples:
#   make run-server ARGS="-addr :8080 -rid S2 -init_state 0"
#   make run-client ARGS="-id 5 -server localhost:8080"
#   make run-lfd ARGS="-target localhost:8080 -hb 1s -timeout 3s -id LFD2"
#
# Environment variables used by binaries (automatically loaded from .env):
#   SERVER_ADDR, SERVER_REPLICA_ID, SERVER_INIT_STATE
#   CLIENT_ID, CLIENT_TARGET_ADDR
#   LFD_TARGET_ADDR, LFD_HB_FREQ, LFD_TIMEOUT, LFD_ID
#
# Environment variables are automatically loaded when no ARGS provided:
#   make run-server    # uses .env defaults
#   make run-client    # uses .env defaults
#   make run-lfd       # uses .env defaults
run-server: $(SERVER_BIN)
	@if [ -n "$(ARGS)" ]; then \
		echo "Running server with args: $(ARGS)"; \
		$(SERVER_BIN) $(ARGS); \
	else \
		echo "Running server with environment variable defaults"; \
		if [ -f .env ]; then . ./.env; fi; \
		$(SERVER_BIN) $${SERVER_ADDR:+-addr $$SERVER_ADDR} $${SERVER_REPLICA_ID:+-rid $$SERVER_REPLICA_ID} $${SERVER_INIT_STATE:+-init_state $$SERVER_INIT_STATE}; \
	fi

run-client: $(CLIENT_BIN)
	@if [ -n "$(ARGS)" ]; then \
		echo "Running client with args: $(ARGS)"; \
		$(CLIENT_BIN) $(ARGS); \
	else \
		echo "Running client with environment variable defaults"; \
		if [ -f .env ]; then . ./.env; fi; \
		$(CLIENT_BIN) $${CLIENT_ID:+-id $$CLIENT_ID} $${CLIENT_TARGET_ADDR:+-server $$CLIENT_TARGET_ADDR}; \
	fi

run-lfd: $(LFD_BIN)
	@if [ -n "$(ARGS)" ]; then \
		echo "Running lfd with args: $(ARGS)"; \
		$(LFD_BIN) $(ARGS); \
	else \
		echo "Running lfd with environment variable defaults"; \
		if [ -f .env ]; then . ./.env; fi; \
		$(LFD_BIN) $${LFD_TARGET_ADDR:+-target $$LFD_TARGET_ADDR} $${LFD_HB_FREQ:+-hb $$LFD_HB_FREQ} $${LFD_TIMEOUT:+-timeout $$LFD_TIMEOUT} $${LFD_ID:+-id $$LFD_ID}; \
	fi


# Display help
help:
	@echo "Available targets:"
	@echo "  make build          - Build all binaries"
	@echo "  make run-server     - Run server directly (optional: ARGS=\"-addr :9000 -rid S1 -init_state 0\")"
	@echo "  make run-client     - Run client directly (optional: ARGS=\"-id 1 -server :9000\")"
	@echo "  make run-lfd        - Run LFD directly (optional: ARGS=\"-target :9000 -hb 1s -timeout 3s -id LFD1\")"
	@echo "  make test           - Run tests"
	@echo "  make fmt            - Format Go code"
	@echo "  make vet            - Run static analysis"
	@echo "  make clean          - Remove build artifacts and logs"
	@echo "  make help           - Show this help message"