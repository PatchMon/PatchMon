#!/bin/sh

# Enable strict error handling
set -e

# Function to log messages with timestamp
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*" >&2
}

# Main execution
log "PatchMon Backend Container Starting..."

# Migrations run inside the Go binary at startup
log "Starting application..."
exec ./patchmon-server
