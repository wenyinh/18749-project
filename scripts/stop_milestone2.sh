#!/bin/bash

# Load environment variables
if [ -f .env ]; then
    source .env
fi

PID_DIR=${PID_DIR:-./run}

echo "========================================="
echo "Stopping Milestone 2 Demo"
echo "========================================="

# Function to stop a process
stop_process() {
    local name=$1
    local pid_file="${PID_DIR}/${name}.pid"

    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if kill -0 $pid 2>/dev/null; then
            echo "  Stopping $name (PID: $pid)..."
            kill $pid
            sleep 1
            # Force kill if still running
            if kill -0 $pid 2>/dev/null; then
                echo "    Force killing $name..."
                kill -9 $pid
            fi
        else
            echo "  $name (PID: $pid) is not running"
        fi
        rm -f "$pid_file"
    else
        echo "  $name: no PID file found"
    fi
}

# Stop all components in reverse order
echo ""
echo "Stopping clients..."
stop_process "client1"
stop_process "client2"
stop_process "client3"

echo ""
echo "Stopping LFDs..."
stop_process "lfd1"
stop_process "lfd2"
stop_process "lfd3"

echo ""
echo "Stopping servers..."
stop_process "server1"
stop_process "server2"
stop_process "server3"

echo ""
echo "Stopping GFD..."
stop_process "gfd"

echo ""
echo "========================================="
echo "Milestone 2 Demo Stopped"
echo "========================================="
echo ""
