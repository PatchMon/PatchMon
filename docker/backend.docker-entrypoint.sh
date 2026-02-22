#!/bin/sh

# Enable strict error handling
set -e

# Function to log messages with timestamp
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*" >&2
}

# Function to get version from binary using --help flag
get_binary_version() {
    local binary="$1"
    if [ -f "$binary" ]; then
        # Make sure binary is executable
        chmod +x "$binary" 2>/dev/null || true
        
        # Try to execute the binary and extract version from help output
        # The Go binary shows version in the --help output as "PatchMon Agent v1.3.0"
        local version=$("$binary" --help 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 | tr -d 'v')
        if [ -n "$version" ]; then
            echo "$version"
        else
            # Fallback: try --version flag
            version=$("$binary" --version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1)
            if [ -n "$version" ]; then
                echo "$version"
            else
                echo "0.0.0"
            fi
        fi
    else
        echo "0.0.0"
    fi
}

# Function to compare versions (returns 0 if $1 > $2)
version_greater() {
    # Use sort -V for version comparison
    test "$(printf '%s\n' "$1" "$2" | sort -V | tail -n1)" = "$1" && test "$1" != "$2"
}

# Check and update agent binaries if necessary
update_agents() {
    local backup_binary="/app/agents_backup/patchmon-agent-linux-amd64"
    local current_binary="/app/agents/patchmon-agent-linux-amd64"

    if [ ! -d "/app/agents" ]; then
        log "ERROR: /app/agents directory not found"
        return 1
    fi

    if [ ! -d "/app/agents_backup" ]; then
        log "WARNING: agents_backup directory not found, skipping agent update"
        return 0
    fi

    local backup_binary_version=$(get_binary_version "$backup_binary")
    local current_binary_version=$(get_binary_version "$current_binary")

    log "Agent version check:"
    log "  Image binary version: ${backup_binary_version}"
    log "  Volume binary version: ${current_binary_version}"

    local needs_update=0

    if [ -z "$(find /app/agents -maxdepth 1 -type f 2>/dev/null | head -n 1)" ]; then
        log "Agents directory is empty - performing initial copy"
        needs_update=1
    elif [ "$current_binary_version" != "0.0.0" ] && version_greater "$backup_binary_version" "$current_binary_version"; then
        log "Newer agent binary available (${backup_binary_version} > ${current_binary_version})"
        needs_update=1
    elif [ "$current_binary_version" = "0.0.0" ] && [ "$backup_binary_version" != "0.0.0" ]; then
        log "No binary found in volume but backup has binaries - performing update"
        needs_update=1
    else
        log "Agents are up to date (binary: ${current_binary_version})"
        needs_update=0
    fi

    if [ $needs_update -eq 1 ]; then
        log "Updating agents to version ${backup_binary_version}..."

        if [ -f "$current_binary" ]; then
            local backup_timestamp=$(date +%Y%m%d_%H%M%S)
            mkdir -p "/app/agents/backups"
            cp "$current_binary" "/app/agents/backups/patchmon-agent-linux-amd64.${backup_timestamp}" 2>/dev/null || true
            log "Previous binary backed up"
        fi

        cp -r /app/agents_backup/* /app/agents/

        chmod +x /app/agents/patchmon-agent-linux-* 2>/dev/null || true
        chmod +x /app/agents/patchmon-agent-freebsd-* 2>/dev/null || true

        local new_binary_version=$(get_binary_version "$current_binary")
        if [ "$new_binary_version" = "$backup_binary_version" ]; then
            log "✅ Agents successfully updated to version ${new_binary_version}"
        else
            log "⚠️ Warning: Agent update may have failed (expected: ${backup_binary_version}, got: ${new_binary_version})"
        fi
    fi
}

# Main execution
log "PatchMon Backend Container Starting..."
log "Environment: ${NODE_ENV:-production}"

# Add workspace node_modules to PATH for workspace binaries and set NODE_PATH for module resolution
export PATH="/app/node_modules/.bin:$PATH"
export NODE_PATH="/app/node_modules:/app/backend/node_modules:$NODE_PATH"

# Update agents (version-aware)
update_agents

# Check if ASSETS_DIR is set and ensure it's writable (for custom branding support)
if [ -n "$ASSETS_DIR" ]; then
    if [ -d "$ASSETS_DIR" ]; then
        # Automatically fix permissions if directory exists but isn't writable
        if [ ! -w "$ASSETS_DIR" ]; then
            log "⚠️  Assets directory is NOT writable: $ASSETS_DIR"
            log "   Attempting to fix permissions automatically..."
            # Try to make directory writable (works if we own it or it's world-writable)
            # Note: We run as user 1000, so we can't chown, but we can chmod if we have access
            chmod -R u+w "$ASSETS_DIR" 2>/dev/null || \
                chmod -R a+w "$ASSETS_DIR" 2>/dev/null || true
        fi
        # Final check - verify it's now writable
        if [ -w "$ASSETS_DIR" ]; then
            log "✅ Assets directory is writable: $ASSETS_DIR"
        else
            log "⚠️  WARNING: Assets directory is still NOT writable: $ASSETS_DIR"
            log "   Custom branding (logo uploads) may fail."
            log "   The frontend container should fix this on startup, or manually run:"
            log "   docker exec -u root <backend-container> chown -R 1000:1000 $ASSETS_DIR && chmod 755 $ASSETS_DIR"
        fi
    else
        log "⚠️  WARNING: Assets directory does not exist: $ASSETS_DIR"
        log "   Attempting to create it..."
        if mkdir -p "$ASSETS_DIR" 2>/dev/null; then
            log "✅ Created assets directory: $ASSETS_DIR"
            # Set correct permissions on newly created directory (we own it, so this works)
            chmod 755 "$ASSETS_DIR" 2>/dev/null || true
        else
            log "❌ ERROR: Failed to create assets directory. Custom branding will not work."
        fi
    fi
fi

log "Running database migrations..."
cd /app/backend && npx prisma migrate deploy

log "Starting application..."
if [ "${NODE_ENV}" = "development" ]; then
    cd /app && exec npm run --workspace=backend dev
else
    cd /app && exec npm run --workspace=backend start
fi
