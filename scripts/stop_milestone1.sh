#!/usr/bin/env sh
set -eu

# Get script and project directories
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
STOP_SCRIPT="$SCRIPT_DIR/stop.sh"

# Check if stop.sh exists
if [ ! -f "$STOP_SCRIPT" ]; then
  echo "[Error] Missing stop.sh script" >&2
  exit 1
fi

echo "==========================================="
echo "Stopping Milestone 1 Components"
echo "==========================================="
echo ""

# Stop components in reverse order (LFD, clients, then server)
echo "Stopping LFD..."
"$STOP_SCRIPT" lfd 2>/dev/null || echo "[Info] lfd was not running"

echo "Stopping clients..."
"$STOP_SCRIPT" client3 2>/dev/null || echo "[Info] client3 was not running"
"$STOP_SCRIPT" client2 2>/dev/null || echo "[Info] client2 was not running"
"$STOP_SCRIPT" client1 2>/dev/null || echo "[Info] client1 was not running"

echo "Stopping server..."
"$STOP_SCRIPT" server 2>/dev/null || echo "[Info] server was not running"

echo ""
echo "==========================================="
echo "Milestone 1 components stopped"
echo "==========================================="
echo ""