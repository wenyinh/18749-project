#!/bin/bash

# Load environment variables
if [ -f .env ]; then
    source .env
fi

# Create necessary directories
mkdir -p ${LOG_DIR:-./logs}
mkdir -p ${PID_DIR:-./run}

echo "========================================="
echo "Starting Milestone 2 Demo"
echo "========================================="

# Step 1: Start GFD
echo ""
echo "[1/10] Starting GFD..."
./bin/gfd -addr ${GFD_ADDR:-127.0.0.1:8000} > ${LOG_DIR:-./logs}/gfd.log 2>&1 &
GFD_PID=$!
echo $GFD_PID > ${PID_DIR:-./run}/gfd.pid
echo "  GFD started (PID: $GFD_PID)"
sleep 2

# Step 2-4: Start 3 Servers
echo ""
echo "[2/10] Starting Server S1..."
./bin/server -addr ${SERVER1_ADDR:-127.0.0.1:9001} -rid ${SERVER1_ID:-S1} -init_state ${SERVER1_INIT_STATE:-0} > ${LOG_DIR:-./logs}/server1.log 2>&1 &
S1_PID=$!
echo $S1_PID > ${PID_DIR:-./run}/server1.pid
echo "  Server S1 started (PID: $S1_PID, Address: ${SERVER1_ADDR:-127.0.0.1:9001})"
sleep 1

echo ""
echo "[3/10] Starting Server S2..."
./bin/server -addr ${SERVER2_ADDR:-127.0.0.1:9002} -rid ${SERVER2_ID:-S2} -init_state ${SERVER2_INIT_STATE:-0} > ${LOG_DIR:-./logs}/server2.log 2>&1 &
S2_PID=$!
echo $S2_PID > ${PID_DIR:-./run}/server2.pid
echo "  Server S2 started (PID: $S2_PID, Address: ${SERVER2_ADDR:-127.0.0.1:9002})"
sleep 1

echo ""
echo "[4/10] Starting Server S3..."
./bin/server -addr ${SERVER3_ADDR:-127.0.0.1:9003} -rid ${SERVER3_ID:-S3} -init_state ${SERVER3_INIT_STATE:-0} > ${LOG_DIR:-./logs}/server3.log 2>&1 &
S3_PID=$!
echo $S3_PID > ${PID_DIR:-./run}/server3.pid
echo "  Server S3 started (PID: $S3_PID, Address: ${SERVER3_ADDR:-127.0.0.1:9003})"
sleep 1

# Step 5-7: Start 3 LFDs
echo ""
echo "[5/10] Starting LFD1 (monitoring S1)..."
./bin/lfd \
    -target ${LFD1_TARGET:-127.0.0.1:9001} \
    -id ${LFD1_ID:-S1} \
    -gfd ${LFD1_GFD:-127.0.0.1:8000} \
    -hb ${LFD1_HB_FREQ:-1s} \
    -timeout ${LFD1_TIMEOUT:-3s} \
    -max-retries ${LFD1_MAX_RETRIES:-5} \
    -base-delay ${LFD1_BASE_DELAY:-1s} \
    -max-delay ${LFD1_MAX_DELAY:-30s} \
    > ${LOG_DIR:-./logs}/lfd1.log 2>&1 &
LFD1_PID=$!
echo $LFD1_PID > ${PID_DIR:-./run}/lfd1.pid
echo "  LFD1 started (PID: $LFD1_PID)"
sleep 1

echo ""
echo "[6/10] Starting LFD2 (monitoring S2)..."
./bin/lfd \
    -target ${LFD2_TARGET:-127.0.0.1:9002} \
    -id ${LFD2_ID:-S2} \
    -gfd ${LFD2_GFD:-127.0.0.1:8000} \
    -hb ${LFD2_HB_FREQ:-1s} \
    -timeout ${LFD2_TIMEOUT:-3s} \
    -max-retries ${LFD2_MAX_RETRIES:-5} \
    -base-delay ${LFD2_BASE_DELAY:-1s} \
    -max-delay ${LFD2_MAX_DELAY:-30s} \
    > ${LOG_DIR:-./logs}/lfd2.log 2>&1 &
LFD2_PID=$!
echo $LFD2_PID > ${PID_DIR:-./run}/lfd2.pid
echo "  LFD2 started (PID: $LFD2_PID)"
sleep 1

echo ""
echo "[7/10] Starting LFD3 (monitoring S3)..."
./bin/lfd \
    -target ${LFD3_TARGET:-127.0.0.1:9003} \
    -id ${LFD3_ID:-S3} \
    -gfd ${LFD3_GFD:-127.0.0.1:8000} \
    -hb ${LFD3_HB_FREQ:-1s} \
    -timeout ${LFD3_TIMEOUT:-3s} \
    -max-retries ${LFD3_MAX_RETRIES:-5} \
    -base-delay ${LFD3_BASE_DELAY:-1s} \
    -max-delay ${LFD3_MAX_DELAY:-30s} \
    > ${LOG_DIR:-./logs}/lfd3.log 2>&1 &
LFD3_PID=$!
echo $LFD3_PID > ${PID_DIR:-./run}/lfd3.pid
echo "  LFD3 started (PID: $LFD3_PID)"
sleep 2

# Step 8-10: Start 3 Clients with auto-send mode
echo ""
echo "[8/10] Starting Client C1 (auto mode)..."
./bin/client \
    -id ${CLIENT1_ID:-C1} \
    -servers "${CLIENT1_SERVERS:-S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003}" \
    -interval ${CLIENT1_INTERVAL:-2s} \
    -auto \
    > ${LOG_DIR:-./logs}/client1.log 2>&1 &
C1_PID=$!
echo $C1_PID > ${PID_DIR:-./run}/client1.pid
echo "  Client C1 started (PID: $C1_PID)"
sleep 1

echo ""
echo "[9/10] Starting Client C2 (auto mode)..."
./bin/client \
    -id ${CLIENT2_ID:-C2} \
    -servers "${CLIENT2_SERVERS:-S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003}" \
    -interval ${CLIENT2_INTERVAL:-2s} \
    -auto \
    > ${LOG_DIR:-./logs}/client2.log 2>&1 &
C2_PID=$!
echo $C2_PID > ${PID_DIR:-./run}/client2.pid
echo "  Client C2 started (PID: $C2_PID)"
sleep 1

echo ""
echo "[10/10] Starting Client C3 (auto mode)..."
./bin/client \
    -id ${CLIENT3_ID:-C3} \
    -servers "${CLIENT3_SERVERS:-S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003}" \
    -interval ${CLIENT3_INTERVAL:-2s} \
    -auto \
    > ${LOG_DIR:-./logs}/client3.log 2>&1 &
C3_PID=$!
echo $C3_PID > ${PID_DIR:-./run}/client3.pid
echo "  Client C3 started (PID: $C3_PID)"

echo ""
echo "========================================="
echo "Milestone 2 Demo Started Successfully!"
echo "========================================="
echo ""
echo "Components running:"
echo "  GFD:      PID $GFD_PID (logs: ${LOG_DIR:-./logs}/gfd.log)"
echo "  Server S1: PID $S1_PID (logs: ${LOG_DIR:-./logs}/server1.log)"
echo "  Server S2: PID $S2_PID (logs: ${LOG_DIR:-./logs}/server2.log)"
echo "  Server S3: PID $S3_PID (logs: ${LOG_DIR:-./logs}/server3.log)"
echo "  LFD1:     PID $LFD1_PID (logs: ${LOG_DIR:-./logs}/lfd1.log)"
echo "  LFD2:     PID $LFD2_PID (logs: ${LOG_DIR:-./logs}/lfd2.log)"
echo "  LFD3:     PID $LFD3_PID (logs: ${LOG_DIR:-./logs}/lfd3.log)"
echo "  Client C1: PID $C1_PID (logs: ${LOG_DIR:-./logs}/client1.log)"
echo "  Client C2: PID $C2_PID (logs: ${LOG_DIR:-./logs}/client2.log)"
echo "  Client C3: PID $C3_PID (logs: ${LOG_DIR:-./logs}/client3.log)"
echo ""
echo "To view logs in real-time:"
echo "  tail -f ${LOG_DIR:-./logs}/gfd.log"
echo "  tail -f ${LOG_DIR:-./logs}/server1.log"
echo "  tail -f ${LOG_DIR:-./logs}/client1.log"
echo ""
echo "To stop all components:"
echo "  make stop-milestone2"
echo ""
