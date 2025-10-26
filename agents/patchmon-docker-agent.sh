#!/bin/bash

# PatchMon Docker Agent Script v1.3.0
# This script collects Docker container and image information and sends it to PatchMon

# Configuration
PATCHMON_SERVER="${PATCHMON_SERVER:-http://localhost:3001}"
API_VERSION="v1"
AGENT_VERSION="1.3.0"
CONFIG_FILE="/etc/patchmon/config.yml"
CREDENTIALS_FILE="/etc/patchmon/credentials.yml"
LOG_FILE="/var/log/patchmon-docker-agent.log"

# Curl flags placeholder (replaced by server based on SSL settings)
CURL_FLAGS=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    if [[ -w "$(dirname "$LOG_FILE")" ]] 2>/dev/null; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE" 2>/dev/null
    fi
}

# Error handling
error() {
    echo -e "${RED}ERROR: $1${NC}" >&2
    log "ERROR: $1"
    exit 1
}

# Info logging
info() {
    echo -e "${BLUE}ℹ️  $1${NC}" >&2
    log "INFO: $1"
}

# Success logging
success() {
    echo -e "${GREEN}✅ $1${NC}" >&2
    log "SUCCESS: $1"
}

# Warning logging
warning() {
    echo -e "${YELLOW}⚠️  $1${NC}" >&2
    log "WARNING: $1"
}

# Check if Docker is installed and running
check_docker() {
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed on this system"
    fi
    
    if ! docker info &> /dev/null; then
        error "Docker daemon is not running or you don't have permission to access it. Try running with sudo."
    fi
}

# Check for yq (YAML processor) presence
# This function is crucial as yq is a dependency.
check_yq_installed() {
    if ! command -v yq &> /dev/null; then
        error "yq (YAML processor) is not installed. Please install it to parse YAML files."
        echo "Refer to https://github.com/mikefarah/yq for installation instructions." >&2
    fi
}

# Load credentials from YAML file
load_credentials() {
    if [[ ! -f "$CREDENTIALS_FILE" ]]; then
        error "Credentials file not found at $CREDENTIALS_FILE. Please configure the main PatchMon agent first."
    fi
    
    # Extract API_ID with raw output (-r)
    API_ID=$(yq -r '.api_id' "$CREDENTIALS_FILE")
    if [[ $? -ne 0 ]] || [[ "$API_ID" == "null" ]] || [[ -z "$API_ID" ]]; then
        error "API_ID not found, is 'null', or is empty in $CREDENTIALS_FILE"
    fi

    # Extract API_KEY with raw output (-r)
    API_KEY=$(yq -r '.api_key' "$CREDENTIALS_FILE")
    if [[ $? -ne 0 ]] || [[ "$API_KEY" == "null" ]] || [[ -z "$API_KEY" ]]; then
        error "API_KEY not found, is 'null', or is empty in $CREDENTIALS_FILE"
    fi
}

# Load configuration from YAML file
load_config() {
    if [[ -f "$CONFIG_FILE" ]]; then
        # Extract optional patchmon_server from config file with raw output (-r)
        local extracted_patchmon_server=$(yq -r '.patchmon_server // null' "$CONFIG_FILE")
        if [[ $? -eq 0 ]] && [[ "$extracted_patchmon_server" != "null" ]] && [[ -n "$extracted_patchmon_server" ]]; then
            PATCHMON_SERVER="$extracted_patchmon_server"
        fi
    fi
}


# Collect Docker containers
collect_containers() {
    info "Collecting Docker container information..."
    
    local containers_json="["
    local first=true
    
    # Get all containers (running and stopped)
    while IFS='|' read -r container_id name image status state created started ports; do
        if [[ -z "$container_id" ]]; then
            continue
        fi
        
        # Parse image name and tag
        local image_name="${image%%:*}"
        local image_tag="${image##*:}"
        if [[ "$image_tag" == "$image_name" ]]; then
            image_tag="latest"
        fi
        
        # Determine image source based on registry
        local image_source="docker-hub"
        if [[ "$image_name" == ghcr.io/* ]]; then
            image_source="github"
        elif [[ "$image_name" == registry.gitlab.com/* ]]; then
            image_source="gitlab"
        elif [[ "$image_name" == *"/"*"/"* ]]; then
            image_source="private"
        fi
        
        # Get repository name (without registry prefix for common registries)
        local image_repository="$image_name"
        image_repository="${image_repository#ghcr.io/}"
        image_repository="${image_repository#registry.gitlab.com/}"
        
        # Get image ID
        local full_image_id=$(docker inspect --format='{{.Image}}' "$container_id" 2>/dev/null || echo "unknown")
        full_image_id="${full_image_id#sha256:}"
        
        # Normalize status (extract just the status keyword)
        local normalized_status="unknown"
        if [[ "$status" =~ ^Up ]]; then
            normalized_status="running"
        elif [[ "$status" =~ ^Exited ]]; then
            normalized_status="exited"
        elif [[ "$status" =~ ^Created ]]; then
            normalized_status="created"
        elif [[ "$status" =~ ^Restarting ]]; then
            normalized_status="restarting"
        elif [[ "$status" =~ ^Paused ]]; then
            normalized_status="paused"
        elif [[ "$status" =~ ^Dead ]]; then
            normalized_status="dead"
        fi
        
        # Parse ports
        local ports_json="null"
        if [[ -n "$ports" && "$ports" != "null" ]]; then
            # Convert Docker port format to JSON
            ports_json=$(echo "$ports" | jq -R -s -c 'split(",") | map(select(length > 0)) | map(split("->") | {(.[0]): .[1]}) | add // {}')
        fi
        
        # Convert dates to ISO 8601 format
        # If date conversion fails, use null instead of invalid date string
        local created_iso=$(date -d "$created" -Iseconds 2>/dev/null || echo "null")
        local started_iso="null"
        if [[ -n "$started" && "$started" != "null" ]]; then
            started_iso=$(date -d "$started" -Iseconds 2>/dev/null || echo "null")
        fi
        
        # Add comma for JSON array
        if [[ "$first" == false ]]; then
            containers_json+=","
        fi
        first=false
        
        # Build JSON object for this container
        containers_json+="{\"container_id\":\"$container_id\","
        containers_json+="\"name\":\"$name\","
        containers_json+="\"image_name\":\"$image_name\","
        containers_json+="\"image_tag\":\"$image_tag\","
        containers_json+="\"image_repository\":\"$image_repository\","
        containers_json+="\"image_source\":\"$image_source\","
        containers_json+="\"image_id\":\"$full_image_id\","
        containers_json+="\"status\":\"$normalized_status\","
        containers_json+="\"state\":\"$state\","
        containers_json+="\"ports\":$ports_json"
        
        # Only add created_at if we have a valid date
        if [[ "$created_iso" != "null" ]]; then
            containers_json+=",\"created_at\":\"$created_iso\""
        fi
        
        # Only add started_at if we have a valid date
        if [[ "$started_iso" != "null" ]]; then
            containers_json+=",\"started_at\":\"$started_iso\""
        fi
        
        containers_json+="}"
        
    done < <(docker ps -a --format '{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}|{{.State}}|{{.CreatedAt}}|{{.RunningFor}}|{{.Ports}}' 2>/dev/null)
    
    containers_json+="]"
    
    echo "$containers_json"
}

# Collect Docker images
collect_images() {
    info "Collecting Docker image information..."
    
    local images_json="["
    local first=true
    
    while IFS='|' read -r repository tag image_id created size digest; do
        if [[ -z "$repository" || "$repository" == "<none>" ]]; then
            continue
        fi
        
        # Clean up tag
        if [[ -z "$tag" || "$tag" == "<none>" ]]; then
            tag="latest"
        fi
        
        # Clean image ID
        image_id="${image_id#sha256:}"
        
        # Determine source
        local source="docker-hub"
        if [[ "$repository" == ghcr.io/* ]]; then
            source="github"
        elif [[ "$repository" == registry.gitlab.com/* ]]; then
            source="gitlab"
        elif [[ "$repository" == *"/"*"/"* ]]; then
            source="private"
        fi
        
        # Convert size to bytes (approximate)
        local size_bytes=0
        if [[ "$size" =~ ([0-9.]+)([KMGT]?B) ]]; then
            local num="${BASH_REMATCH[1]}"
            local unit="${BASH_REMATCH[2]}"
            case "$unit" in
                KB) size_bytes=$(echo "$num * 1024" | bc | cut -d. -f1) ;;
                MB) size_bytes=$(echo "$num * 1024 * 1024" | bc | cut -d. -f1) ;;
                GB) size_bytes=$(echo "$num * 1024 * 1024 * 1024" | bc | cut -d. -f1) ;;
                TB) size_bytes=$(echo "$num * 1024 * 1024 * 1024 * 1024" | bc | cut -d. -f1) ;;
                B) size_bytes=$(echo "$num" | cut -d. -f1) ;;
            esac
        fi
        
        # Convert created date to ISO 8601
        # If date conversion fails, use null instead of invalid date string
        local created_iso=$(date -d "$created" -Iseconds 2>/dev/null || echo "null")
        
        # Add comma for JSON array
        if [[ "$first" == false ]]; then
            images_json+=","
        fi
        first=false
        
        # Build JSON object for this image
        images_json+="{\"repository\":\"$repository\","
        images_json+="\"tag\":\"$tag\","
        images_json+="\"image_id\":\"$image_id\","
        images_json+="\"source\":\"$source\","
        images_json+="\"size_bytes\":$size_bytes"
        
        # Only add created_at if we have a valid date
        if [[ "$created_iso" != "null" ]]; then
            images_json+=",\"created_at\":\"$created_iso\""
        fi
        
        # Only add digest if present
        if [[ -n "$digest" && "$digest" != "<none>" ]]; then
            images_json+=",\"digest\":\"$digest\""
        fi
        
        images_json+="}"
        
    done < <(docker images --format '{{.Repository}}|{{.Tag}}|{{.ID}}|{{.CreatedAt}}|{{.Size}}|{{.Digest}}' --no-trunc 2>/dev/null)
    
    images_json+="]"
    
    echo "$images_json"
}

# Check for image updates
check_image_updates() {
    info "Checking for image updates..."
    
    local updates_json="["
    local first=true
    local update_count=0
    
    # Get all images
    while IFS='|' read -r repository tag image_id digest; do
        if [[ -z "$repository" || "$repository" == "<none>" || "$tag" == "<none>" ]]; then
            continue
        fi
        
        # Skip checking 'latest' tag as it's always considered current by name
        # We'll still check digest though
        local full_image="${repository}:${tag}"
        
        # Try to get remote digest from registry
        # Use docker manifest inspect to avoid pulling the image
        local remote_digest=$(docker manifest inspect "$full_image" 2>/dev/null | jq -r '.config.digest // .manifests[0].digest // empty' 2>/dev/null)
        
        if [[ -z "$remote_digest" ]]; then
            # If manifest inspect fails, try buildx imagetools inspect (works for more registries)
            remote_digest=$(docker buildx imagetools inspect "$full_image" 2>/dev/null | grep -oP 'Digest:\s*\K\S+' | head -1)
        fi
        
        # Clean up digests for comparison
        local local_digest="${digest#sha256:}"
        remote_digest="${remote_digest#sha256:}"
        
        # If we got a remote digest and it's different from local, there's an update
        if [[ -n "$remote_digest" && -n "$local_digest" && "$remote_digest" != "$local_digest" ]]; then
            if [[ "$first" == false ]]; then
                updates_json+=","
            fi
            first=false
            
            # Build update JSON object
            updates_json+="{\"repository\":\"$repository\","
            updates_json+="\"current_tag\":\"$tag\","
            updates_json+="\"available_tag\":\"$tag\","
            updates_json+="\"current_digest\":\"$local_digest\","
            updates_json+="\"available_digest\":\"$remote_digest\","
            updates_json+="\"image_id\":\"${image_id#sha256:}\""
            updates_json+="}"
            
            ((update_count++))
        fi
        
    done < <(docker images --format '{{.Repository}}|{{.Tag}}|{{.ID}}|{{.Digest}}' --no-trunc 2>/dev/null)
    
    updates_json+="]"
    
    info "Found $update_count image update(s) available"
    
    echo "$updates_json"
}

# Send Docker data to server
send_docker_data() {
    load_credentials
    
    info "Collecting Docker data..."
    
    local containers=$(collect_containers)
    local images=$(collect_images)
    local updates=$(check_image_updates)
    
    # Count collected items
    local container_count=$(echo "$containers" | jq '. | length' 2>/dev/null || echo "0")
    local image_count=$(echo "$images" | jq '. | length' 2>/dev/null || echo "0")
    local update_count=$(echo "$updates" | jq '. | length' 2>/dev/null || echo "0")
    
    info "Found $container_count containers, $image_count images, and $update_count update(s) available"
    
    # Build payload
    local payload="{\"apiId\":\"$API_ID\",\"apiKey\":\"$API_KEY\",\"containers\":$containers,\"images\":$images,\"updates\":$updates}"
    
    # Send to server
    info "Sending Docker data to PatchMon server..."
    
    local response=$(curl $CURL_FLAGS -s -w "\n%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$payload" \
        "${PATCHMON_SERVER}/api/${API_VERSION}/docker/collect" 2>&1)
    
    local http_code=$(echo "$response" | tail -n1)
    local response_body=$(echo "$response" | head -n-1)
    
    if [[ "$http_code" == "200" ]]; then
        success "Docker data sent successfully!"
        log "Docker data sent: $container_count containers, $image_count images"
        return 0
    else
        error "Failed to send Docker data. HTTP Status: $http_code\nResponse: $response_body"
    fi
}

# Test Docker data collection without sending
test_collection() {
    check_docker
    
    info "Testing Docker data collection (dry run)..."
    echo ""
    
    local containers=$(collect_containers)
    local images=$(collect_images)
    local updates=$(check_image_updates)
    
    local container_count=$(echo "$containers" | jq '. | length' 2>/dev/null || echo "0")
    local image_count=$(echo "$images" | jq '. | length' 2>/dev/null || echo "0")
    local update_count=$(echo "$updates" | jq '. | length' 2>/dev/null || echo "0")
    
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${GREEN}Docker Data Collection Results${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "Containers found: ${GREEN}$container_count${NC}"
    echo -e "Images found:     ${GREEN}$image_count${NC}"
    echo -e "Updates available: ${YELLOW}$update_count${NC}"
    echo ""
    
    if command -v jq &> /dev/null; then
        echo "━━━ Containers ━━━"
        echo "$containers" | jq -r '.[] | "\(.name) (\(.status)) - \(.image_name):\(.image_tag)"' | head -10
        if [[ $container_count -gt 10 ]]; then
            echo "... and $((container_count - 10)) more"
        fi
        echo ""
        echo "━━━ Images ━━━"
        echo "$images" | jq -r '.[] | "\(.repository):\(.tag) (\(.size_bytes / 1024 / 1024 | floor)MB)"' | head -10
        if [[ $image_count -gt 10 ]]; then
            echo "... and $((image_count - 10)) more"
        fi
        
        if [[ $update_count -gt 0 ]]; then
            echo ""
            echo "━━━ Available Updates ━━━"
            echo "$updates" | jq -r '.[] | "\(.repository):\(.current_tag) → \(.available_tag)"'
        fi
    fi
    
    echo ""
    success "Test collection completed successfully!"
}

# Show help
show_help() {
    cat << EOF
PatchMon Docker Agent v${AGENT_VERSION}

This agent collects Docker container and image information and sends it to PatchMon.

USAGE:
    $0 <command>

COMMANDS:
    collect         Collect and send Docker data to PatchMon server
    test            Test Docker data collection without sending (dry run)
    help            Show this help message

REQUIREMENTS:
    - Docker must be installed and running
    - Main PatchMon agent must be configured first
    - Credentials file must exist at $CREDENTIALS_FILE

EXAMPLES:
    # Test collection (dry run)
    sudo $0 test

    # Collect and send Docker data
    sudo $0 collect

SCHEDULING:
    To run this agent automatically, add a cron job:
    
    # Run every 5 minutes
    */5 * * * * /usr/local/bin/patchmon-docker-agent.sh collect

    # Run every hour
    0 * * * * /usr/local/bin/patchmon-docker-agent.sh collect

FILES:
    Config:      $CONFIG_FILE
    Credentials: $CREDENTIALS_FILE
    Log:         $LOG_FILE

EOF
}

# Main function
main() {
    case "$1" in
        "collect")
            check_docker
            check_yq_installed
            load_config
            send_docker_data
            ;;
        "test")
            check_docker
            check_yq_installed
            load_config
            test_collection
            ;;
        "help"|"--help"|"-h"|"")
            show_help
            ;;
        *)
            error "Unknown command: $1\n\nRun '$0 help' for usage information."
            ;;
    esac
}

# Run main function
main "$@"

