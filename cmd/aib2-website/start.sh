#!/bin/bash
# AIB 2.0 Website - Start Script
# Usage: ./start.sh [stop|restart|status]

BINARY="./cmd/aib2-website/aib2-website"
PIDFILE="/tmp/aib2-website.pid"
LOGFILE="/tmp/aib2-website.log"
ADDR=":51234"
ROOT="./cmd/aib2-portal/new"

start() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "aib2-website already running (PID $(cat "$PIDFILE"))"
        return 1
    fi
    echo "Starting aib2-website on $ADDR ..."
    nohup "$BINARY" -addr="$ADDR" -root="$ROOT" >> "$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
    sleep 1
    if kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "Started (PID $(cat "$PIDFILE"))"
    else
        echo "Failed to start. Check $LOGFILE"
        return 1
    fi
}

stop() {
    if [ ! -f "$PIDFILE" ]; then
        # Try to find by process name
        PID=$(pgrep -f "aib2-website" | head -1)
        if [ -z "$PID" ]; then
            echo "aib2-website not running"
            return 0
        fi
    else
        PID=$(cat "$PIDFILE")
    fi
    echo "Stopping aib2-website (PID $PID)..."
    kill "$PID" 2>/dev/null
    sleep 1
    rm -f "$PIDFILE"
    echo "Stopped"
}

status() {
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "aib2-website running (PID $(cat "$PIDFILE"))"
    elif pgrep -f "aib2-website" > /dev/null; then
        echo "aib2-website running (PID $(pgrep -f aib2-website | head -1))"
    else
        echo "aib2-website not running"
    fi
}

case "${1:-start}" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; sleep 1; start ;;
    status)  status ;;
    *)       echo "Usage: $0 {start|stop|restart|status}" ;;
esac
