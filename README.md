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
# Start all components (1 server + 3 test clients + 1 LFD)
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

# Start client (normal mode)
make run COMPONENT=client NAME=c1 ARGS="-id 1 -server :9000"

# Start client (test mode - sends periodic requests)
make run COMPONENT=client NAME=c1 ARGS="-id 1 -server :9000 -test"

# Start LFD
make run COMPONENT=lfd NAME=lfd1 ARGS="-target :9000 -interval-ms 1000 -id LFD1"

# Stop a component
make stop NAME=s1
```

### Direct Binary Execution

```bash
# Run server directly
./bin/server -addr :9000 -rid S1 -init_state 0

# Run client directly
./bin/client -id 1 -server :9000 -test

# Run LFD directly
./bin/lfd -target :9000 -interval-ms 1000 -id LFD1
```

## Component Parameters

### Server
- `-addr`: Listen address (default: ":9000")
- `-rid`: Replica ID for logging (default: "S1")
- `-init_state`: Initial state value (default: 0)

### Client
- `-id`: Client identifier (default: "1")
- `-server`: Server address (default: "127.0.0.1:9000")
- `-test`: Enable test mode for periodic requests (default: false)

### LFD (Local Fault Detector)
- `-target`: Server address to monitor (default: "127.0.0.1:9000")
- `-interval-ms`: Heartbeat interval in milliseconds (default: 1000)
- `-id`: LFD identifier (default: "LFD1")

## Project Structure

```
.
├── cmd/                    # Main entry points
│   ├── srunner.go         # Server launcher
│   ├── crunner.go         # Client launcher
│   └── lrunner.go         # LFD launcher
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
make help           # Show all available targets
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
LFD_INTERVAL_MS="1000"

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
