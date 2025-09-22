#!/usr/bin/env sh
set -eu

# General component launcher script
# Usage: ./run.sh <component> [name] [args...]
# Example: ./run.sh server server1 -addr :9000 -rid S1 -init_state 0
#          ./run.sh client client1 -id 1 -server 127.0.0.1:9000
#          ./run.sh lfd lfd1 -target 127.0.0.1:9000 -hb 1s -timeout 3s -id LFD1
#
# If no arguments are provided for a component, defaults from environment variables will be used:
# - server: uses SERVER_ADDR (-addr), SERVER_REPLICA_ID (-rid), SERVER_INIT_STATE (-init_state)
# - client: uses CLIENT_ID (-id), CLIENT_TARGET_ADDR (-server)
# - lfd: uses LFD_TARGET_ADDR (-target), LFD_HB_FREQ (-hb), LFD_TIMEOUT (-timeout), LFD_ID (-id)

# Check arguments
if [ $# -lt 1 ]; then
  echo "Usage: $0 <component> [name] [args...]"
  echo ""
  echo "Components:"
  echo "  server - Run server binary"
  echo "  client - Run client binary"
  echo "  lfd    - Run LFD binary"
  echo ""
  echo "Examples:"
  echo "  $0 server server1 -addr :9000 -rid S1 -init_state 0"
  echo "  $0 client client1 -id 1 -server 127.0.0.1:9000"
  echo "  $0 lfd lfd1 -target 127.0.0.1:9000 -hb 1s -timeout 3s -id LFD1"
  echo ""
  echo "If no arguments provided, environment variables will be used as defaults"
  exit 1
fi

COMPONENT="$1"
shift 1

# Set default name if not provided
if [ $# -gt 0 ]; then
  NAME="$1"
  shift 1
else
  NAME="${COMPONENT}1"
fi

ARGS="$@"

# Get script and project directories
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"

# Load environment variables if available
ENV_FILE="$ROOT_DIR/.env"
if [ -f "$ENV_FILE" ]; then
  . "$ENV_FILE"
else
  # Default values if no .env file
  LOG_DIR="${LOG_DIR:-./logs}"
  PID_DIR="${PID_DIR:-./run}"
  BIN_DIR="${BIN_DIR:-./bin}"
fi

# If no additional args provided, use environment variable defaults
if [ -z "$ARGS" ]; then
  case "$COMPONENT" in
    server)
      if [ -n "${SERVER_ADDR:-}" ]; then
        ARGS="-addr $SERVER_ADDR"
      fi
      if [ -n "${SERVER_REPLICA_ID:-}" ]; then
        ARGS="$ARGS -rid $SERVER_REPLICA_ID"
      fi
      if [ -n "${SERVER_INIT_STATE:-}" ]; then
        ARGS="$ARGS -init_state $SERVER_INIT_STATE"
      fi
      ;;
    client)
      if [ -n "${CLIENT_ID:-}" ]; then
        ARGS="-id $CLIENT_ID"
      fi
      if [ -n "${CLIENT_TARGET_ADDR:-}" ]; then
        ARGS="$ARGS -server $CLIENT_TARGET_ADDR"
      fi
      ;;
    lfd)
      if [ -n "${LFD_TARGET_ADDR:-}" ]; then
        ARGS="-target $LFD_TARGET_ADDR"
      fi
      if [ -n "${LFD_HB_FREQ:-}" ]; then
        ARGS="$ARGS -hb $LFD_HB_FREQ"
      fi
      if [ -n "${LFD_TIMEOUT:-}" ]; then
        ARGS="$ARGS -timeout $LFD_TIMEOUT"
      fi
      if [ -n "${LFD_ID:-}" ]; then
        ARGS="$ARGS -id $LFD_ID"
      fi
      ;;
  esac
  # Remove leading space if exists
  ARGS=$(echo "$ARGS" | sed 's/^ *//')
fi

# Validate component type
case "$COMPONENT" in
  server|client|lfd)
    BINARY="$BIN_DIR/$COMPONENT"
    ;;
  *)
    echo "[Error] Invalid component: $COMPONENT"
    echo "Valid components: server, client, lfd"
    exit 1
    ;;
esac

# Check if binary exists
if [ ! -f "$BINARY" ]; then
  echo "[Error] Binary not found: $BINARY"
  echo "Please run 'make build' first"
  exit 1
fi

# Create required directories
mkdir -p "$LOG_DIR" "$PID_DIR"

# Check if already running
PID_FILE="$PID_DIR/$NAME.pid"
if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE" 2>/dev/null || true)
  if [ -n "${OLD_PID:-}" ] && kill -0 "$OLD_PID" 2>/dev/null; then
    echo "[Warning] $NAME is already running with PID $OLD_PID"
    echo "Use 'kill $OLD_PID' to stop it first"
    exit 1
  else
    echo "[Info] Removing stale PID file for $NAME"
    rm -f "$PID_FILE"
  fi
fi

# Start component
echo "==========================================="
echo "Starting $COMPONENT: $NAME"
echo "==========================================="
echo "Binary: $BINARY"
if [ -n "$ARGS" ]; then
  echo "Arguments: $ARGS"
else
  echo "Arguments: (none - using defaults if available)"
fi
echo "Log file: $LOG_DIR/$NAME.log"
echo "PID file: $PID_DIR/$NAME.pid"
echo ""

# Start component in background and redirect output to log file
echo "[Starting] $NAME..."
"$BINARY" $ARGS > "$LOG_DIR/$NAME.log" 2>&1 &
PID=$!

# Save PID
echo "$PID" > "$PID_FILE"

# Check if process started successfully
sleep 0.2
if kill -0 "$PID" 2>/dev/null; then
  echo "[Started] $NAME with PID $PID"
  echo ""
  echo "To view logs:"
  echo "  tail -f $LOG_DIR/$NAME.log"
  echo ""
  echo "To stop:"
  echo "  kill $PID"
  echo "  rm $PID_FILE"
else
  echo "[Error] Failed to start $NAME"
  echo "Check log file for details: $LOG_DIR/$NAME.log"
  rm -f "$PID_FILE"
  exit 1
fi