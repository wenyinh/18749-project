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
.PHONY: all build clean fmt vet test run stop run-milestone1 run-server run-client run-lfd stop-milestone1 help

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

# Run a single component with custom name and args
# Usage: make run COMPONENT=server NAME=s1 ARGS="-addr :9000 -rid S1"
run: build
	@if [ -z "$(COMPONENT)" ] || [ -z "$(NAME)" ]; then \
		echo "Usage: make run COMPONENT=<type> NAME=<name> ARGS=\"<args>\""; \
		echo "Example: make run COMPONENT=server NAME=s1 ARGS=\"-addr :9000 -rid S1\""; \
		exit 1; \
	fi
	./scripts/run.sh $(COMPONENT) $(NAME) $(ARGS)

# Stop a single component by name
# Usage: make stop NAME=s1
stop:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make stop NAME=<name>"; \
		echo "Example: make stop NAME=s1"; \
		exit 1; \
	fi
	./scripts/stop.sh $(NAME)

# Run Milestone 1 demo (1 server + 3 test clients + 1 LFD)
run-milestone1: build
	@if [ ! -f .env ]; then \
		echo "Error: .env not found. Copy .env.example to .env and edit it."; \
		exit 1; \
	fi
	./scripts/run_milestone1.sh

# Run individual components (use environment variables or pass your own args)
# Examples:
#   make run-server ARGS="-addr :8080 -rid S2"
#   make run-client ARGS="-id 5 -server localhost:8080 -test"
run-server: $(SERVER_BIN)
	$(SERVER_BIN) $(ARGS)

run-client: $(CLIENT_BIN)
	$(CLIENT_BIN) $(ARGS)

run-lfd: $(LFD_BIN)
	$(LFD_BIN) $(ARGS)

# Stop Milestone 1 components
stop-milestone1:
	./scripts/stop_milestone1.sh

# Display help
help:
	@echo "Available targets:"
	@echo "  make build          - Build all binaries"
	@echo "  make run            - Run a component (COMPONENT=server|client|lfd NAME=<name> ARGS=\"...\")"
	@echo "  make stop           - Stop a component (NAME=<name>)"
	@echo "  make run-milestone1 - Run Milestone 1 demo (1 server + 3 test clients + 1 LFD)"
	@echo "  make stop-milestone1 - Stop Milestone 1 components"
	@echo "  make run-server     - Run server directly (pass ARGS=\"-addr :9000 -rid S1 -init_state 0\")"
	@echo "  make run-client     - Run client directly (pass ARGS=\"-id 1 -server :9000\")"
	@echo "  make run-lfd        - Run LFD directly (pass ARGS=\"-target :9000 -hb 1s -timeout 3s -id LFD1\")"
	@echo "  make test           - Run tests"
	@echo "  make fmt            - Format Go code"
	@echo "  make vet            - Run static analysis"
	@echo "  make clean          - Remove build artifacts and logs"
	@echo "  make help           - Show this help message"