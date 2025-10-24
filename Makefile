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
GFD_BIN    := $(BIN_DIR)/gfd

# Source files
SERVER_SRC := $(CMD_DIR)/server/srunner.go
CLIENT_SRC := $(CMD_DIR)/client/crunner.go
LFD_SRC    := $(CMD_DIR)/lfd/lrunner.go
GFD_SRC    := $(CMD_DIR)/gfd/grunner.go

# ===== Phony Targets =====
.PHONY: all build clean fmt vet test help

# Default target
all: build

# Build all binaries
build: $(SERVER_BIN) $(CLIENT_BIN) $(LFD_BIN) $(GFD_BIN)
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

$(GFD_BIN): $(GFD_SRC)
	@mkdir -p $(BIN_DIR)
	@echo "Building gfd..."
	$(GO) build -ldflags="$(LDFLAGS)" -o $(GFD_BIN) $(GFD_SRC)

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

# Display help
help:
	@echo "Available targets:"
	@echo "  make build   - Build all binaries (gfd, server, lfd, client)"
	@echo "  make clean   - Remove build artifacts and logs"
	@echo "  make fmt     - Format Go code"
	@echo "  make vet     - Run static analysis"
	@echo "  make test    - Run tests"
	@echo "  make help    - Show this help message"
	@echo ""
	@echo "To run components, use the binaries directly:"
	@echo "  ./bin/gfd -addr :8000"
	@echo "  ./bin/server -addr :9001 -rid S1 -init_state 0"
	@echo "  ./bin/lfd -target 127.0.0.1:9001 -id S1 -gfd 127.0.0.1:8000"
	@echo "  ./bin/client -id C1 -servers \"S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003\" -auto"