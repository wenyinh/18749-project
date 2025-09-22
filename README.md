# 18749: Building Reliable Distributed System - Project

## Team 12: Vincent Guo, Wenyin He, Fan Bu, Qiaoan Shen, Lenghan Zhu

## Quick Start

### Prerequisites

- Go 1.19+ installed
- Linux/macOS/WSL environment
- Make utility

### Installation

```bash
# Clone the repository
git clone https://github.com/wenyinh/18749-project.git
cd 18749-project

# Copy environment configuration
cp .env.example .env

# Build all binaries
make build
```

### Running Milestone 1 Demo

```bash
# Start all components (1 server + 3 long-running clients + 1 LFD)
make run-milestone1

# View logs in another terminal
tail -f logs/server.log
tail -f logs/client1.log

# Stop all components
make stop-milestone1
```

## Usage

### Individual Component Management

Start components individually with custom configurations:

```bash
# Start server
make run COMPONENT=server NAME=s1 ARGS="-addr :9000 -rid S1 -init_state 0"

# Start long-running client
make run COMPONENT=client NAME=c1 ARGS="-id 1 -server :9000"

# Start LFD
make run COMPONENT=lfd NAME=lfd1 ARGS="-target :9000 -interval-ms 1000 -id LFD1"

# Stop a component
make stop NAME=s1
```

### Direct Binary Execution

```bash
# Run components directly via Make
make run-server ARGS="-addr :9000 -rid S1 -init_state 0"
make run-client ARGS="-id 1 -server :9000"
make run-lfd ARGS="-target :9000 -hb 1s -timeout 3s -id LFD1"

# Or run binaries directly
./bin/server -addr :9000 -rid S1 -init_state 0
./bin/client -id 1 -server :9000
./bin/lfd -target :9000 -hb 1s -timeout 3s -id LFD1
```

## Component Parameters

### Server
- `-addr`: Listen address (default: ":9000")
- `-rid`: Replica ID for logging (default: "S1")
- `-init_state`: Initial state value (default: 0)

### Client
- `-id`: Client identifier (default: "1")
- `-server`: Server address (default: "127.0.0.1:9000")

**Note**: Clients now run continuously as long-running processes that maintain connections to the server.

### LFD (Local Fault Detector)
- `-target`: Server address to monitor (default: "127.0.0.1:9000")
- `-hb`: Heartbeat interval (e.g., "1s", "1000ms") (default: "1s")
- `-timeout`: Timeout duration (e.g., "3s") (default: "3s")
- `-id`: LFD identifier (default: "LFD1")

## Project Structure

```
.
├── cmd/                    # Main entry points
│   ├── server/
│   │   └── srunner.go     # Server launcher
│   ├── client/
│   │   └── crunner.go     # Client launcher
│   └── lfd/
│       └── lrunner.go     # LFD launcher
├── server/                # Server implementation
│   ├── server_api.go      # Server interface
│   └── server_impl.go     # Server logic
├── client/                # Client implementation
│   ├── client_api.go      # Client interface
│   └── client_impl.go     # Client logic
├── lfd/                   # LFD implementation
│   ├── lfd_api.go         # LFD interface
│   └── lfd_impl.go        # LFD logic
├── utils/                 # Shared utilities
│   └── utils.go           # Network helpers
├── scripts/               # Management scripts
│   ├── run.sh            # Generic component launcher
│   ├── stop.sh           # Generic component stopper
│   ├── run_milestone1.sh # Milestone 1 demo launcher
│   └── stop_milestone1.sh # Milestone 1 demo stopper
├── bin/                   # Compiled binaries (generated)
├── logs/                  # Log files (generated)
├── run/                   # PID files (generated)
├── Makefile              # Build and run automation
├── .env.example          # Environment configuration template
└── go.mod                # Go module definition
```

## Development

### Building

```bash
make build          # Build all binaries
make clean          # Clean build artifacts and logs
```

### Testing

```bash
make test           # Run all tests
make fmt            # Format Go code
make vet            # Run static analysis
```

### Available Make Targets

```bash
make build          # Build all binaries
make run            # Run a component (COMPONENT=server|client|lfd NAME=<name> ARGS="...")
make stop           # Stop a component (NAME=<name>)
make run-milestone1 # Run Milestone 1 demo (1 server + 3 long-running clients + 1 LFD)
make stop-milestone1 # Stop Milestone 1 components
make run-server     # Run server directly (pass ARGS="-addr :9000 -rid S1 -init_state 0")
make run-client     # Run client directly (pass ARGS="-id 1 -server :9000")
make run-lfd        # Run LFD directly (pass ARGS="-target :9000 -hb 1s -timeout 3s -id LFD1")
make test           # Run tests
make fmt            # Format Go code
make vet            # Run static analysis
make clean          # Remove build artifacts and logs
make help           # Show detailed help message
```

## Configuration

Edit `.env` file to customize default settings:

```bash
# Server configuration
SERVER_ADDR="127.0.0.1:9000"
SERVER_REPLICA_ID="S1"
SERVER_INIT_STATE="0"

# Client configuration
CLIENT1_ID="1"
CLIENT2_ID="2"
CLIENT3_ID="3"

# LFD configuration
LFD_TARGET_ADDR="127.0.0.1:9000"
LFD_HB_FREQ="1s"
LFD_TIMEOUT="3s"
LFD_ID="LFD1"

# Directory configuration
LOG_DIR="./logs"
PID_DIR="./run"
BIN_DIR="./bin"
```

## Logging

All components write detailed logs to the `logs/` directory:
- `server.log`: Server operations and state changes
- `client*.log`: Client requests and responses
- `lfd.log`: Heartbeat status and failure detection

## License

This project is for educational purposes as part of CMU 18-749 course.
