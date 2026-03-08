#!/bin/bash
# E-Ink Picture — Start/Stop/Status/Logs Management
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOGS_DIR="$SCRIPT_DIR/logs"
SERVER_PID_FILE="/tmp/eink_server.pid"
CLIENT_PID_FILE="/tmp/eink_client.pid"
SERVER_BIN="$SCRIPT_DIR/server/eink-server"
CLIENT_SCRIPT="$SCRIPT_DIR/client/client.py"
VENV="$SCRIPT_DIR/venv"
ENV_FILE="$SCRIPT_DIR/.env"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Load environment variables
load_env() {
    if [ -f "$ENV_FILE" ]; then
        set -a
        source "$ENV_FILE"
        set +a
    fi
    # Override DATA_DIR to local path
    export DATA_DIR="$SCRIPT_DIR/data"
    export EINK_SERVER_URL="${EINK_SERVER_URL:-http://localhost:${PORT:-5000}}"
}

is_running() {
    local pid_file="$1"
    if [ -f "$pid_file" ]; then
        local pid
        pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            return 0
        fi
        # Stale PID file
        rm -f "$pid_file"
    fi
    return 1
}

start_server() {
    if is_running "$SERVER_PID_FILE"; then
        echo -e "${YELLOW}Server already running (PID $(cat "$SERVER_PID_FILE"))${NC}"
        return 0
    fi

    echo -n "Starting server... "
    mkdir -p "$LOGS_DIR"

    load_env

    nohup "$SERVER_BIN" >> "$LOGS_DIR/server.log" 2>&1 &
    local pid=$!
    echo "$pid" > "$SERVER_PID_FILE"

    # Wait for server to be ready (max 30s)
    for i in $(seq 1 30); do
        if curl -sf "http://localhost:${PORT:-5000}/health" > /dev/null 2>&1; then
            echo -e "${GREEN}running (PID $pid)${NC}"
            return 0
        fi
        # Check if process died
        if ! kill -0 "$pid" 2>/dev/null; then
            echo -e "${RED}failed${NC}"
            echo "Check logs: $LOGS_DIR/server.log"
            rm -f "$SERVER_PID_FILE"
            return 1
        fi
        sleep 1
    done

    echo -e "${YELLOW}started (PID $pid) but health check not responding yet${NC}"
}

start_client() {
    if is_running "$CLIENT_PID_FILE"; then
        echo -e "${YELLOW}Client already running (PID $(cat "$CLIENT_PID_FILE"))${NC}"
        return 0
    fi

    if [ ! -f "$VENV/bin/python3" ]; then
        echo -e "${RED}Virtual environment not found. Run ./setup.sh first.${NC}"
        return 1
    fi

    echo -n "Starting client... "
    mkdir -p "$LOGS_DIR"

    load_env

    cd "$SCRIPT_DIR/client"
    nohup "$VENV/bin/python3" "$CLIENT_SCRIPT" >> "$LOGS_DIR/client.log" 2>&1 &
    local pid=$!
    echo "$pid" > "$CLIENT_PID_FILE"
    cd "$SCRIPT_DIR"

    sleep 2
    if kill -0 "$pid" 2>/dev/null; then
        echo -e "${GREEN}running (PID $pid)${NC}"
    else
        echo -e "${RED}failed${NC}"
        echo "Check logs: $LOGS_DIR/client.log"
        rm -f "$CLIENT_PID_FILE"
        return 1
    fi
}

stop_process() {
    local name="$1"
    local pid_file="$2"

    if ! is_running "$pid_file"; then
        echo -e "$name: ${YELLOW}not running${NC}"
        return 0
    fi

    local pid
    pid=$(cat "$pid_file")
    echo -n "Stopping $name (PID $pid)... "

    kill "$pid" 2>/dev/null || true

    # Wait for graceful shutdown (max 10s)
    for i in $(seq 1 10); do
        if ! kill -0 "$pid" 2>/dev/null; then
            rm -f "$pid_file"
            echo -e "${GREEN}stopped${NC}"
            return 0
        fi
        sleep 1
    done

    # Force kill
    kill -9 "$pid" 2>/dev/null || true
    rm -f "$pid_file"
    echo -e "${YELLOW}killed${NC}"
}

cmd_start() {
    start_server || return 1
    start_client || true  # Client may fail without hardware
    echo ""
    IP=$(hostname -I 2>/dev/null | awk '{print $1}')
    echo "Designer: http://${IP:-localhost}:${PORT:-5000}/designer"
}

cmd_stop() {
    stop_process "Client" "$CLIENT_PID_FILE"
    stop_process "Server" "$SERVER_PID_FILE"
}

cmd_restart() {
    cmd_stop
    sleep 1
    cmd_start
}

cmd_status() {
    echo "=== E-Ink Picture Status ==="
    echo ""

    if is_running "$SERVER_PID_FILE"; then
        echo -e "Server:  ${GREEN}running${NC} (PID $(cat "$SERVER_PID_FILE"))"
        local port="${PORT:-5000}"
        if curl -sf "http://localhost:$port/health" > /dev/null 2>&1; then
            echo -e "  Health: ${GREEN}OK${NC}"
        else
            echo -e "  Health: ${RED}not responding${NC}"
        fi
    else
        echo -e "Server:  ${RED}stopped${NC}"
    fi

    if is_running "$CLIENT_PID_FILE"; then
        echo -e "Client:  ${GREEN}running${NC} (PID $(cat "$CLIENT_PID_FILE"))"
    else
        echo -e "Client:  ${RED}stopped${NC}"
    fi

    echo ""
    if [ -f "$LOGS_DIR/server.log" ]; then
        echo "--- Last 5 server log lines ---"
        tail -5 "$LOGS_DIR/server.log" 2>/dev/null || true
    fi
    echo ""
    if [ -f "$LOGS_DIR/client.log" ]; then
        echo "--- Last 5 client log lines ---"
        tail -5 "$LOGS_DIR/client.log" 2>/dev/null || true
    fi
}

cmd_logs() {
    echo "Following logs (Ctrl+C to stop)..."
    tail -f "$LOGS_DIR/server.log" "$LOGS_DIR/client.log" 2>/dev/null
}

# ----- Main -----
case "${1:-}" in
    start)   cmd_start ;;
    stop)    cmd_stop ;;
    restart) cmd_restart ;;
    status)  cmd_status ;;
    logs)    cmd_logs ;;
    *)
        echo "Usage: $0 {start|stop|restart|status|logs}"
        echo ""
        echo "  start    Start server and client"
        echo "  stop     Stop all processes"
        echo "  restart  Restart all processes"
        echo "  status   Show process status and recent logs"
        echo "  logs     Follow log output (tail -f)"
        exit 1
        ;;
esac
