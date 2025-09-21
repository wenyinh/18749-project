# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Context

This is a distributed systems implementation for CMU 18-749 course. The system consists of three main components that communicate via TCP:
- **Server**: Maintains state and processes client requests
- **Client**: Sends requests to server
- **LFD (Local Fault Detector)**: Monitors server health via heartbeats

## Essential Commands

### Building and Running
```bash
# Build all binaries
make build

# Run Milestone 1 demo (1 server + 3 test clients + 1 LFD)
make run-milestone1
make stop-milestone1

# Run individual components with custom configuration
make run COMPONENT=server NAME=s1 ARGS="-addr :9000 -rid S1 -init_state 0"
make run COMPONENT=client NAME=c1 ARGS="-id 1 -server :9000 -test"
make run COMPONENT=lfd NAME=lfd1 ARGS="-target :9000 -interval-ms 1000"
make stop NAME=s1

# Clean build artifacts and logs
make clean
```

### Testing and Code Quality
```bash
make test           # Run all tests with race detection
make fmt            # Format Go code
make vet            # Run static analysis
```

## Architecture Overview

The codebase follows a clean separation pattern:

1. **Interface/Implementation Split**: Each component has an `_api.go` file defining the interface and an `_impl.go` file with the implementation. All components expose a single `Run() error` method.

2. **Runner Pattern**: The `cmd/` directory contains thin launcher programs (`srunner.go`, `crunner.go`, `lrunner.go`) that only handle command-line arguments and call the actual implementation.

3. **Script Hierarchy**:
   - `scripts/run.sh` and `scripts/stop.sh`: Generic component management with full PID/log handling
   - `scripts/run_milestone1.sh` and `scripts/stop_milestone1.sh`: Orchestrate multiple components using the generic scripts

## Communication Protocols

### Client-Server Protocol
- Request: `REQ <client_id> <request_id>`
- Response: `RESP<client_id> <request_id> <server_state>`
- Server increments its state counter on each request

### LFD-Server Protocol
- LFD sends: `PING`
- Server responds: `PONG`
- After 3 consecutive failures, LFD marks server as DOWN

## Key Implementation Details

- **Network Utils**: The `utils` package provides `WriteLine()`/`ReadLine()` for text-based protocols and `MustListen()`/`MustDial()` that panic on failure (used during initialization).

- **Client Modes**: Clients have two modes:
  - Normal mode: Just maintains connection
  - Test mode (`-test` flag): Sends requests every 5 seconds

- **Process Management**: All components create PID files in `run/` and logs in `logs/` directories. The scripts handle graceful shutdown with SIGTERM followed by SIGKILL if needed.

## Environment Configuration

The `.env` file (copy from `.env.example`) sets defaults for Milestone 1:
- `SERVER_ADDR`: Default server address
- `CLIENT[1-3]_ID`: IDs for the three test clients
- `LFD_TARGET_ADDR`: Address that LFD monitors
- Directory paths for logs, PIDs, and binaries

## Important Notes

- Server is currently single-instance (Milestone 1). Future milestones will add replication.
- Client reconnection in test mode exits after 3 failed attempts - this is intentional for Milestone 1.
- LFD currently only logs DOWN status. Integration with GFD (Global Fault Detector) will come in later milestones.