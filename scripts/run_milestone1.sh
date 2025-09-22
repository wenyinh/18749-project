#!/usr/bin/env sh
set -eu

# Get script and project directories
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="$ROOT_DIR/.env"
RUN_SCRIPT="$SCRIPT_DIR/run.sh"

# Load environment variables
if [ -f "$ENV_FILE" ]; then
  . "$ENV_FILE"
else
  echo "[Error] Missing .env file. Please copy .env.example to .env and configure it." >&2
  exit 1
fi

# Check if run.sh exists
if [ ! -f "$RUN_SCRIPT" ]; then
  echo "[Error] Missing run.sh script" >&2
  exit 1
fi

echo "==========================================="
echo "Starting Milestone 1 Components"
echo "==========================================="

# Start server
echo ""
echo "1. Starting server..."
"$RUN_SCRIPT" server server \
  -addr "$SERVER_ADDR" \
  -rid "$SERVER_REPLICA_ID" \
  -init_state "$SERVER_INIT_STATE"

# Give server time to start
sleep 1

# Start clients (long-running)
echo ""
echo "2. Starting long-running clients..."

echo "Starting client1..."
"$RUN_SCRIPT" client client1 \
  -id "$CLIENT1_ID" \
  -server "$SERVER_ADDR"

sleep 0.5

echo "Starting client2..."
"$RUN_SCRIPT" client client2 \
  -id "$CLIENT2_ID" \
  -server "$SERVER_ADDR"

sleep 0.5

echo "Starting client3..."
"$RUN_SCRIPT" client client3 \
  -id "$CLIENT3_ID" \
  -server "$SERVER_ADDR"

sleep 0.5

echo "All clients started and running"

# Start LFD
echo ""
echo "3. Starting Local Fault Detector..."
"$RUN_SCRIPT" lfd lfd \
  -target "$LFD_TARGET_ADDR" \
  -hb "$LFD_HB_FREQ" \
  -timeout "$LFD_TIMEOUT" \
  -id "$LFD_ID"

echo ""
echo "==========================================="
echo "All components started successfully!"
echo "==========================================="
echo ""
echo "Log files:"
echo "  Server: tail -f $LOG_DIR/server.log"
echo "  Client 1: tail -f $LOG_DIR/client1.log"
echo "  Client 2: tail -f $LOG_DIR/client2.log"
echo "  Client 3: tail -f $LOG_DIR/client3.log"
echo "  LFD: tail -f $LOG_DIR/lfd.log"
echo ""
echo "To stop all components: make stop-milestone1"
echo ""