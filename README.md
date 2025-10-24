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

---

## Milestone 1: Basic Client-Server with LFD

### Running Milestone 1

```bash
# Run binaries directly
./bin/server -addr :9000 -rid S1 -init_state 0
./bin/client -id 1 -server :9000
./bin/lfd -target :9000 -hb 1s -timeout 3s -id LFD1
```

### Milestone 1 Component Parameters

**Server:**
- `-addr`: Listen address (default: `:9000`)
- `-rid`: Replica ID (default: `S1`)
- `-init_state`: Initial state value (default: `0`)

**Client (Milestone 1):**
- `-id`: Client identifier (default: `1`)
- `-server`: Server address (default: `127.0.0.1:9000`)

**LFD (Milestone 1):**
- `-target`: Server address to monitor (default: `127.0.0.1:9000`)
- `-hb`: Heartbeat interval (default: `1s`)
- `-timeout`: Heartbeat timeout (default: `3s`)
- `-id`: LFD identifier (default: `LFD1`)

---

## Milestone 2: Active Replication with Fault Tolerance

Milestone 2 implements active replication with:
- **GFD (Global Fault Detector)**: Maintains global membership list
- **3 Server Replicas**: S1, S2, S3 running on different ports
- **3 LFDs**: One per server, with exponential backoff reconnection
- **3 Clients**: Each connects to all replicas, with duplicate detection and request queueing

### Running Milestone 2

**You need 10 terminal windows to run all components.**

#### Terminal 1 - GFD
```bash
./bin/gfd -addr :8000
```

#### Terminal 2-4 - Servers
```bash
# Terminal 2 - Server S1
./bin/server -addr :9001 -rid S1 -init_state 0

# Terminal 3 - Server S2
./bin/server -addr :9002 -rid S2 -init_state 0

# Terminal 4 - Server S3
./bin/server -addr :9003 -rid S3 -init_state 0
```

#### Terminal 5-7 - LFDs (monitoring each server)
```bash
# Terminal 5 - LFD1 monitoring S1
./bin/lfd -target 127.0.0.1:9001 -id S1 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s -max-retries 5

# Terminal 6 - LFD2 monitoring S2
./bin/lfd -target 127.0.0.1:9002 -id S2 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s -max-retries 5

# Terminal 7 - LFD3 monitoring S3
./bin/lfd -target 127.0.0.1:9003 -id S3 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s -max-retries 5
```

#### Terminal 8-10 - Clients (auto mode)
```bash
# Terminal 8 - Client C1
./bin/client -id C1 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" -interval 2s -auto

# Terminal 9 - Client C2
./bin/client -id C2 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" -interval 2s -auto

# Terminal 10 - Client C3
./bin/client -id C3 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" -interval 2s -auto
```

### Milestone 2 Component Parameters

**GFD:**
| Parameter | Description | Default |
|-----------|-------------|---------|
| `-addr` | Listen address | `:8000` |

**Server:**
| Parameter | Description | Default |
|-----------|-------------|---------|
| `-addr` | Listen address | `:9000` |
| `-rid` | Replica ID (S1, S2, S3) | `S1` |
| `-init_state` | Initial state value | `0` |

**LFD (Milestone 2):**
| Parameter | Description | Default |
|-----------|-------------|---------|
| `-target` | Server address to monitor | `127.0.0.1:9000` |
| `-id` | Replica ID (must match server) | `LFD1` |
| `-gfd` | GFD address | `127.0.0.1:8000` |
| `-hb` | Heartbeat interval | `1s` |
| `-timeout` | Heartbeat timeout | `3s` |
| `-max-retries` | Max reconnection attempts | `5` |
| `-base-delay` | Base delay for exponential backoff | `1s` |
| `-max-delay` | Max delay for exponential backoff | `30s` |

**Client (Milestone 2):**
| Parameter | Description | Default |
|-----------|-------------|---------|
| `-id` | Client ID (C1, C2, C3) | `C1` |
| `-servers` | Server list: `"ID1=addr1,ID2=addr2,..."` | - |
| `-interval` | Request interval (auto mode) | `2s` |
| `-auto` | Enable auto-send mode | `false` |

### Milestone 2 Features

✅ **GFD**: Maintains membership list `[S1, S2, S3]`
✅ **LFD Exponential Backoff**: Reconnection delays: 1s → 2s → 4s → 8s → 16s → 30s
✅ **Active Replication**: Clients send to all 3 replicas
✅ **Duplicate Detection**: First reply accepted, others discarded
✅ **Request Queueing**: Queues requests when disconnected, flushes on reconnect
✅ **Fault Tolerance**: Tolerates 1-2 server failures

### Testing Fault Tolerance

```bash
# Kill Server S1 (in a new terminal)
kill $(cat run/server1.pid)  # if running in background
# or press Ctrl+C in Server S1's terminal

# Observe:
# - LFD1: Attempts reconnection with exponential backoff
# - GFD: Updates membership to "GFD: 2 members: S2, S3"
# - Clients: Continue using S2 and S3 without interruption
```

### Running in Background (Optional)

If you prefer to run components in the background:

```bash
mkdir -p logs run

# GFD
./bin/gfd -addr :8000 > logs/gfd.log 2>&1 &
echo $! > run/gfd.pid

# Servers
./bin/server -addr :9001 -rid S1 -init_state 0 > logs/server1.log 2>&1 &
echo $! > run/server1.pid

./bin/server -addr :9002 -rid S2 -init_state 0 > logs/server2.log 2>&1 &
echo $! > run/server2.pid

./bin/server -addr :9003 -rid S3 -init_state 0 > logs/server3.log 2>&1 &
echo $! > run/server3.pid

# LFDs
./bin/lfd -target 127.0.0.1:9001 -id S1 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s > logs/lfd1.log 2>&1 &
echo $! > run/lfd1.pid

./bin/lfd -target 127.0.0.1:9002 -id S2 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s > logs/lfd2.log 2>&1 &
echo $! > run/lfd2.pid

./bin/lfd -target 127.0.0.1:9003 -id S3 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s > logs/lfd3.log 2>&1 &
echo $! > run/lfd3.pid

# Clients
./bin/client -id C1 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" -interval 2s -auto > logs/client1.log 2>&1 &
echo $! > run/client1.pid

./bin/client -id C2 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" -interval 2s -auto > logs/client2.log 2>&1 &
echo $! > run/client2.pid

./bin/client -id C3 -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" -interval 2s -auto > logs/client3.log 2>&1 &
echo $! > run/client3.pid

# View logs
tail -f logs/gfd.log
tail -f logs/client1.log

# Stop all
kill $(cat run/*.pid)
rm -rf run/*.pid
```

---

## Project Structure

```
.
├── cmd/                    # Main entry points
│   ├── server/
│   │   └── srunner.go     # Server launcher
│   ├── client/
│   │   └── crunner.go     # Client launcher
│   ├── lfd/
│   │   └── lrunner.go     # LFD launcher
│   └── gfd/
│       └── grunner.go     # GFD launcher (Milestone 2)
├── server/                # Server implementation
│   ├── server_api.go      # Server interface
│   └── server_impl.go     # Server logic
├── client/                # Client implementation
│   ├── client_api.go      # Client interface
│   └── client_impl.go     # Client logic (Milestone 2: active replication)
├── lfd/                   # LFD implementation
│   ├── lfd_api.go         # LFD interface
│   └── lfd_impl.go        # LFD logic (Milestone 2: exponential backoff)
├── gfd/                   # GFD implementation (Milestone 2)
│   ├── gfd_api.go         # GFD interface
│   └── gfd_impl.go        # GFD logic
├── utils/                 # Shared utilities
│   └── utils.go           # Network helpers
├── bin/                   # Compiled binaries (generated)
├── logs/                  # Log files (generated)
├── run/                   # PID files (generated)
├── Makefile              # Build automation
├── .env.example          # Environment configuration template
└── go.mod                # Go module definition
```

---

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
make build   # Build all binaries (gfd, server, lfd, client)
make clean   # Remove build artifacts and logs
make fmt     # Format Go code
make vet     # Run static analysis
make test    # Run tests
make help    # Show help message
```

**Note:** All component execution is done by running binaries directly (see commands above). Makefile is for build/development tasks only.

---

## Configuration

Edit `.env` file to customize default settings. See `.env.example` for all available options.

**Milestone 1 Configuration:**
```bash
SERVER_ADDR="127.0.0.1:9000"
SERVER_REPLICA_ID="S1"
SERVER_INIT_STATE="0"
LFD_TARGET_ADDR="127.0.0.1:9000"
LFD_HB_FREQ="1s"
LFD_TIMEOUT="3s"
```

**Milestone 2 Configuration:**
```bash
GFD_ADDR="127.0.0.1:8000"
SERVER1_ADDR="127.0.0.1:9001"
SERVER2_ADDR="127.0.0.1:9002"
SERVER3_ADDR="127.0.0.1:9003"
# ... (see .env.example for full configuration)
```

---

## Logging

All components write detailed logs to the `logs/` directory:
- `gfd.log`: GFD membership changes
- `server*.log`: Server operations and state changes
- `client*.log`: Client requests, responses, and duplicate detection
- `lfd*.log`: Heartbeat status and failure detection

---

## Architecture Overview

### Milestone 2 Architecture

```
┌─────────────────────────────────────────────────────────┐
│                         GFD                              │
│                    (Port 8000)                          │
│              membership: [S1, S2, S3]                   │
└─────────────────────────────────────────────────────────┘
         ▲           ▲           ▲
         │ADD/DELETE │ADD/DELETE │ADD/DELETE
         │           │           │
    ┌────┴────┐ ┌────┴────┐ ┌────┴────┐
    │  LFD1   │ │  LFD2   │ │  LFD3   │
    │  (S1)   │ │  (S2)   │ │  (S3)   │
    └────┬────┘ └────┬────┘ └────┬────┘
         │PING/PONG  │PING/PONG  │PING/PONG
         ▼           ▼           ▼
    ┌────────┐  ┌────────┐  ┌────────┐
    │   S1   │  │   S2   │  │   S3   │
    │ :9001  │  │ :9002  │  │ :9003  │
    └────────┘  └────────┘  └────────┘
         ▲           ▲           ▲
         │           │           │
         │  REQ/RESP │  REQ/RESP │  REQ/RESP
         └───────────┴───────────┴───────────
                     │
            ┌────────┼────────┐
            │        │        │
        ┌───┴──┐ ┌───┴──┐ ┌───┴──┐
        │  C1  │ │  C2  │ │  C3  │
        └──────┘ └──────┘ └──────┘
```

---

## License

This project is for educational purposes as part of CMU 18-749 course.
