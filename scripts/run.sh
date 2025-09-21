#!/usr/bin/env sh
set -eu

# General component launcher script
# Usage: ./run_component.sh <component> <name> [args...]
# Example: ./run_component.sh server server1 -addr :9000 -rid S1
#          ./run_component.sh client client1 -id 1 -server :9000 -test
#          ./run_component.sh lfd lfd1 -target :9000 -interval-ms 1000

# Check arguments
if [ $# -lt 2 ]; then
  echo "Usage: $0 <component> <name> [args...]"
  echo ""
  echo "Components:"
  echo "  server - Run server binary"
  echo "  client - Run client binary"
  echo "  lfd    - Run LFD binary"
  echo ""
  echo "Examples:"
  echo "  $0 server server1 -addr :9000 -rid S1"
  echo "  $0 client client1 -id 1 -server :9000 -test"
  echo "  $0 lfd lfd1 -target :9000 -interval-ms 1000"
  exit 1
fi

COMPONENT="$1"
NAME="$2"
shift 2
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
echo "Arguments: $ARGS"
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