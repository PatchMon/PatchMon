#!/bin/sh

# Enable strict error handling
set -e

# Function to log messages with timestamp
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*" >&2
}

# Function to extract version from agent script (legacy)
get_agent_version() {
    local file="$1"
    if [ -f "$file" ]; then
        grep -m 1 '^AGENT_VERSION=' "$file" | cut -d'"' -f2 2>/dev/null || echo "0.0.0"
    else
        echo "0.0.0"
    fi
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

# Check and update agent files if necessary
update_agents() {
    local backup_agent="/app/agents_backup/patchmon-agent.sh"
    local current_agent="/app/agents/patchmon-agent.sh"
    local backup_binary="/app/agents_backup/patchmon-agent-linux-amd64"
    local current_binary="/app/agents/patchmon-agent-linux-amd64"
    
    # Check if agents directory exists
    if [ ! -d "/app/agents" ]; then
        log "ERROR: /app/agents directory not found"
        return 1
    fi
    
    # Check if backup exists
    if [ ! -d "/app/agents_backup" ]; then
        log "WARNING: agents_backup directory not found, skipping agent update"
        return 0
    fi
    
    # Get versions from both script and binary
    local backup_script_version=$(get_agent_version "$backup_agent")
    local current_script_version=$(get_agent_version "$current_agent")
    local backup_binary_version=$(get_binary_version "$backup_binary")
    local current_binary_version=$(get_binary_version "$current_binary")
    
    log "Agent version check:"
    log "  Image script version: ${backup_script_version}"
    log "  Volume script version: ${current_script_version}"
    log "  Image binary version: ${backup_binary_version}"
    log "  Volume binary version: ${current_binary_version}"
    
    # Determine if update is needed
    local needs_update=0
    
    # Case 1: No agents in volume at all (first time setup)
    if [ -z "$(find /app/agents -maxdepth 1 -type f 2>/dev/null | head -n 1)" ]; then
        log "Agents directory is empty - performing initial copy"
        needs_update=1
    # Case 2: Binary exists but backup binary is newer
    elif [ "$current_binary_version" != "0.0.0" ] && version_greater "$backup_binary_version" "$current_binary_version"; then
        log "Newer agent binary available (${backup_binary_version} > ${current_binary_version})"
        needs_update=1
    # Case 3: No binary in volume, but shell scripts exist (legacy setup) - copy binaries
    elif [ "$current_binary_version" = "0.0.0" ] && [ "$backup_binary_version" != "0.0.0" ]; then
        log "No binary found in volume but backup has binaries - performing update"
        needs_update=1
    else
        log "Agents are up to date (binary: ${current_binary_version})"
        needs_update=0
    fi
    
    # Perform update if needed
    if [ $needs_update -eq 1 ]; then
        log "Updating agents to version ${backup_binary_version}..."
        
        # Create backup of existing agents if they exist
        if [ -f "$current_agent" ] || [ -f "$current_binary" ]; then
            local backup_timestamp=$(date +%Y%m%d_%H%M%S)
            mkdir -p "/app/agents/backups"
            
            # Backup shell script if it exists
            if [ -f "$current_agent" ]; then
                cp "$current_agent" "/app/agents/backups/patchmon-agent.sh.${backup_timestamp}" 2>/dev/null || true
                log "Previous script backed up"
            fi
            
            # Backup binary if it exists
            if [ -f "$current_binary" ]; then
                cp "$current_binary" "/app/agents/backups/patchmon-agent-linux-amd64.${backup_timestamp}" 2>/dev/null || true
                log "Previous binary backed up"
            fi
        fi
        
        # Copy new agents (both scripts and binaries)
        cp -r /app/agents_backup/* /app/agents/
        
        # Make agent binaries executable
        chmod +x /app/agents/patchmon-agent-linux-* 2>/dev/null || true
        
        # Verify update
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

# Update agents (version-aware)
update_agents

log "Running database migrations..."
cd /app/backend
if [ -f "./node_modules/.bin/prisma" ]; then
    ./node_modules/.bin/prisma migrate deploy
else
    # Fallback to npx with explicit version
    npx -y prisma@6.1.0 migrate deploy
fi

log "Starting application..."
if [ "${NODE_ENV}" = "development" ]; then
    exec npm run dev
else
    exec npm start
fi
