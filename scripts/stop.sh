#!/usr/bin/env sh
set -eu

# General component stop script
# Usage: ./stop_component.sh <name>
# Example: ./stop_component.sh server1
#          ./stop_component.sh client1
#          ./stop_component.sh lfd1

# Check arguments
if [ $# -lt 1 ]; then
  echo "Usage: $0 <name>"
  echo ""
  echo "Example:"
  echo "  $0 server1"
  echo "  $0 client1"
  echo "  $0 lfd1"
  echo ""
  echo "To list running components:"
  echo "  ls run/*.pid"
  exit 1
fi

NAME="$1"

# Get script and project directories
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"

# Load environment variables if available
ENV_FILE="$ROOT_DIR/.env"
if [ -f "$ENV_FILE" ]; then
  . "$ENV_FILE"
else
  # Default values if no .env file
  PID_DIR="${PID_DIR:-./run}"
fi

PID_FILE="$PID_DIR/$NAME.pid"

# Check if PID file exists
if [ ! -f "$PID_FILE" ]; then
  echo "[Error] No PID file found for $NAME"
  echo "PID file expected at: $PID_FILE"
  echo ""
  echo "Running components:"
  if [ -d "$PID_DIR" ] && [ -n "$(ls -A $PID_DIR 2>/dev/null || true)" ]; then
    for f in $PID_DIR/*.pid; do
      [ -f "$f" ] || continue
      basename "$f" .pid
    done
  else
    echo "  None found"
  fi
  exit 1
fi

# Read PID
PID=$(cat "$PID_FILE" 2>/dev/null || true)
if [ -z "${PID:-}" ]; then
  echo "[Error] Invalid PID file for $NAME"
  rm -f "$PID_FILE"
  exit 1
fi

echo "==========================================="
echo "Stopping component: $NAME"
echo "==========================================="
echo "PID: $PID"
echo ""

# Check if process is running
if ! kill -0 "$PID" 2>/dev/null; then
  echo "[Info] Process $PID is not running (stale PID file)"
  rm -f "$PID_FILE"
  echo "[Cleaned] Removed stale PID file"
  exit 0
fi

# Send SIGTERM for graceful shutdown
echo "[Stopping] Sending SIGTERM to $NAME (PID: $PID)..."
kill "$PID" 2>/dev/null || true

# Wait for process to terminate (up to 3 seconds)
WAIT_COUNT=0
MAX_WAIT=30
while kill -0 "$PID" 2>/dev/null; do
  WAIT_COUNT=$((WAIT_COUNT + 1))
  if [ "$WAIT_COUNT" -ge "$MAX_WAIT" ]; then
    echo "[Warning] Process did not terminate gracefully, sending SIGKILL..."
    kill -9 "$PID" 2>/dev/null || true
    sleep 0.5
    break
  fi
  sleep 0.1
done

# Verify process stopped
if kill -0 "$PID" 2>/dev/null; then
  echo "[Error] Failed to stop $NAME (PID: $PID)"
  exit 1
fi

# Clean up PID file
rm -f "$PID_FILE"
echo "[Stopped] $NAME has been stopped successfully"