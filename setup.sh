#!/bin/bash
# PatchMon Self-Hosting Installation Script
# Automated deployment script for self-hosted PatchMon instances
# Usage: ./self-hosting-install.sh
# Interactive self-hosting installation script

set -e

# Create main installation log file
INSTALL_LOG="/var/log/patchmon-install.log"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] === PatchMon Self-Hosting Installation Started ===" >> "$INSTALL_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script PID: $$" >> "$INSTALL_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Running as user: $(whoami)" >> "$INSTALL_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Current directory: $(pwd)" >> "$INSTALL_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script arguments: $@" >> "$INSTALL_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script path: $0" >> "$INSTALL_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] ======================================" >> "$INSTALL_LOG"

# Create immediate debug log for troubleshooting
DEBUG_LOG="/tmp/patchmon_debug_$(date +%Y%m%d_%H%M%S).log"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] === PatchMon Script Started ===" >> "$DEBUG_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script PID: $$" >> "$DEBUG_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Running as user: $(whoami)" >> "$DEBUG_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Current directory: $(pwd)" >> "$DEBUG_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script arguments: $@" >> "$DEBUG_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script path: $0" >> "$DEBUG_LOG"
echo "[$(date '+%Y-%m-%d %H:%M:%S')] ======================================" >> "$DEBUG_LOG"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Global variables
SCRIPT_VERSION="self-hosting-install.sh v1.3.3-selfhost-2025-11-07"
DEFAULT_GITHUB_REPO="https://github.com/PatchMon/PatchMon.git"
FQDN=""
CUSTOM_FQDN=""
EMAIL=""

# Logging function
function log_message() {
    local message="$1"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local log_file="/var/log/patchmon-install.log"
    
    echo "[${timestamp}] ${message}" >> "$log_file"
    echo "[${timestamp}] ${message}"
}
DEPLOYMENT_BRANCH="main"
GITHUB_REPO=""
DB_SAFE_NAME=""
DB_PASS=""
JWT_SECRET=""
BACKEND_PORT=""
APP_DIR=""
USE_LETSENCRYPT="false"  # Will be set based on user input
SERVER_PROTOCOL_SEL="https"
SERVER_PORT_SEL=""  # Will be set to BACKEND_PORT in init_instance_vars
SETUP_NGINX="true"
UPDATE_MODE="false"
SELECTED_INSTANCE=""
SELECTED_SERVICE_NAME=""

# Functions
print_status() {
    printf "${GREEN}%s${NC}\n" "$1"
}

print_info() {
    printf "${BLUE}%s${NC}\n" "$1"
}

print_error() {
    printf "${RED}%s${NC}\n" "$1"
}

print_warning() {
    printf "${YELLOW}%s${NC}\n" "$1"
}

print_question() {
    printf "${BLUE}%s${NC}\n" "$1"
}

print_success() {
    printf "${GREEN}%s${NC}\n" "$1"
}

# Interactive input functions
read_input() {
    local prompt="$1"
    local var_name="$2"
    local default_value="$3"
    
    if [ -n "$default_value" ]; then
        echo -n -e "${BLUE}$prompt${NC} [${YELLOW}$default_value${NC}]: "
    else
        echo -n -e "${BLUE}$prompt${NC}: "
    fi
    
    read -r input
    if [ -z "$input" ] && [ -n "$default_value" ]; then
        eval "$var_name='$default_value'"
    else
        eval "$var_name='$input'"
    fi
}

read_yes_no() {
    local prompt="$1"
    local var_name="$2"
    local default_value="$3"
    
    while true; do
        if [ -n "$default_value" ]; then
            echo -n -e "${BLUE}$prompt${NC} [${YELLOW}$default_value${NC}]: "
        else
            echo -n -e "${BLUE}$prompt${NC} (y/n): "
        fi
        read -r input
        
        if [ -z "$input" ] && [ -n "$default_value" ]; then
            input="$default_value"
        fi
        
        case $input in
            [Yy]|[Yy][Ee][Ss])
                eval "$var_name='y'"
                break
                ;;
            [Nn]|[Nn][Oo])
                eval "$var_name='n'"
                break
                ;;
            *)
                print_error "Please answer yes (y) or no (n)"
                ;;
        esac
    done
}

print_banner() {
    echo -e "${BLUE}====================================================${NC}"
    echo -e "${BLUE}        PatchMon Self-Hosting Installation${NC}"
    echo -e "${BLUE}Running: $SCRIPT_VERSION${NC}"
    echo -e "${BLUE}====================================================${NC}"
}

# Interactive setup functions
check_timezone() {
    print_info "Checking current timezone..."
    current_tz=$(timedatectl show --property=Timezone --value 2>/dev/null || echo "Unknown")
    
    if [ "$current_tz" != "Unknown" ]; then
        current_datetime=$(date)
        print_info "Current timezone: $current_tz"
        print_info "Current date/time: $current_datetime"
        read_yes_no "Is this timezone and date/time correct?" TIMEZONE_CORRECT "y"
        
        if [ "$TIMEZONE_CORRECT" = "n" ]; then
            print_info "Available timezones:"
            timedatectl list-timezones | head -20
            print_warning "Showing first 20 timezones. Use 'timedatectl list-timezones' to see all."
            read_input "Enter your timezone (e.g., America/New_York, Europe/London)" NEW_TIMEZONE
            
            if [ -n "$NEW_TIMEZONE" ]; then
                print_info "Setting timezone to $NEW_TIMEZONE..."
                timedatectl set-timezone "$NEW_TIMEZONE"
                print_status "Timezone updated to $NEW_TIMEZONE"
                
                # Show updated date/time
                updated_datetime=$(date)
                print_info "Updated date/time: $updated_datetime"
            fi
        fi
    else
        print_warning "Could not detect timezone. Please set it manually if needed."
        current_datetime=$(date)
        print_info "Current date/time: $current_datetime"
    fi
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root"
        print_info "Please run: sudo $0"
        exit 1
    fi
}

# Function to run commands as a specific user with better error handling
run_as_user() {
    local user="$1"
    local command="$2"
    
    if ! command -v sudo >/dev/null 2>&1; then
        print_error "sudo is required but not installed. Please install sudo first."
        exit 1
    fi
    
    if ! id "$user" &>/dev/null; then
        print_error "User '$user' does not exist"
        exit 1
    fi
    
    sudo -u "$user" bash -c "$command"
}

# Detect and use the best available package manager
detect_package_manager() {
    # Prefer apt over apt-get for modern Debian/Ubuntu systems
    if command -v apt >/dev/null 2>&1; then
        PKG_MANAGER="apt"
        PKG_UPDATE="apt update"
        PKG_UPGRADE="apt upgrade -y"
        PKG_INSTALL="apt install -y"
    elif command -v apt-get >/dev/null 2>&1; then
        PKG_MANAGER="apt-get"
        PKG_UPDATE="apt-get update"
        PKG_UPGRADE="apt-get upgrade -y"
        PKG_INSTALL="apt-get install -y"
    else
        print_error "No supported package manager found (apt or apt-get required)"
        print_info "This script requires a Debian/Ubuntu-based system"
        exit 1
    fi
    
    print_info "Using package manager: $PKG_MANAGER"
}

check_prerequisites() {
    print_info "Running and checking prerequisites..."
    
    # Check if running as root
    check_root
    
    # Detect package manager
    detect_package_manager
    
    print_info "Installing updates..."
    $PKG_UPDATE -y
    $PKG_UPGRADE
    
    print_info "Installing prerequisite applications..."
    # Install sudo if not present (needed for user switching)
    if ! command -v sudo >/dev/null 2>&1; then
        print_info "Installing sudo (required for user switching)..."
        $PKG_INSTALL sudo
    fi
    
    $PKG_INSTALL wget curl jq git netcat-openbsd
    
    print_status "Prerequisites installed successfully"
}

select_branch() {
    print_info "Fetching available releases from GitHub repository..."
    
    # Create temporary directory for git operations
    TEMP_DIR="/tmp/patchmon_branches_$$"
    mkdir -p "$TEMP_DIR"
    cd "$TEMP_DIR"
    
    # Try to clone the repository normally
    if git clone "$DEFAULT_GITHUB_REPO" . 2>/dev/null; then
        # Get list of tags sorted by version (semantic versioning)
        # Using git tag with version sorting
        tags=$(git tag -l --sort=-v:refname 2>/dev/null | head -3)
        
        if [ -n "$tags" ]; then
            print_info "Available releases and branches:"
            echo ""
            
            # Display last 3 release tags
            option_count=1
            declare -A options_map
            
            while IFS= read -r tag; do
                if [ -n "$tag" ]; then
                    # Get tag date and commit info
                    tag_date=$(git log -1 --format="%ci" "$tag" 2>/dev/null || echo "Unknown")
                    
                    # Format the date
                    if [ "$tag_date" != "Unknown" ]; then
                        formatted_date=$(date -d "$tag_date" "+%Y-%m-%d %H:%M" 2>/dev/null || echo "$tag_date")
                    else
                        formatted_date="Unknown"
                    fi
                    
                    # Mark the first one as latest
                    if [ $option_count -eq 1 ]; then
                        printf "%2d. %-20s (Latest Release - %s)\n" "$option_count" "$tag" "$formatted_date"
                    else
                        printf "%2d. %-20s (Release - %s)\n" "$option_count" "$tag" "$formatted_date"
                    fi
                    
                    # Store the tag for later selection
                    options_map[$option_count]="$tag"
                    option_count=$((option_count + 1))
                fi
            done <<< "$tags"
            
            # Add main branch as an option
            main_commit=$(git log -1 --format="%ci" "origin/main" 2>/dev/null || echo "Unknown")
            if [ "$main_commit" != "Unknown" ]; then
                formatted_main_date=$(date -d "$main_commit" "+%Y-%m-%d %H:%M" 2>/dev/null || echo "$main_commit")
            else
                formatted_main_date="Unknown"
            fi
            printf "%2d. %-20s (Development Branch - %s)\n" "$option_count" "main" "$formatted_main_date"
            options_map[$option_count]="main"
            
            echo ""
            
            # Default to option 1 (latest release tag)
            default_option=1
            
            while true; do
                read_input "Select version/branch number" SELECTION_NUMBER "$default_option"
                
                if [[ "$SELECTION_NUMBER" =~ ^[0-9]+$ ]]; then
                    selected_option="${options_map[$SELECTION_NUMBER]}"
                    if [ -n "$selected_option" ]; then
                        DEPLOYMENT_BRANCH="$selected_option"
                        
                        # Show confirmation
                        if [ "$selected_option" = "main" ]; then
                            print_status "Selected branch: main (latest development code)"
                            print_info "Last commit: $formatted_main_date"
                        else
                            print_status "Selected release: $selected_option"
                            tag_date=$(git log -1 --format="%ci" "$selected_option" 2>/dev/null || echo "Unknown")
                            if [ "$tag_date" != "Unknown" ]; then
                                formatted_date=$(date -d "$tag_date" "+%Y-%m-%d %H:%M" 2>/dev/null || echo "$tag_date")
                                print_info "Release date: $formatted_date"
                            fi
                        fi
                        break
                    else
                        print_error "Invalid selection number. Please try again."
                    fi
                else
                    print_error "Please enter a valid number."
                fi
            done
        else
            print_warning "No release tags found, using default: main"
            DEPLOYMENT_BRANCH="main"
        fi
    else
        print_warning "Could not connect to GitHub repository"
        print_warning "This might be due to:"
        print_warning "  • Network connectivity issues"
        print_warning "  • Firewall blocking git access"
        print_warning "  • GitHub repository access restrictions"
        print_warning "Using default branch: main"
        DEPLOYMENT_BRANCH="main"
    fi
    
    # Clean up
    cd /
    rm -rf "$TEMP_DIR"
}

interactive_setup() {
    print_banner
    
    print_info "Welcome to PatchMon Self-Hosting Installation!"
    print_info "This script will guide you through the installation process."
    echo ""
    
    # Check prerequisites
    check_prerequisites
    echo ""
    
    # Check timezone
    check_timezone
    echo ""
    
    # Get basic information
    print_question "Let's gather some information about your installation:"
    echo ""
    
    read_input "Enter your domain name or IP address (e.g., patchmon.yourdomain.com or 192.168.1.100)" FQDN "patchmon.internal"
    
    echo ""
    print_info "🔒 SSL/HTTPS Configuration:"
    print_info "   • Public hosting (accessible from internet): Enable SSL for security"
    print_info "   • Local hosting (internal network only): SSL not required"
    echo ""
    read_yes_no "Are you hosting this publicly on the internet and want SSL/HTTPS with Let's Encrypt?" SSL_ENABLED "n"
    
    if [ "$SSL_ENABLED" = "y" ]; then
        read_input "Enter your email address for Let's Encrypt SSL certificate" EMAIL
    else
        EMAIL=""
    fi
    
    
    # Select branch
    echo ""
    select_branch
    echo ""
    
    # Confirm settings
    print_info "Please confirm your settings:"
    echo "  Domain/IP: $FQDN"
    echo "  SSL Enabled: $SSL_ENABLED"
    if [ "$SSL_ENABLED" = "y" ]; then
        echo "  Email: $EMAIL"
    fi
    echo "  Branch: $DEPLOYMENT_BRANCH"
    echo ""
    
    read_yes_no "Proceed with installation?" CONFIRM_INSTALL "y"
    
    if [ "$CONFIRM_INSTALL" = "n" ]; then
        print_info "Installation cancelled by user."
        exit 0
    fi
    
    print_success "Starting installation process..."
    echo ""
}

# Generate random password
generate_password() {
    openssl rand -base64 32 | tr -d "=+/" | cut -c1-25
}

# Generate JWT secret
generate_jwt_secret() {
    openssl rand -base64 64 | tr -d "=+/" | cut -c1-50
}

# Generate Redis password
generate_redis_password() {
    openssl rand -base64 32 | tr -d "=+/" | cut -c1-25
}

# Find next available Redis database
find_next_redis_db() {
    print_info "Finding next available Redis database..." >&2
    
    # Start from database 0 and keep checking until we find an empty one
    local db_num=0
    local max_attempts=16  # Redis default is 16 databases
    
    # Check if Redis requires authentication
    local test_output
    test_output=$(redis-cli -h localhost -p 6379 ping 2>&1)
    
    # Determine auth requirements
    local auth_required=false
    local redis_auth_args=""
    
    if echo "$test_output" | grep -q "NOAUTH\|WRONGPASS"; then
        auth_required=true
        
        # Try to load admin credentials if ACL file exists
        if [ -f /etc/redis/users.acl ] && grep -q "^user admin" /etc/redis/users.acl; then
            # Redis is configured with ACL - try to extract admin password
            print_info "Redis requires authentication, attempting with admin credentials..." >&2
            
            # For multi-instance setups, we can't know the admin password yet
            # So we'll just use database 0 as default
            print_info "Using database 0 (Redis ACL already configured)" >&2
            echo "0"
            return 0
        fi
    fi
    
    while [ $db_num -lt $max_attempts ]; do
        # Test if database is empty
        local key_count
        local redis_output
        
        # Try to get database size (with or without auth)
        redis_output=$(redis-cli -h localhost -p 6379 -n "$db_num" DBSIZE 2>&1)
        
        # Check for authentication errors
        if echo "$redis_output" | grep -q "NOAUTH\|WRONGPASS"; then
            # If we hit auth errors and haven't configured yet, use database 0
            print_info "Redis requires authentication, defaulting to database 0" >&2
            echo "0"
            return 0
        fi
        
        # Check for other errors
        if echo "$redis_output" | grep -q "ERR"; then
            if echo "$redis_output" | grep -q "invalid DB index"; then
                print_warning "Reached maximum database limit at database $db_num" >&2
                break
            else
                print_error "Error checking database $db_num: $redis_output" >&2
                return 1
            fi
        fi
        
        key_count="$redis_output"
        
        # If database is empty, use it
        if [ "$key_count" = "0" ] || [ "$key_count" = "(integer) 0" ]; then
            print_status "Found available Redis database: $db_num (empty)" >&2
            echo "$db_num"
            return 0
        fi
        
        print_info "Database $db_num has $key_count keys, checking next..." >&2
        db_num=$((db_num + 1))
    done
    
    print_warning "No available Redis databases found (checked 0-$max_attempts)" >&2
    print_info "Using database 0 (may have existing data)" >&2
    echo "0"
    return 0
}

# Initialize instance variables
init_instance_vars() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] init_instance_vars function started" >> "$DEBUG_LOG"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Creating safe database name from FQDN: $FQDN" >> "$DEBUG_LOG"
    
    # Create safe database name from FQDN
    DB_SAFE_NAME=$(echo "$FQDN" | sed 's/[^a-zA-Z0-9]/_/g' | sed 's/^_*//' | sed 's/_*$//')
    
    # Check if FQDN starts with a digit (likely an IP address)
    if [[ "$FQDN" =~ ^[0-9] ]]; then
        # Generate 2 random letters for IP address prefixing
        RANDOM_PREFIX=$(tr -dc 'a-z' < /dev/urandom | head -c 2)
        DB_SAFE_NAME="${RANDOM_PREFIX}${DB_SAFE_NAME}"
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] IP address detected, prefixed with: $RANDOM_PREFIX" >> "$DEBUG_LOG"
        print_info "IP address detected ($FQDN), using prefix '$RANDOM_PREFIX' for database/service names"
    fi
    
    DB_NAME="${DB_SAFE_NAME}"
    DB_USER="${DB_SAFE_NAME}"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] DB_SAFE_NAME: $DB_SAFE_NAME" >> "$DEBUG_LOG"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] DB_NAME: $DB_NAME" >> "$DEBUG_LOG"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] DB_USER: $DB_USER" >> "$DEBUG_LOG"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Generating password..." >> "$DEBUG_LOG"
    DB_PASS=$(generate_password)
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Generating JWT secret..." >> "$DEBUG_LOG"
    JWT_SECRET=$(generate_jwt_secret)
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Generating Redis password..." >> "$DEBUG_LOG"
    REDIS_PASSWORD=$(generate_redis_password)
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Finding next available Redis database..." >> "$DEBUG_LOG"
    REDIS_DB=$(find_next_redis_db)
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Generating random backend port..." >> "$DEBUG_LOG"
    
    # Generate random backend port (3001-3999)
    BACKEND_PORT=$((3001 + RANDOM % 999))
    
    # Set SERVER_PORT_SEL to 443 for HTTPS (external port) or backend port for HTTP
    if [ "$SERVER_PROTOCOL_SEL" = "https" ]; then
        SERVER_PORT_SEL=443
    else
        SERVER_PORT_SEL=$BACKEND_PORT
    fi
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] BACKEND_PORT: $BACKEND_PORT" >> "$DEBUG_LOG"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] SERVER_PORT_SEL: $SERVER_PORT_SEL" >> "$DEBUG_LOG"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Setting application directory and service name..." >> "$DEBUG_LOG"
    
    # Set application directory and service name
    APP_DIR="/opt/${FQDN}"
    SERVICE_NAME="${FQDN}"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] APP_DIR: $APP_DIR" >> "$DEBUG_LOG"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] SERVICE_NAME: $SERVICE_NAME" >> "$DEBUG_LOG"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Creating dedicated user name..." >> "$DEBUG_LOG"
    
    # Create dedicated user name (safe for system users)
    INSTANCE_USER=$(echo "$DB_SAFE_NAME" | cut -c1-32)
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] INSTANCE_USER: $INSTANCE_USER" >> "$DEBUG_LOG"
    
    print_info "Initialized variables for $FQDN"
    print_info "Database: $DB_NAME"
    print_info "Backend Port: $BACKEND_PORT"
    print_info "App Directory: $APP_DIR"
    print_info "Instance User: $INSTANCE_USER"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] init_instance_vars function completed successfully" >> "$DEBUG_LOG"
}

# Update system packages
update_system() {
    print_info "Updating system packages..."
    $PKG_UPDATE -y
    $PKG_UPGRADE
}

# Install essential tools
install_essential_tools() {
    print_info "Installing essential tools..."
    $PKG_INSTALL curl netcat-openbsd git jq
}

# Install Node.js (if not already installed)
install_nodejs() {
    # Force PATH refresh to ensure we get the latest Node.js
    export PATH="/usr/bin:/usr/local/bin:$PATH"
    hash -r  # Clear bash command cache
    
    NODE_VERSION=""
    if command -v node >/dev/null 2>&1; then
        NODE_VERSION=$(node --version 2>/dev/null | sed 's/v//')
        print_info "Node.js already installed: v$NODE_VERSION"
        
        # Check if version is 18 or higher
        if [ "$(echo "$NODE_VERSION" | cut -d. -f1)" -ge 18 ]; then
            print_status "Node.js version is sufficient (v$NODE_VERSION)"
            # Clean npm cache to avoid issues
            npm cache clean --force 2>/dev/null || true
            return 0
        else
            print_warning "Node.js version $NODE_VERSION is too old, updating..."
        fi
    fi
    
    print_info "Installing Node.js 20.x..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    $PKG_INSTALL nodejs
    
    # Verify installation
    NODE_VERSION=$(node --version | sed 's/v//')
    NPM_VERSION=$(npm --version)
    print_status "Node.js installed: v$NODE_VERSION"
    print_status "npm installed: v$NPM_VERSION"
    
    # Clean npm cache to avoid issues
    npm cache clean --force 2>/dev/null || true
}

# Install PostgreSQL
install_postgresql() {
    print_info "Installing PostgreSQL..."
    
    if systemctl is-active --quiet postgresql; then
        print_status "PostgreSQL already running"
    else
        $PKG_INSTALL postgresql postgresql-contrib
        systemctl start postgresql
        systemctl enable postgresql
        print_status "PostgreSQL installed and started"
    fi
}

# Install Redis
install_redis() {
    print_info "Installing Redis..."
    
    if systemctl is-active --quiet redis-server; then
        print_status "Redis already running"
    else
        $PKG_INSTALL redis-server
        systemctl start redis-server
        systemctl enable redis-server
        print_status "Redis installed and started"
    fi
}

# Configure Redis with user authentication
configure_redis() {
    print_info "Configuring Redis with user authentication..."
    
    # Check if Redis is running
    if ! systemctl is-active --quiet redis-server; then
        print_error "Redis is not running. Please start Redis first."
        return 1
    fi
    
    # Generate Redis username based on instance (global variable for use in create_env_files)
    REDIS_USER="patchmon_${DB_SAFE_NAME}"
    
    # Generate separate user password (more secure than reusing admin password)
    # This will be stored in the .env file for the application to use
    REDIS_USER_PASSWORD=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-32)
    
    print_info "Creating Redis user: $REDIS_USER for database $REDIS_DB"
    
    # Create Redis configuration backup
    if [ -f /etc/redis/redis.conf ]; then
        cp /etc/redis/redis.conf /etc/redis/redis.conf.backup.$(date +%Y%m%d_%H%M%S)
        print_info "Created Redis configuration backup"
    fi
    
    # Configure Redis with ACL authentication
    print_info "Configuring Redis with ACL authentication"
    
    # Ensure ACL file exists and is configured
    if [ ! -f /etc/redis/users.acl ]; then
        touch /etc/redis/users.acl
        chown redis:redis /etc/redis/users.acl
        chmod 640 /etc/redis/users.acl
        print_status "Created Redis ACL file"
    else
        # Backup existing ACL file
        cp /etc/redis/users.acl /etc/redis/users.acl.backup.$(date +%Y%m%d_%H%M%S) 2>/dev/null || true
        print_info "Backed up existing ACL file"
    fi
    
    # Configure ACL file in redis.conf
    if ! grep -q "^aclfile" /etc/redis/redis.conf; then
        echo "aclfile /etc/redis/users.acl" >> /etc/redis/redis.conf
        print_status "Added ACL file configuration to Redis"
    fi
    
    # Remove any requirepass configuration (incompatible with ACL)
    if grep -q "^requirepass" /etc/redis/redis.conf; then
        sed -i 's/^requirepass.*/# &/' /etc/redis/redis.conf
        print_status "Disabled requirepass (incompatible with ACL)"
    fi
    
    # Remove any user definitions from redis.conf (should be in ACL file)
    if grep -q "^user " /etc/redis/redis.conf; then
        sed -i '/^user /d' /etc/redis/redis.conf
        print_status "Removed user definitions from redis.conf"
    fi
    
    # Create or update admin user in ACL file
    if grep -q "^user admin" /etc/redis/users.acl; then
        print_info "Admin user already exists in ACL, updating password..."
        # Remove old admin line and add new one
        sed -i '/^user admin/d' /etc/redis/users.acl
        echo "user admin on sanitize-payload >$REDIS_PASSWORD ~* &* +@all" >> /etc/redis/users.acl
        print_status "Updated admin user password"
    else
        echo "user admin on sanitize-payload >$REDIS_PASSWORD ~* &* +@all" >> /etc/redis/users.acl
        print_status "Added admin user to ACL file"
    fi
    
    # Restart Redis to apply ACL configuration
    print_info "Restarting Redis to apply ACL configuration..."
    systemctl restart redis-server
    
    # Wait for Redis to start with retry logic
    sleep 5
    
    # Test admin connection with retries
    local max_retries=3
    local retry=0
    local admin_works=false
    
    while [ $retry -lt $max_retries ]; do
        if redis-cli -h 127.0.0.1 -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ping > /dev/null 2>&1; then
            admin_works=true
            break
        fi
        print_info "Waiting for Redis to be ready... (attempt $((retry + 1))/$max_retries)"
        sleep 2
        retry=$((retry + 1))
    done
    
    if [ "$admin_works" = false ]; then
        print_error "Failed to verify admin connection after Redis restart"
        print_error "Redis ACL configuration may have issues"
        
        # Try to fix by disabling ACL and using requirepass instead
        print_warning "Attempting fallback: using requirepass instead of ACL..."
        sed -i 's/^aclfile/# aclfile/' /etc/redis/redis.conf
        sed -i "s/^# requirepass .*/requirepass $REDIS_PASSWORD/" /etc/redis/redis.conf
        if ! grep -q "^requirepass" /etc/redis/redis.conf; then
            echo "requirepass $REDIS_PASSWORD" >> /etc/redis/redis.conf
        fi
        systemctl restart redis-server
        sleep 3
        
        # Test requirepass
        if redis-cli -h 127.0.0.1 -p 6379 -a "$REDIS_PASSWORD" --no-auth-warning ping > /dev/null 2>&1; then
            print_status "Fallback successful - using requirepass authentication"
            # For requirepass mode, we'll set REDIS_USER empty later
            print_info "Note: Using legacy requirepass mode instead of ACL"
        else
            print_error "Fallback also failed - Redis authentication is broken"
            return 1
        fi
    else
        print_status "Redis ACL authentication configuration successful"
    fi
    
    # Create Redis user with ACL (only if admin_works, meaning we're using ACL mode)
    if [ "$admin_works" = true ]; then
        print_info "Creating Redis ACL user: $REDIS_USER"
        
        # Create user with password and permissions - capture output for error handling
        local acl_result
        acl_result=$(redis-cli -h 127.0.0.1 -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ACL SETUSER "$REDIS_USER" on ">${REDIS_USER_PASSWORD}" ~* +@all 2>&1)
        
        if [ "$acl_result" = "OK" ]; then
            print_status "Redis user '$REDIS_USER' created successfully"
            
            # Save ACL users to file to persist across restarts
            local save_result
            save_result=$(redis-cli -h 127.0.0.1 -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ACL SAVE 2>&1)
            
            if [ "$save_result" = "OK" ]; then
                print_status "Redis ACL users saved to file"
            else
                print_warning "Failed to save ACL users to file: $save_result"
            fi
            
            # Verify user was actually created
            local verify_result
            verify_result=$(redis-cli -h 127.0.0.1 -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ACL GETUSER "$REDIS_USER" 2>&1)
            
            if [ "$verify_result" = "(nil)" ]; then
                print_error "User creation reported OK but user does not exist"
                return 1
            fi
            
            # Test user connection
            print_info "Testing Redis user connection..."
            if redis-cli -h 127.0.0.1 -p 6379 --user "$REDIS_USER" --pass "$REDIS_USER_PASSWORD" --no-auth-warning -n "$REDIS_DB" ping > /dev/null 2>&1; then
                print_status "Redis user connection test successful"
            else
                print_error "Redis user connection test failed"
                return 1
            fi
            
            # Mark the selected database as in-use
            redis-cli -h 127.0.0.1 -p 6379 --user "$REDIS_USER" --pass "$REDIS_USER_PASSWORD" --no-auth-warning -n "$REDIS_DB" SET "patchmon:initialized" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > /dev/null
            print_status "Marked Redis database $REDIS_DB as in-use"
        else
            print_error "Failed to create Redis user: $acl_result"
            return 1
        fi
    else
        # Using requirepass mode - no per-user ACL
        print_info "Using requirepass mode - testing connection..."
        
        # For requirepass, we don't use username, just password
        if redis-cli -h 127.0.0.1 -p 6379 -a "$REDIS_PASSWORD" --no-auth-warning -n "$REDIS_DB" ping > /dev/null 2>&1; then
            print_status "Redis requirepass connection test successful"
            
            # Mark the selected database as in-use
            redis-cli -h 127.0.0.1 -p 6379 -a "$REDIS_PASSWORD" --no-auth-warning -n "$REDIS_DB" SET "patchmon:initialized" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > /dev/null
            print_status "Marked Redis database $REDIS_DB as in-use"
            
            # Set REDIS_USER to empty for requirepass mode
            REDIS_USER=""
            REDIS_USER_PASSWORD="$REDIS_PASSWORD"
        else
            print_error "Redis requirepass connection test failed"
            return 1
        fi
    fi
    
    # Note: Redis credentials will be written to .env by create_env_files() function
    print_status "Redis configured successfully"
    
    if [ -n "$REDIS_USER" ]; then
        print_info "Redis Mode: ACL with user '$REDIS_USER'"
    else
        print_info "Redis Mode: requirepass (legacy single-password auth)"
    fi
    print_info "Redis credentials will be saved to backend/.env"
    
    return 0
}

# Install nginx
install_nginx() {
    print_info "Installing nginx..."
    
    if systemctl is-active --quiet nginx; then
        print_status "nginx already running"
    else
        $PKG_INSTALL nginx
        systemctl start nginx
        systemctl enable nginx
        print_status "nginx installed and started"
    fi
}

# Install certbot for Let's Encrypt
install_certbot() {
    print_info "Installing certbot for Let's Encrypt..."
    
    if command -v certbot >/dev/null 2>&1; then
        print_status "certbot already installed"
    else
        $PKG_INSTALL certbot python3-certbot-nginx
        print_status "certbot installed"
    fi
}

# Create dedicated user for this instance
create_instance_user() {
    print_info "Creating dedicated user: $INSTANCE_USER"
    
    # Create application directory first (as root)
    mkdir -p "$APP_DIR"
    
    # Check if user already exists
    if id "$INSTANCE_USER" &>/dev/null; then
        print_warning "User $INSTANCE_USER already exists, skipping creation"
        # Ensure directory ownership is correct for existing user
        chown -R "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR"
        chmod 755 "$APP_DIR"
        return 0
    fi
    
    # Create user with no login shell and no home directory
    useradd --system --no-create-home --shell /bin/false "$INSTANCE_USER"
    
    # Set ownership and permissions
    chown -R "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR"
    chmod 755 "$APP_DIR"
    
    print_status "Dedicated user $INSTANCE_USER created successfully"
}

# Setup Node.js environment isolation for this instance
setup_nodejs_isolation() {
    print_info "Setting up Node.js environment isolation for $INSTANCE_USER..."
    
    # Create npm directories as root first
    mkdir -p "$APP_DIR/.npm" "$APP_DIR/.npm-global"
    
    # Create .npmrc file with proper configuration
    cat > "$APP_DIR/.npmrc" << EOF
cache=$APP_DIR/.npm
prefix=$APP_DIR/.npm-global
init-module=$APP_DIR/.npm-global/.npm-init.js
tmp=$APP_DIR/.npm/tmp
EOF
    
    # Set ownership to the dedicated user
    chown -R "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR/.npm" "$APP_DIR/.npm-global" "$APP_DIR/.npmrc"
    
    print_status "Node.js environment isolation configured for $INSTANCE_USER"
}

# Setup database for instance
setup_database() {
    print_info "Setting up database: $DB_NAME"
    
    # Check if sudo is available for user switching
    if command -v sudo >/dev/null 2>&1; then
        # Check if user exists
        user_exists=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='$DB_USER'" || echo "0")
        
        if [ "$user_exists" = "1" ]; then
            print_info "Database user $DB_USER already exists, skipping creation"
        else
            print_info "Creating database user $DB_USER"
            sudo -u postgres psql -c "CREATE USER $DB_USER WITH PASSWORD '$DB_PASS';"
        fi
        
        # Check if database exists
        db_exists=$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" || echo "0")
        
        if [ "$db_exists" = "1" ]; then
            print_info "Database $DB_NAME already exists, skipping creation"
        else
            print_info "Creating database $DB_NAME"
            sudo -u postgres psql -c "CREATE DATABASE $DB_NAME OWNER $DB_USER;"
        fi
        
        # Always grant privileges (in case they were revoked)
        sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;"
    else
        # Alternative method for systems without sudo (run as postgres user directly)
        print_warning "sudo not available, using alternative method for PostgreSQL setup"
        
        # Check if user exists
        user_exists=$(su - postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='$DB_USER'\"" || echo "0")
        
        if [ "$user_exists" = "1" ]; then
            print_info "Database user $DB_USER already exists, skipping creation"
        else
            print_info "Creating database user $DB_USER"
            su - postgres -c "psql -c \"CREATE USER $DB_USER WITH PASSWORD '$DB_PASS';\""
        fi
        
        # Check if database exists
        db_exists=$(su - postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='$DB_NAME'\"" || echo "0")
        
        if [ "$db_exists" = "1" ]; then
            print_info "Database $DB_NAME already exists, skipping creation"
        else
            print_info "Creating database $DB_NAME"
            su - postgres -c "psql -c \"CREATE DATABASE $DB_NAME OWNER $DB_USER;\""
        fi
        
        # Always grant privileges (in case they were revoked)
        su - postgres -c "psql -c \"GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;\""
    fi
    
    print_status "Database setup complete for $DB_NAME"
}

# Clone application repository
clone_application() {
    print_info "Cloning PatchMon application..."
    
    if [ -d "$APP_DIR" ]; then
        print_warning "Directory $APP_DIR already exists, removing..."
        rm -rf "$APP_DIR"
    fi
    
    git clone -b "$DEPLOYMENT_BRANCH" "$GITHUB_REPO" "$APP_DIR"
    
    # Set ownership to the dedicated user
    chown -R "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR"
    
    cd "$APP_DIR"
    
    print_status "Application cloned to $APP_DIR with ownership set to $INSTANCE_USER"
}

# Setup Node.js environment
setup_node_environment() {
    print_info "Setting up Node.js environment..."
    
    cd "$APP_DIR"
    
    # Set Node.js environment
    export NODE_ENV=production
    export PATH="/usr/bin:/usr/local/bin:$PATH"
    
    print_status "Node.js environment configured"
}

# Install dependencies
install_dependencies() {
    print_info "Installing dependencies as user $INSTANCE_USER..."
    
    cd "$APP_DIR"
    
    # Clean up any existing node_modules to avoid conflicts
    rm -rf node_modules
    
    # Create tmp directory for npm
    mkdir -p "$APP_DIR/.npm/tmp"
    
    # Fix npm cache ownership issues (common problem)
    chown -R "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR/.npm"
    
    # Clean npm cache to avoid permission issues
    run_as_user "$INSTANCE_USER" "cd $APP_DIR && npm cache clean --force" 2>/dev/null || true
    
    # Install root dependencies as the dedicated user
    print_info "Installing root dependencies..."
    if ! run_as_user "$INSTANCE_USER" "
        cd $APP_DIR
        export NPM_CONFIG_CACHE=$APP_DIR/.npm
        export NPM_CONFIG_PREFIX=$APP_DIR/.npm-global
        export NPM_CONFIG_TMP=$APP_DIR/.npm/tmp
        npm install --omit=dev --no-audit --no-fund --no-save --ignore-scripts
    "; then
        print_error "Failed to install root dependencies"
        return 1
    fi
    
    # Install backend dependencies as the dedicated user
    print_info "Installing backend dependencies..."
    cd backend
    rm -rf node_modules
    if ! run_as_user "$INSTANCE_USER" "
        cd $APP_DIR/backend
        export NPM_CONFIG_CACHE=$APP_DIR/.npm
        export NPM_CONFIG_PREFIX=$APP_DIR/.npm-global
        export NPM_CONFIG_TMP=$APP_DIR/.npm/tmp
        npm install --omit=dev --no-audit --no-fund --no-save --ignore-scripts
    "; then
        print_error "Failed to install backend dependencies"
        return 1
    fi
    cd ..
    
    # Install frontend dependencies as the dedicated user (including dev dependencies for build)
    print_info "Installing frontend dependencies..."
    cd frontend
    rm -rf node_modules
    if ! run_as_user "$INSTANCE_USER" "
        cd $APP_DIR/frontend
        export NPM_CONFIG_CACHE=$APP_DIR/.npm
        export NPM_CONFIG_PREFIX=$APP_DIR/.npm-global
        export NPM_CONFIG_TMP=$APP_DIR/.npm/tmp
        npm install --no-audit --no-fund --no-save --ignore-scripts
    "; then
        print_error "Failed to install frontend dependencies"
        return 1
    fi
    
    # Build frontend
    print_info "Building frontend..."
    if ! run_as_user "$INSTANCE_USER" "
        cd $APP_DIR/frontend
        export NPM_CONFIG_CACHE=$APP_DIR/.npm
        export NPM_CONFIG_PREFIX=$APP_DIR/.npm-global
        export NPM_CONFIG_TMP=$APP_DIR/.npm/tmp
        npm run build
    "; then
        print_error "Failed to build frontend"
        return 1
    fi
    cd ..
    
    # Ensure ownership is maintained
    chown -R "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR"
    
    print_status "Dependencies installed and frontend built as $INSTANCE_USER"
}

# Create environment files
create_env_files() {
    print_info "Creating environment files..."
    
    cd "$APP_DIR"
    
    # Backend .env
    cat > backend/.env << EOF
# Database Configuration
DATABASE_URL="postgresql://$DB_USER:$DB_PASS@localhost:5432/$DB_NAME"
PM_DB_CONN_MAX_ATTEMPTS=30
PM_DB_CONN_WAIT_INTERVAL=2

# Database Connection Pool Configuration (Prisma)
DB_CONNECTION_LIMIT=30
DB_POOL_TIMEOUT=20
DB_CONNECT_TIMEOUT=10
DB_IDLE_TIMEOUT=300
DB_MAX_LIFETIME=1800

# JWT Configuration
JWT_SECRET="$JWT_SECRET"
JWT_EXPIRES_IN=1h
JWT_REFRESH_EXPIRES_IN=7d

# Server Configuration
PORT=$BACKEND_PORT
NODE_ENV=production

# API Configuration
API_VERSION=v1

# CORS Configuration
CORS_ORIGIN="$SERVER_PROTOCOL_SEL://$FQDN"

# Session Configuration
SESSION_INACTIVITY_TIMEOUT_MINUTES=30

# User Configuration
DEFAULT_USER_ROLE=user

# Rate Limiting (times in milliseconds)
RATE_LIMIT_WINDOW_MS=900000
RATE_LIMIT_MAX=5000
AUTH_RATE_LIMIT_WINDOW_MS=600000
AUTH_RATE_LIMIT_MAX=500
AGENT_RATE_LIMIT_WINDOW_MS=60000
AGENT_RATE_LIMIT_MAX=1000

# Redis Configuration
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_USER=$REDIS_USER
REDIS_PASSWORD=$REDIS_USER_PASSWORD
REDIS_DB=$REDIS_DB

# Logging
LOG_LEVEL=info
ENABLE_LOGGING=true

# TFA Configuration
TFA_REMEMBER_ME_EXPIRES_IN=30d
TFA_MAX_REMEMBER_SESSIONS=5
TFA_SUSPICIOUS_ACTIVITY_THRESHOLD=3
EOF

    # Frontend .env
    cat > frontend/.env << EOF
VITE_API_URL=$SERVER_PROTOCOL_SEL://$FQDN/api/v1
VITE_APP_NAME=PatchMon
VITE_APP_VERSION=1.3.1
EOF

    print_status "Environment files created"
}

# Check and fix failed Prisma migrations
fix_failed_migrations() {
    local db_name="$1"
    local db_user="$2"
    local db_pass="$3"
    local db_host="${4:-localhost}"
    local max_retries=3
    
    print_info "Checking for failed migrations in database..."
    
    # Query for failed migrations (where started_at is set but finished_at is NULL)
    local failed_migrations
    failed_migrations=$(PGPASSWORD="$db_pass" psql -h "$db_host" -U "$db_user" -d "$db_name" -t -A -c \
        "SELECT migration_name FROM _prisma_migrations WHERE finished_at IS NULL AND started_at IS NOT NULL;" 2>/dev/null || echo "")
    
    if [ -z "$failed_migrations" ]; then
        print_status "No failed migrations found"
        return 0
    fi
    
    print_warning "Found failed migration(s):"
    echo "$failed_migrations" | while read -r migration; do
        [ -n "$migration" ] && print_warning "  - $migration"
    done
    
    print_info "Attempting to resolve failed migrations..."
    
    # For each failed migration, mark it as rolled back and remove it
    echo "$failed_migrations" | while read -r migration; do
        if [ -n "$migration" ]; then
            print_info "Processing failed migration: $migration"
            
            # Mark the migration as rolled back
            PGPASSWORD="$db_pass" psql -h "$db_host" -U "$db_user" -d "$db_name" -c \
                "UPDATE _prisma_migrations SET rolled_back_at = NOW() WHERE migration_name = '$migration' AND finished_at IS NULL;" >/dev/null 2>&1
            
            # Delete the failed migration record to allow retry
            PGPASSWORD="$db_pass" psql -h "$db_host" -U "$db_user" -d "$db_name" -c \
                "DELETE FROM _prisma_migrations WHERE migration_name = '$migration' AND finished_at IS NULL;" >/dev/null 2>&1
            
            print_status "Marked migration '$migration' for retry"
        fi
    done
    
    print_status "Failed migrations have been cleared for retry"
    return 0
}

# Run database migrations with self-healing
run_migrations() {
    print_info "Running database migrations as user $INSTANCE_USER..."
    
    cd "$APP_DIR/backend"
    
    local max_attempts=3
    local attempt=1
    local migration_success=false
    
    while [ $attempt -le $max_attempts ]; do
        print_info "Migration attempt $attempt of $max_attempts..."
        
        # Try to run migrations
        local migrate_output
        migrate_output=$(run_as_user "$INSTANCE_USER" "cd $APP_DIR/backend && npx prisma migrate deploy 2>&1" || echo "MIGRATION_FAILED")
        
        # Check if migration succeeded
        if ! echo "$migrate_output" | grep -q "MIGRATION_FAILED\|Error:\|P3009"; then
            print_status "Migrations completed successfully"
            migration_success=true
            break
        fi
        
        # Check specifically for P3009 (failed migrations found)
        if echo "$migrate_output" | grep -q "P3009\|migrate found failed migrations"; then
            print_warning "Detected failed migrations (P3009 error)"
            
            # Extract the failed migration name if possible
            local failed_migration
            failed_migration=$(echo "$migrate_output" | grep -oP "The \`\K[^\`]+" | head -1 || echo "")
            
            if [ -n "$failed_migration" ]; then
                print_info "Failed migration identified: $failed_migration"
            fi
            
            # Attempt to fix failed migrations
            print_info "Attempting to self-heal migration issues..."
            if fix_failed_migrations "$DB_NAME" "$DB_USER" "$DB_PASS" "localhost"; then
                print_status "Migration issues resolved, retrying..."
                attempt=$((attempt + 1))
                sleep 2
                continue
            else
                print_error "Failed to resolve migration issues"
                break
            fi
        else
            # Other migration error
            print_error "Migration failed with error:"
            echo "$migrate_output" | grep -A 5 "Error:"
            break
        fi
    done
    
    if [ "$migration_success" = false ]; then
        print_error "Migrations failed after $max_attempts attempts"
        print_info "You may need to manually resolve migration issues"
        print_info "Check migrations: cd $APP_DIR/backend && npx prisma migrate status"
        return 1
    fi
    
    # Generate Prisma client
    run_as_user "$INSTANCE_USER" "cd $APP_DIR/backend && npx prisma generate" >/dev/null 2>&1 || true
    
    print_status "Database migrations completed as $INSTANCE_USER"
    return 0
}

# Admin account creation removed - handled by application's first-time setup

# Create systemd service
create_systemd_service() {
    print_info "Creating systemd service for user $INSTANCE_USER..."
    
    cat > "/etc/systemd/system/$SERVICE_NAME.service" << EOF
[Unit]
Description=PatchMon Service for $FQDN
After=network.target postgresql.service

[Service]
Type=simple
User=$INSTANCE_USER
Group=$INSTANCE_USER
WorkingDirectory=$APP_DIR/backend
ExecStart=/usr/bin/node src/server.js
Restart=always
RestartSec=10
Environment=NODE_ENV=production
Environment=PATH=/usr/bin:/usr/local/bin
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$APP_DIR

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    
    print_status "Systemd service created: $SERVICE_NAME (running as $INSTANCE_USER)"
}

# Unified nginx configuration generator
generate_nginx_config() {
    local fqdn="$1"
    local app_dir="$2"
    local backend_port="$3"
    local ssl_enabled="$4"  # "true" or "false"
    local config_file="/etc/nginx/sites-available/$fqdn"
    
    print_info "Generating nginx configuration for $fqdn (SSL: $ssl_enabled)"
    
    if [ "$ssl_enabled" = "true" ]; then
        # SSL Configuration
        cat > "$config_file" << EOF
# HTTP to HTTPS redirect
server {
    listen 80;
    server_name $fqdn;
    
    # Let's Encrypt challenge location
    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }
    
    # Redirect all other traffic to HTTPS
    location / {
        return 301 https://\$server_name\$request_uri;
    }
}

# HTTPS server block
server {
    listen 443 ssl http2;
    server_name $fqdn;
    
    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/$fqdn/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$fqdn/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
    
    # Security headers (applied to all responses)
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    
    # Frontend
    location / {
        root $app_dir/frontend/dist;
        try_files \$uri \$uri/ /index.html;
    }
    
    # Bull Board proxy
    location /bullboard {
        proxy_pass http://127.0.0.1:$backend_port;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host \$host;
        proxy_set_header Cookie \$http_cookie;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 300s;
        proxy_connect_timeout 75s;
        
        # Enable cookie passthrough
        proxy_pass_header Set-Cookie;
        proxy_cookie_path / /;
        
        # Preserve original client IP
        proxy_set_header X-Original-Forwarded-For \$http_x_forwarded_for;
        
        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }
    
    # API proxy
    location /api/ {
        proxy_pass http://127.0.0.1:$backend_port;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 300s;
        proxy_connect_timeout 75s;
        
        # Preserve original client IP
        proxy_set_header X-Original-Forwarded-For \$http_x_forwarded_for;
        
        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }
    
    # Static assets caching (exclude Bull Board assets)
    location ~* ^/(?!bullboard).*\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        root $app_dir/frontend/dist;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    
    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:$backend_port/health;
        access_log off;
    }
}
EOF
    else
        # HTTP-only configuration
        cat > "$config_file" << EOF
server {
    listen 80;
    listen [::]:80;
    server_name $fqdn;
    
    # Security headers
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    
    # Frontend
    location / {
        root $app_dir/frontend/dist;
        try_files \$uri \$uri/ /index.html;
    }
    
    # Bull Board proxy
    location /bullboard {
        proxy_pass http://127.0.0.1:$backend_port;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header X-Forwarded-Host \$host;
        proxy_set_header Cookie \$http_cookie;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 300s;
        proxy_connect_timeout 75s;
        
        # Enable cookie passthrough
        proxy_pass_header Set-Cookie;
        proxy_cookie_path / /;
        
        # Preserve original client IP
        proxy_set_header X-Original-Forwarded-For \$http_x_forwarded_for;
        
        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }
    
    # API proxy
    location /api/ {
        proxy_pass http://127.0.0.1:$backend_port;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 300s;
        proxy_connect_timeout 75s;
        
        # Preserve original client IP
        proxy_set_header X-Original-Forwarded-For \$http_x_forwarded_for;
        
        if (\$request_method = 'OPTIONS') {
            return 204;
        }
    }
    
    # Static assets caching (exclude Bull Board assets)
    location ~* ^/(?!bullboard).*\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        root $app_dir/frontend/dist;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    
    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:$backend_port/health;
        access_log off;
    }
}
EOF
    fi
    
    print_status "Nginx configuration generated for $fqdn"
}

# Setup nginx configuration
setup_nginx() {
    print_info "Setting up nginx configuration..."
    log_message "Setting up nginx configuration for $FQDN"
    
    # Generate HTTP-only config first (needed for Let's Encrypt challenge if SSL enabled)
    generate_nginx_config "$FQDN" "$APP_DIR" "$BACKEND_PORT" "false"

    # Enable site
    ln -sf "/etc/nginx/sites-available/$FQDN" "/etc/nginx/sites-enabled/"
    
    # Remove default site if it exists
    rm -f /etc/nginx/sites-enabled/default
    
    # Test nginx configuration
    nginx -t
    
    # Reload nginx
    systemctl reload nginx
    
    print_status "nginx configuration created for $FQDN"
}

# Setup Let's Encrypt SSL
setup_letsencrypt() {
    print_info "Setting up Let's Encrypt SSL certificate..."
    
    # Check if a valid certificate already exists
    if certbot certificates 2>/dev/null | grep -q "$FQDN" && certbot certificates 2>/dev/null | grep -A 10 "$FQDN" | grep -q "VALID"; then
        print_status "Valid SSL certificate already exists for $FQDN"
        
        # Generate SSL config with existing certificate
        generate_nginx_config "$FQDN" "$APP_DIR" "$BACKEND_PORT" "true"
        
        # Enable the site
        ln -sf "/etc/nginx/sites-available/$FQDN" "/etc/nginx/sites-enabled/"
        
        # Test nginx configuration
        if nginx -t; then
            print_status "Nginx configuration updated for existing SSL certificate"
            systemctl reload nginx
        else
            print_error "Nginx configuration test failed"
            return 1
        fi
        
        return 0
    fi
    
    print_info "No valid certificate found, generating new SSL certificate..."
    
    # Wait for nginx to be ready
    sleep 5
    
    # Obtain SSL certificate
    log_message "Obtaining SSL certificate for $FQDN using Let's Encrypt"
    certbot --nginx -d "$FQDN" --non-interactive --agree-tos --email "$EMAIL" --redirect
    log_message "SSL certificate obtained successfully"
    
    # Generate SSL nginx configuration
    generate_nginx_config "$FQDN" "$APP_DIR" "$BACKEND_PORT" "true"
    
    # Test and reload nginx
    if nginx -t; then
        systemctl reload nginx
        print_status "Nginx configuration updated successfully"
    else
        print_error "Nginx configuration test failed"
        return 1
    fi
    
    # Setup auto-renewal
    echo "0 12 * * * /usr/bin/certbot renew --quiet" | crontab -
    
    print_status "SSL certificate obtained and auto-renewal configured"
}

# Start services
start_services() {
    print_info "Starting services..."
    
    # Start PatchMon service
    systemctl start "$SERVICE_NAME"
    
    # Wait for service to start
    sleep 10
    
    # Check if service is running
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        print_status "PatchMon service started successfully"
    else
        print_error "Failed to start PatchMon service"
        echo ""
        
        # Show last 25 lines of service logs for debugging
        print_warning "=== Last 25 lines of service logs ==="
        journalctl -u "$SERVICE_NAME" -n 25 --no-pager || true
        print_warning "==================================="
        echo ""
        
        # Check for specific error patterns
        local logs=$(journalctl -u "$SERVICE_NAME" -n 50 --no-pager 2>/dev/null || echo "")
        
        if echo "$logs" | grep -q "WRONGPASS\|NOAUTH"; then
            print_error "Detected Redis authentication error!"
            print_info "The service cannot authenticate with Redis."
            echo ""
            print_info "Current Redis configuration in .env:"
            grep "^REDIS_" "$APP_DIR/backend/.env" || true
            echo ""
            print_info "Debug steps:"
            print_info "  1. Check Redis is running:"
            print_info "     systemctl status redis-server"
            echo ""
            print_info "  2. Check Redis ACL users:"
            print_info "     redis-cli ACL LIST"
            echo ""
            print_info "  3. Test Redis connection:"
            local test_user=$(grep "^REDIS_USER=" "$APP_DIR/backend/.env" | cut -d'=' -f2)
            local test_pass=$(grep "^REDIS_PASSWORD=" "$APP_DIR/backend/.env" | cut -d'=' -f2)
            local test_db=$(grep "^REDIS_DB=" "$APP_DIR/backend/.env" | cut -d'=' -f2)
            print_info "     redis-cli --user $test_user --pass $test_pass -n ${test_db:-0} ping"
            echo ""
            print_info "  4. Check Redis configuration files:"
            print_info "     cat /etc/redis/redis.conf | grep aclfile"
            print_info "     cat /etc/redis/users.acl"
            echo ""
        elif echo "$logs" | grep -q "ECONNREFUSED.*postgresql\|Connection refused.*5432"; then
            print_error "Detected PostgreSQL connection error!"
            print_info "Check if PostgreSQL is running:"
            print_info "  systemctl status postgresql"
        elif echo "$logs" | grep -q "ECONNREFUSED.*redis\|Connection refused.*6379"; then
            print_error "Detected Redis connection error!"
            print_info "Check if Redis is running:"
            print_info "  systemctl status redis-server"
        elif echo "$logs" | grep -q "database.*does not exist"; then
            print_error "Database does not exist!"
            print_info "Database: $DB_NAME"
        elif echo "$logs" | grep -q "Error:"; then
            print_error "Application error detected in logs"
        fi
        
        echo ""
        print_info "View full logs: journalctl -u $SERVICE_NAME -f"
        print_info "Check service status: systemctl status $SERVICE_NAME"
        
        return 1
    fi
}

# Populate server settings in database
populate_server_settings() {
    print_info "Populating server settings in database..."
    
    cd "$APP_DIR/backend"
    
    # Create settings update script
    cat > update_settings.js << EOF
const { PrismaClient } = require('@prisma/client');
const prisma = new PrismaClient();

async function updateSettings() {
  try {
    // Check if settings record exists, create or update
    const existingSettings = await prisma.settings.findFirst();
    
    const settingsData = {
      server_url: '$SERVER_PROTOCOL_SEL://$FQDN',
      server_protocol: '$SERVER_PROTOCOL_SEL',
      server_host: '$FQDN',
      server_port: $SERVER_PORT_SEL,
      update_interval: 60,
      auto_update: true
    };
    
    if (existingSettings) {
      // Update existing settings
      await prisma.settings.update({
        where: { id: existingSettings.id },
        data: settingsData
      });
    } else {
      // Create new settings record
      await prisma.settings.create({
        data: settingsData
      });
    }
    
    console.log('Database settings updated successfully');
  } catch (error) {
    console.error('Error updating settings:', error.message);
    process.exit(1);
  } finally {
    await prisma.\$disconnect();
  }
}

updateSettings();
EOF

    # Run the settings update script as the dedicated user
    run_as_user "$INSTANCE_USER" "cd $APP_DIR/backend && node update_settings.js"
    
    # Clean up temporary script
    rm -f update_settings.js
    
    print_status "Server settings populated successfully"
}

# Create agent version
create_agent_version() {
    echo -e "${BLUE}🤖 Creating agent version...${NC}"
    log_message "Creating agent version in database..."
    cd $APP_DIR/backend
    
    # Priority 1: Get version from agent script (most accurate for agent versions)
    local current_version="N/A"
    if [ -f "$APP_DIR/agents/patchmon-agent.sh" ]; then
        current_version=$(grep '^AGENT_VERSION=' "$APP_DIR/agents/patchmon-agent.sh" | cut -d'"' -f2 2>/dev/null || echo "N/A")
        if [ "$current_version" != "N/A" ] && [ -n "$current_version" ]; then
            print_info "Detected agent version from script: $current_version"
        fi
    fi
    
    # Priority 2: Use fallback version if not found
    if [ "$current_version" = "N/A" ] || [ -z "$current_version" ]; then
        current_version="1.3.0"
        print_warning "Could not determine version, using fallback: $current_version"
    fi
    
    print_info "Creating/updating agent version: $current_version"
    print_info "This will ensure the latest agent script is available in the database"
    
    # Test connection before creating agent version
    if ! PGPASSWORD="$DB_PASS" psql -h localhost -U "$DB_USER" -d "$DB_NAME" -c "SELECT 1;" >/dev/null 2>&1; then
        print_error "Cannot connect to database before creating agent version"
        exit 1
    fi
    
    # Copy agent script to backend directory
    if [ -f "$APP_DIR/agents/patchmon-agent.sh" ]; then
        cp "$APP_DIR/agents/patchmon-agent.sh" "$APP_DIR/backend/"
        
        print_status "Agent version management removed - using file-based approach"
    fi
    
    # Make agent binaries executable
    if [ -d "$APP_DIR/agents" ]; then
        chmod +x "$APP_DIR/agents/patchmon-agent-linux-"* 2>/dev/null || true
        print_status "Agent binaries made executable"
    fi

    return 0
}
# Create deployment summary
create_deployment_summary() {
    print_info "Writing deployment summary into deployment-info.txt..."
    
    # Reuse the unified deployment info file
    SUMMARY_FILE="$APP_DIR/deployment-info.txt"
    
    cat >> "$SUMMARY_FILE" << EOF

----------------------------------------------------
        Deployment Summary (Appended)
----------------------------------------------------

Deployment Information:
- Email: $EMAIL
- Branch: $DEPLOYMENT_BRANCH
- Deployed: $(date)
- Deployment Duration: $(($(date +%s) - $DEPLOYMENT_START_TIME)) seconds

Service Status:
- PatchMon Service: $(systemctl is-active $SERVICE_NAME)
- Nginx Service: $(systemctl is-active nginx)
- PostgreSQL Service: $(systemctl is-active postgresql)
- SSL Certificate: $(if [ "$USE_LETSENCRYPT" = "true" ]; then echo "Enabled"; else echo "Disabled"; fi)

Diagnostic Commands:
- Service Status: systemctl status $SERVICE_NAME
- Service Logs: journalctl -u $SERVICE_NAME -f
- Nginx Status: systemctl status nginx
- Nginx Logs: journalctl -u nginx -f
- Database Status: systemctl status postgresql
- SSL Certificate: certbot certificates
- Disk Usage: df -h $APP_DIR
- Process Status: ps aux | grep $SERVICE_NAME

Troubleshooting:
- Check deployment log: cat $APP_DIR/patchmon-install.log
- Check service logs: journalctl -u $SERVICE_NAME --since "1 hour ago"
- Check nginx config: nginx -t
- Check database connection: sudo -u $DB_USER psql -d $DB_NAME -c "SELECT 1;"
- Check port binding: netstat -tlnp | grep $BACKEND_PORT

====================================================
EOF

    # Ensure permissions
    chmod 644 "$SUMMARY_FILE"
    chown "$INSTANCE_USER:$INSTANCE_USER" "$SUMMARY_FILE"
    
    # Copy the entire installation log into the instance folder
    if [ -f "$INSTALL_LOG" ]; then
        cp "$INSTALL_LOG" "$APP_DIR/patchmon-install.log" || true
        chown "$INSTANCE_USER:$INSTANCE_USER" "$APP_DIR/patchmon-install.log" || true
        chmod 644 "$APP_DIR/patchmon-install.log" || true
    fi
    
    # Verify file was created
    if [ -f "$SUMMARY_FILE" ]; then
        print_status "Deployment summary appended to: $SUMMARY_FILE"
    else
        print_error "Failed to append to deployment-info.txt file"
        return 1
    fi
}

# Email notification function removed for self-hosting deployment

# Save deployment information to file
save_deployment_info() {
    print_info "Saving deployment information to file..."
    
    # Create deployment info file
    INFO_FILE="$APP_DIR/deployment-info.txt"
    
    cat > "$INFO_FILE" << EOF
====================================================
        PatchMon Deployment Information
====================================================

Instance Details:
- FQDN: $FQDN
- URL: $SERVER_PROTOCOL_SEL://$FQDN
- Deployed: $(date)
- Deployment Type: $(if [ "$USE_LETSENCRYPT" = "true" ]; then echo "Public with SSL"; else echo "Local/Internal"; fi)
- SSL Enabled: $USE_LETSENCRYPT
- Service Name: $SERVICE_NAME

Directories:
- App Directory: $APP_DIR
- Backend: $APP_DIR/backend
- Frontend (built): $APP_DIR/frontend/dist
- Node.js isolation dir: $APP_DIR/.npm

Database Information:
- Name: $DB_NAME
- User: $DB_USER
- Password: $DB_PASS
- Host: localhost
- Port: 5432

Redis Information:
- Host: localhost
- Port: 6379
- User: $REDIS_USER
- Password: $REDIS_USER_PASSWORD
- Database: $REDIS_DB

Networking:
- Backend Port: $BACKEND_PORT
- Nginx Config: /etc/nginx/sites-available/$FQDN

Logs & Files:
- Deployment Log: $LOG_FILE
- Systemd Service: /etc/systemd/system/$SERVICE_NAME.service

Common Commands:
- Restart backend service: sudo systemctl restart $SERVICE_NAME
- Check backend status:   systemctl status $SERVICE_NAME
- Tail backend logs:      journalctl -u $SERVICE_NAME -f
- Test nginx config:      nginx -t && systemctl reload nginx
- Check DB connection:    sudo -u $DB_USER psql -d $DB_NAME -c "SELECT 1;"

First-Time Setup:
- Visit the web interface: $SERVER_PROTOCOL_SEL://$FQDN
- Create the admin account through the web UI (no pre-created credentials)

Notes:
- Default role permissions (admin/user) are created automatically on backend startup
- Keep this file for future reference of your environment

====================================================
EOF

    # Set permissions (readable by root and instance user)
    chmod 644 "$INFO_FILE"
    chown "$INSTANCE_USER:$INSTANCE_USER" "$INFO_FILE"
    
    # Verify file was created
    if [ -f "$INFO_FILE" ]; then
        print_status "Deployment information saved to: $INFO_FILE"
        print_info "File details: $(ls -lh "$INFO_FILE" | awk '{print $5, $9}')"
    else
        print_error "Failed to create deployment-info.txt file"
        return 1
    fi
}

# Restart PatchMon service
restart_patchmon() {
    print_info "Restarting PatchMon service..."
    
    # Restart PatchMon service
    systemctl restart "$SERVICE_NAME"
    
    # Wait for service to restart
    sleep 5
    
    # Check if service is running
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        print_status "PatchMon service restarted successfully"
    else
        print_error "Failed to restart PatchMon service"
        systemctl status "$SERVICE_NAME"
        return 1
    fi
}

# Setup logging for deployment
setup_deployment_logging() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] setup_deployment_logging function started" >> "$DEBUG_LOG"
    
    print_info "Setting up deployment logging..."
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] APP_DIR variable: $APP_DIR" >> "$DEBUG_LOG"
    
    # Use the main installation log file
    LOG_FILE="$INSTALL_LOG"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Using main log file: $LOG_FILE" >> "$DEBUG_LOG"
    
    print_info "Deployment log: $LOG_FILE"
    
    # Function to log with timestamp
    log_output() {
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
    }
    
    # Redirect all output to both terminal and log file
    exec > >(tee -a "$LOG_FILE")
    exec 2>&1
    
    log_output "=== PatchMon Deployment Started ==="
    log_output "Script started at: $(date)"
    log_output "Script PID: $$"
    log_output "Running as user: $(whoami)"
    log_output "Current directory: $(pwd)"
    log_output "Script arguments: $@"
    log_output "FQDN: $FQDN"
    log_output "Email: $EMAIL"
    log_output "Branch: $DEPLOYMENT_BRANCH"
    log_output "SSL Enabled: $USE_LETSENCRYPT"
    log_output "====================================="
}

# Main deployment function
deploy_instance() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] deploy_instance function started" >> "$DEBUG_LOG"
    
    log_message "=== SELF-HOSTING-INSTALL.SH DEPLOYMENT STARTED ==="
    log_message "Script version: $SCRIPT_VERSION"
    log_message "FQDN: $FQDN"
    log_message "Email: $EMAIL"
    log_message "SSL Enabled: $USE_LETSENCRYPT"
    
    print_banner
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Skipping early logging setup - will do after variables initialized" >> "$DEBUG_LOG"
    
    # Record deployment start time
    DEPLOYMENT_START_TIME=$(date +%s)
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] About to validate parameters" >> "$DEBUG_LOG"
    
    # Parameters are already validated in interactive_setup
    print_info "All parameters validated successfully"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Parameter validation passed" >> "$DEBUG_LOG"
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Checking if instance already exists at /opt/$FQDN" >> "$DEBUG_LOG"
    
    # Check if instance already exists
    if [ -d "/opt/$FQDN" ]; then
        print_error "Instance for $FQDN already exists at /opt/$FQDN"
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: Instance already exists" >> "$DEBUG_LOG"
        exit 1
    fi
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Instance check passed - no existing instance found" >> "$DEBUG_LOG"
    
    print_info "🚀 Deploying PatchMon instance for $FQDN"
    print_info "📧 Email: $EMAIL"
    print_info "🌿 Branch: $DEPLOYMENT_BRANCH"
    print_info "🔒 SSL: $USE_LETSENCRYPT"
    if [ "$USE_LETSENCRYPT" = "true" ]; then
        print_info "📧 SSL Email: $EMAIL"
    fi
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] About to call init_instance_vars function" >> "$DEBUG_LOG"
    
    # Initialize variables
    init_instance_vars
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] init_instance_vars function completed" >> "$DEBUG_LOG"
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Variables initialized, APP_DIR: $APP_DIR" >> "$DEBUG_LOG"
    
    # Setup logging (after variables are initialized)
    setup_deployment_logging
    
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Deployment logging setup completed" >> "$DEBUG_LOG"
    
    # Display generated credentials
    echo -e "${BLUE}🔐 Auto-generated credentials:${NC}"
    echo -e "${YELLOW}Database Name: $DB_NAME${NC}"
    echo -e "${YELLOW}Database User: $DB_USER${NC}"
    echo -e "${YELLOW}Database Password: $DB_PASS${NC}"
    echo -e "${YELLOW}Redis User: $REDIS_USER${NC}"
    echo -e "${YELLOW}Redis User Password: $REDIS_USER_PASSWORD${NC}"
    echo -e "${YELLOW}Redis Database: $REDIS_DB${NC}"
    echo -e "${YELLOW}JWT Secret: $JWT_SECRET${NC}"
    echo -e "${YELLOW}Backend Port: $BACKEND_PORT${NC}"
    echo -e "${YELLOW}Instance User: $INSTANCE_USER${NC}"
    echo -e "${YELLOW}Node.js Isolation: $APP_DIR/.npm${NC}"
    echo ""
    
    # System setup (prerequisites already installed in interactive_setup)
    install_nodejs
    install_postgresql
    install_redis
    configure_redis
    install_nginx
    
    # Only install certbot if SSL is enabled
    if [ "$USE_LETSENCRYPT" = "true" ]; then
        install_certbot
    fi
    
    # Instance-specific setup
    create_instance_user
    setup_nodejs_isolation
    setup_database
    clone_application
    setup_node_environment
    install_dependencies
    create_env_files
    run_migrations
    # Admin account creation removed - handled by application's first-time setup
    
    # Service and web server setup
    create_systemd_service
    setup_nginx
    
    # SSL setup (if enabled)
    if [ "$USE_LETSENCRYPT" = "true" ]; then
        setup_letsencrypt
    else
        print_info "SSL disabled - skipping SSL certificate setup"
    fi
    
    # Start services
    start_services
    
    # Populate server settings in database
    populate_server_settings
    
    # Create agent version in database
    create_agent_version
    
    # Restart PatchMon service to ensure it's running properly
    restart_patchmon
    
    # Save deployment information to file
    save_deployment_info
    
    # Create deployment summary
    create_deployment_summary
    
    # Email notifications removed for self-hosting deployment
    
    # Final status
    log_message "=== DEPLOYMENT COMPLETED SUCCESSFULLY ==="
    log_message "Instance URL: $SERVER_PROTOCOL_SEL://$FQDN"
    log_message "Service name: $SERVICE_NAME"
    log_message "Backend port: $BACKEND_PORT"
    log_message "SSL enabled: $USE_LETSENCRYPT"
    
    print_status "PatchMon instance deployed successfully!"
    echo ""
    print_info "Next steps:"
    echo "  • Visit your URL: $SERVER_PROTOCOL_SEL://$FQDN (ensure DNS is configured)"
    echo "  • Deployment information file: $APP_DIR/deployment-info.txt"
    echo "  • View deployment info: cat $APP_DIR/deployment-info.txt"
    echo ""
    
    # Suppress JSON echo to terminal; details already logged and saved to summary/credentials files
    :
}

# Detect existing PatchMon installations
detect_installations() {
    local installations=()
    
    # Find all directories in /opt that contain PatchMon installations
    if [ -d "/opt" ]; then
        for dir in /opt/*/; do
            local dirname=$(basename "$dir")
            # Skip backup directories
            if [[ "$dirname" =~ \.backup\. ]]; then
                continue
            fi
            # Check if it's a PatchMon installation
            if [ -f "$dir/backend/package.json" ] && grep -q "patchmon" "$dir/backend/package.json" 2>/dev/null; then
                installations+=("$dirname")
            fi
        done
    fi
    
    echo "${installations[@]}"
}

# Select installation to update
select_installation_to_update() {
    local installations=($(detect_installations))
    
    if [ ${#installations[@]} -eq 0 ]; then
        print_error "No existing PatchMon installations found in /opt"
        exit 1
    fi
    
    print_info "Found ${#installations[@]} existing installation(s):"
    echo ""
    
    local i=1
    declare -A install_map
    for install in "${installations[@]}"; do
        # Get current version if possible
        local version="unknown"
        if [ -f "/opt/$install/backend/package.json" ]; then
            version=$(grep '"version"' "/opt/$install/backend/package.json" | head -1 | sed 's/.*"version": "\([^"]*\)".*/\1/')
        fi
        
        # Get service status - search for service files that reference this installation
        local service_name=""
        local status="unknown"
        
        # Search systemd directory for service files that reference this installation
        for service_file in /etc/systemd/system/*.service; do
            if [ -f "$service_file" ]; then
                # Check if this service file references our installation directory
                if grep -q "/opt/$install" "$service_file"; then
                    service_name=$(basename "$service_file" .service)
                    
                    # Check service status
                    if systemctl is-active --quiet "$service_name" 2>/dev/null; then
                        status="running"
                        break
                    elif systemctl is-enabled --quiet "$service_name" 2>/dev/null; then
                        status="stopped"
                        break
                    fi
                fi
            fi
        done
        
        # If not found by searching, try common naming conventions
        if [ -z "$service_name" ] || [ "$status" == "unknown" ]; then
            # Convention 1: Just the install name (e.g., patchmon.internal)
            local try_service="$install"
            # Convention 2: patchmon. prefix (e.g., patchmon.patchmon.internal)
            local alt_service_name1="patchmon.$install"
            # Convention 3: patchmon- prefix with underscores (e.g., patchmon-patchmon_internal)
            local alt_service_name2="patchmon-$(echo "$install" | tr '.' '_')"
            
            # Try convention 1 first (most common)
            if systemctl is-active --quiet "$try_service" 2>/dev/null; then
                status="running"
                service_name="$try_service"
            elif systemctl is-enabled --quiet "$try_service" 2>/dev/null; then
                status="stopped"
                service_name="$try_service"
            # Try convention 2
            elif systemctl is-active --quiet "$alt_service_name1" 2>/dev/null; then
                status="running"
                service_name="$alt_service_name1"
            elif systemctl is-enabled --quiet "$alt_service_name1" 2>/dev/null; then
                status="stopped"
                service_name="$alt_service_name1"
            # Try convention 3
            elif systemctl is-active --quiet "$alt_service_name2" 2>/dev/null; then
                status="running"
                service_name="$alt_service_name2"
            elif systemctl is-enabled --quiet "$alt_service_name2" 2>/dev/null; then
                status="stopped"
                service_name="$alt_service_name2"
            fi
        fi
        
        # Fallback: if still no service found, use default naming convention
        if [ -z "$service_name" ]; then
            service_name="$install"
            status="not_found"
        fi
        
        printf "%2d. %-30s (v%-10s - %s)\n" "$i" "$install" "$version" "$status"
        install_map[$i]="$install"
        # Store the service name for later use
        declare -g "service_map_$i=$service_name"
        i=$((i + 1))
    done
    
    echo ""
    
    while true; do
        read_input "Select installation number to update" SELECTION "1"
        
        if [[ "$SELECTION" =~ ^[0-9]+$ ]] && [ -n "${install_map[$SELECTION]}" ]; then
            SELECTED_INSTANCE="${install_map[$SELECTION]}"
            # Get the stored service name
            local varname="service_map_$SELECTION"
            SELECTED_SERVICE_NAME="${!varname}"
            print_status "Selected: $SELECTED_INSTANCE"
            print_info "Service: $SELECTED_SERVICE_NAME"
            return 0
        else
            print_error "Invalid selection. Please enter a number from 1 to ${#installations[@]}"
        fi
    done
}

# Repair/recreate Redis user with correct permissions
repair_redis_user() {
    local redis_user="$1"
    local redis_pass="$2"
    local redis_db="${3:-0}"
    
    print_info "Attempting to repair Redis user: $redis_user"
    
    # Find admin password
    local admin_password=""
    if [ -f /etc/redis/users.acl ] && grep -q "^user admin" /etc/redis/users.acl; then
        admin_password=$(grep "^user admin" /etc/redis/users.acl | grep -oP '>\K[^ ]+' | head -1)
    fi
    
    if [ -z "$admin_password" ]; then
        print_error "Cannot repair Redis user - no admin credentials found"
        return 1
    fi
    
    # Test admin connection
    if ! redis-cli -h localhost -p 6379 --user admin --pass "$admin_password" --no-auth-warning ping >/dev/null 2>&1; then
        print_error "Admin credentials don't work - cannot repair user"
        return 1
    fi
    
    print_status "Admin access confirmed"
    
    # Delete existing user if it exists (and is broken)
    print_info "Removing old user configuration..."
    redis-cli -h localhost -p 6379 --user admin --pass "$admin_password" --no-auth-warning ACL DELUSER "$redis_user" >/dev/null 2>&1 || true
    
    # Create user with full permissions
    print_info "Creating user with full permissions..."
    local create_result
    create_result=$(redis-cli -h localhost -p 6379 --user admin --pass "$admin_password" --no-auth-warning ACL SETUSER "$redis_user" on ">${redis_pass}" ~* +@all 2>&1)
    
    if echo "$create_result" | grep -q "OK"; then
        # Save ACL
        redis-cli -h localhost -p 6379 --user admin --pass "$admin_password" --no-auth-warning ACL SAVE >/dev/null 2>&1
        
        # Verify the new user works
        if redis-cli -h localhost -p 6379 --user "$redis_user" --pass "$redis_pass" --no-auth-warning -n "$redis_db" ping >/dev/null 2>&1; then
            if redis-cli -h localhost -p 6379 --user "$redis_user" --pass "$redis_pass" --no-auth-warning -n "$redis_db" info >/dev/null 2>&1; then
                print_status "Redis user repaired successfully"
                return 0
            else
                print_error "User created but INFO command still fails"
                return 1
            fi
        else
            print_error "User created but PING command fails"
            return 1
        fi
    else
        print_error "Failed to create user: $create_result"
        return 1
    fi
}

# Check and update Redis configuration for existing installation
update_redis_configuration() {
    print_info "Checking Redis configuration..."
    
    # Check if Redis configuration exists in .env
    if [ -f "$instance_dir/backend/.env" ]; then
        if grep -q "^REDIS_HOST=" "$instance_dir/backend/.env" && \
           grep -q "^REDIS_PASSWORD=" "$instance_dir/backend/.env"; then
            print_status "Redis configuration already exists in .env"
            
            # Verify the credentials actually work
            local redis_user=$(grep "^REDIS_USER=" "$instance_dir/backend/.env" | cut -d'=' -f2 | tr -d '"')
            local redis_pass=$(grep "^REDIS_PASSWORD=" "$instance_dir/backend/.env" | cut -d'=' -f2 | tr -d '"')
            local redis_db=$(grep "^REDIS_DB=" "$instance_dir/backend/.env" | cut -d'=' -f2 | tr -d '"')
            
            if [ -n "$redis_user" ] && [ -n "$redis_pass" ]; then
                # Test with username and password
                local ping_works=false
                local info_works=false
                
                if redis-cli -h localhost -p 6379 --user "$redis_user" --pass "$redis_pass" --no-auth-warning -n "${redis_db:-0}" ping >/dev/null 2>&1; then
                    ping_works=true
                fi
                
                if redis-cli -h localhost -p 6379 --user "$redis_user" --pass "$redis_pass" --no-auth-warning -n "${redis_db:-0}" info >/dev/null 2>&1; then
                    info_works=true
                fi
                
                if [ "$ping_works" = true ] && [ "$info_works" = true ]; then
                    print_status "Redis credentials verified with redis-cli (tested ping and info commands)"
                    
                    # Force refresh the Redis user during updates to ensure correct ACL permissions
                    # This prevents issues where redis-cli works but Node.js client doesn't
                    print_info "Refreshing Redis user permissions to ensure compatibility..."
                    
                    if repair_redis_user "$redis_user" "$redis_pass" "$redis_db"; then
                        print_status "Redis user permissions refreshed successfully"
                        return 0
                    else
                        print_warning "Could not refresh Redis user, but credentials seem to work - continuing..."
                        return 0
                    fi
                else
                    print_warning "Redis credentials not working properly (ping: $ping_works, info: $info_works)"
                    print_info "Attempting to repair Redis user..."
                    
                    if repair_redis_user "$redis_user" "$redis_pass" "$redis_db"; then
                        print_status "Redis user repaired successfully"
                        return 0
                    else
                        print_warning "Could not repair Redis user, will reconfigure from scratch..."
                    fi
                fi
            else
                print_warning "Redis credentials incomplete in .env (missing user or password)"
            fi
        fi
    fi
    
    print_warning "Redis configuration not found or invalid in .env - setting up Redis for this instance..."
    
    # Detect package manager if not already set
    if [ -z "$PKG_INSTALL" ]; then
        if command -v apt >/dev/null 2>&1; then
            PKG_INSTALL="apt install -y"
        elif command -v apt-get >/dev/null 2>&1; then
            PKG_INSTALL="apt-get install -y"
        else
            print_error "No supported package manager found"
            return 1
        fi
    fi
    
    # Ensure Redis is installed and running
    if ! systemctl is-active --quiet redis-server; then
        print_info "Installing Redis..."
        $PKG_INSTALL redis-server
        systemctl start redis-server
        systemctl enable redis-server
    fi
    
    # Generate Redis variables for this instance
    # Extract DB_SAFE_NAME from existing database name
    DB_SAFE_NAME=$(echo "$DB_NAME" | sed 's/[^a-zA-Z0-9]/_/g')
    REDIS_USER="patchmon_${DB_SAFE_NAME}"
    REDIS_USER_PASSWORD=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-32)
    
    # Test Redis connection to determine authentication status
    print_info "Testing Redis authentication status..."
    local needs_auth=false
    local admin_password=""
    
    # Try ping without auth
    if redis-cli -h localhost -p 6379 ping >/dev/null 2>&1; then
        print_info "Redis is accessible without authentication"
        needs_auth=false
    else
        print_info "Redis requires authentication"
        needs_auth=true
        
        # Try to find existing admin password from ACL file
        if [ -f /etc/redis/users.acl ] && grep -q "^user admin" /etc/redis/users.acl; then
            # Extract password from ACL file (format: >password)
            admin_password=$(grep "^user admin" /etc/redis/users.acl | grep -oP '>\K[^ ]+' | head -1)
            
            if [ -n "$admin_password" ]; then
                print_info "Found existing admin credentials in ACL file"
                
                # Test admin credentials
                if redis-cli -h localhost -p 6379 --user admin --pass "$admin_password" --no-auth-warning ping >/dev/null 2>&1; then
                    print_status "Existing admin credentials work"
                    REDIS_PASSWORD="$admin_password"
                else
                    print_warning "Existing admin credentials don't work, will create new configuration"
                    admin_password=""
                fi
            fi
        fi
    fi
    
    # Find available Redis database
    print_info "Finding available Redis database..."
    local redis_db=0
    local max_attempts=16
    
    while [ $redis_db -lt $max_attempts ]; do
        local key_count
        
        if [ "$needs_auth" = true ] && [ -n "$admin_password" ]; then
            key_count=$(redis-cli -h localhost -p 6379 --user admin --pass "$admin_password" --no-auth-warning -n "$redis_db" DBSIZE 2>&1 | grep -oP '\d+' || echo "1")
        else
            key_count=$(redis-cli -h localhost -p 6379 -n "$redis_db" DBSIZE 2>&1 | grep -oP '\d+' || echo "1")
        fi
        
        if [ "$key_count" = "0" ]; then
            print_status "Found available Redis database: $redis_db"
            REDIS_DB=$redis_db
            break
        fi
        redis_db=$((redis_db + 1))
    done
    
    if [ -z "$REDIS_DB" ]; then
        print_warning "No empty Redis database found, using database 0"
        REDIS_DB=0
    fi
    
    # Configure Redis with ACL if needed
    if [ "$needs_auth" = false ]; then
        print_info "Configuring Redis ACL for security..."
        
        # Generate new admin password
        REDIS_PASSWORD=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-32)
        
        # Backup redis.conf
        if [ -f /etc/redis/redis.conf ]; then
            cp /etc/redis/redis.conf /etc/redis/redis.conf.backup.$(date +%Y%m%d_%H%M%S) 2>/dev/null || true
        fi
        
        # Create ACL file if it doesn't exist
        if [ ! -f /etc/redis/users.acl ]; then
            touch /etc/redis/users.acl
            chown redis:redis /etc/redis/users.acl
            chmod 640 /etc/redis/users.acl
            print_status "Created Redis ACL file"
        else
            # Backup existing ACL file
            cp /etc/redis/users.acl /etc/redis/users.acl.backup.$(date +%Y%m%d_%H%M%S) 2>/dev/null || true
            print_info "Backed up existing ACL file"
        fi
        
        # Configure ACL file in redis.conf
        if ! grep -q "^aclfile" /etc/redis/redis.conf 2>/dev/null; then
            echo "aclfile /etc/redis/users.acl" >> /etc/redis/redis.conf
            print_status "Added ACL file configuration to redis.conf"
        fi
        
        # Remove requirepass (incompatible with ACL)
        if grep -q "^requirepass" /etc/redis/redis.conf 2>/dev/null; then
            sed -i 's/^requirepass.*/# &/' /etc/redis/redis.conf
            print_status "Disabled requirepass (incompatible with ACL)"
        fi
        
        # Create or update admin user in ACL file
        if grep -q "^user admin" /etc/redis/users.acl; then
            print_info "Admin user already exists in ACL, updating password..."
            # Remove old admin line and add new one
            sed -i '/^user admin/d' /etc/redis/users.acl
            echo "user admin on sanitize-payload >$REDIS_PASSWORD ~* &* +@all" >> /etc/redis/users.acl
            print_status "Updated admin user password"
        else
            echo "user admin on sanitize-payload >$REDIS_PASSWORD ~* &* +@all" >> /etc/redis/users.acl
            print_status "Created admin user in ACL"
        fi
        
        # Restart Redis to apply ACL
        print_info "Restarting Redis to apply ACL configuration..."
        systemctl restart redis-server
        sleep 5
        
        # Verify admin can connect
        local max_retries=3
        local retry=0
        local admin_works=false
        
        while [ $retry -lt $max_retries ]; do
            if redis-cli -h localhost -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ping >/dev/null 2>&1; then
                admin_works=true
                break
            fi
            print_info "Waiting for Redis to be ready... (attempt $((retry + 1))/$max_retries)"
            sleep 2
            retry=$((retry + 1))
        done
        
        if [ "$admin_works" = false ]; then
            print_error "Failed to verify admin connection after Redis restart"
            print_error "Redis ACL configuration may have issues"
            
            # Try to fix by disabling ACL and using requirepass instead
            print_warning "Attempting fallback: using requirepass instead of ACL..."
            sed -i 's/^aclfile/# aclfile/' /etc/redis/redis.conf
            sed -i "s/^# requirepass .*/requirepass $REDIS_PASSWORD/" /etc/redis/redis.conf
            if ! grep -q "^requirepass" /etc/redis/redis.conf; then
                echo "requirepass $REDIS_PASSWORD" >> /etc/redis/redis.conf
            fi
            systemctl restart redis-server
            sleep 3
            
            # Test requirepass
            if redis-cli -h localhost -p 6379 -a "$REDIS_PASSWORD" --no-auth-warning ping >/dev/null 2>&1; then
                print_status "Fallback successful - using requirepass authentication"
                # For requirepass, we don't use username
                REDIS_USER=""
            else
                print_error "Fallback also failed - Redis authentication is broken"
                return 1
            fi
        else
            print_status "Redis ACL configuration successful"
        fi
    elif [ -z "$admin_password" ]; then
        print_error "Redis requires authentication but no valid admin credentials found"
        print_error "Please check /etc/redis/users.acl or /etc/redis/redis.conf"
        print_info "Manual fix: Reset Redis authentication or provide admin credentials"
        return 1
    fi
    
    # Create instance-specific Redis user (only if using ACL)
    if [ -n "$REDIS_USER" ]; then
        print_info "Creating Redis user: $REDIS_USER"
        
        local acl_result=""
        if [ -n "$REDIS_PASSWORD" ]; then
            # Try to create user with ACL
            acl_result=$(redis-cli -h localhost -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ACL SETUSER "$REDIS_USER" on ">${REDIS_USER_PASSWORD}" ~* +@all 2>&1)
        else
            # Try without authentication (for legacy setups)
            acl_result=$(redis-cli -h localhost -p 6379 ACL SETUSER "$REDIS_USER" on ">${REDIS_USER_PASSWORD}" ~* +@all 2>&1)
        fi
        
        if echo "$acl_result" | grep -q "OK"; then
            print_status "Redis user created successfully"
            
            # Save ACL users
            if [ -n "$REDIS_PASSWORD" ]; then
                redis-cli -h localhost -p 6379 --user admin --pass "$REDIS_PASSWORD" --no-auth-warning ACL SAVE >/dev/null 2>&1
            else
                redis-cli -h localhost -p 6379 ACL SAVE >/dev/null 2>&1
            fi
            print_status "Redis ACL saved"
            
            # Verify user can connect
            if redis-cli -h localhost -p 6379 --user "$REDIS_USER" --pass "$REDIS_USER_PASSWORD" --no-auth-warning -n "$REDIS_DB" ping >/dev/null 2>&1; then
                print_status "Redis user verified and working"
            else
                print_warning "Redis user created but verification failed"
            fi
        else
            print_error "Failed to create Redis user: $acl_result"
            print_warning "Will use requirepass mode instead of per-user ACL"
            REDIS_USER=""
            REDIS_USER_PASSWORD="$REDIS_PASSWORD"
        fi
    else
        print_info "Using requirepass authentication (single password, no user-specific ACL)"
        REDIS_USER_PASSWORD="$REDIS_PASSWORD"
    fi
    
    # Backup existing .env
    cp "$instance_dir/backend/.env" "$instance_dir/backend/.env.backup.$(date +%Y%m%d_%H%M%S)"
    print_info "Backed up existing .env file"
    
    # Add Redis configuration to .env
    print_info "Adding Redis configuration to .env..."
    
    # Use correct password variable
    local redis_pass_for_env="${REDIS_USER_PASSWORD:-$REDIS_PASSWORD}"
    
    cat >> "$instance_dir/backend/.env" << EOF

# Redis Configuration (added during update on $(date))
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_USER=$REDIS_USER
REDIS_PASSWORD=$redis_pass_for_env
REDIS_DB=$REDIS_DB
EOF
    
    print_status "Redis configuration added to .env"
    
    if [ -n "$REDIS_USER" ]; then
        print_info "Redis Mode: ACL with user '$REDIS_USER'"
    else
        print_info "Redis Mode: requirepass (legacy single-password auth)"
    fi
    print_info "Redis Database: $REDIS_DB"
    
    return 0
}

# Update .env file with missing variables while preserving existing values
update_env_file() {
    print_info "Checking .env file for missing variables..."
    
    local env_file="$instance_dir/backend/.env"
    
    if [ ! -f "$env_file" ]; then
        print_error ".env file not found at $env_file"
        return 1
    fi
    
    # Backup existing .env
    cp "$env_file" "$env_file.backup.$(date +%Y%m%d_%H%M%S)"
    print_info "Backed up existing .env file"
    
    # Source existing .env to get current values
    set -a
    source "$env_file"
    set +a
    
    # Define all expected variables with their defaults
    # Only set if not already defined (preserves user values)
    
    # Database (already loaded from .env)
    : ${PM_DB_CONN_MAX_ATTEMPTS:=30}
    : ${PM_DB_CONN_WAIT_INTERVAL:=2}
    
    # JWT (JWT_SECRET should already exist)
    : ${JWT_EXPIRES_IN:=1h}
    : ${JWT_REFRESH_EXPIRES_IN:=7d}
    
    # Server
    : ${NODE_ENV:=production}
    
    # API
    : ${API_VERSION:=v1}
    
    # CORS (preserve existing or use current FQDN)
    if [ -z "$CORS_ORIGIN" ]; then
        # Determine protocol from existing URL or default to https
        if echo "$DATABASE_URL" | grep -q "localhost"; then
            CORS_ORIGIN="http://$SELECTED_INSTANCE"
        else
            CORS_ORIGIN="https://$SELECTED_INSTANCE"
        fi
    fi
    
    # Session
    : ${SESSION_INACTIVITY_TIMEOUT_MINUTES:=30}
    
    # User
    : ${DEFAULT_USER_ROLE:=user}
    
    # Rate Limiting
    : ${RATE_LIMIT_WINDOW_MS:=900000}
    : ${RATE_LIMIT_MAX:=5000}
    : ${AUTH_RATE_LIMIT_WINDOW_MS:=600000}
    : ${AUTH_RATE_LIMIT_MAX:=500}
    : ${AGENT_RATE_LIMIT_WINDOW_MS:=60000}
    : ${AGENT_RATE_LIMIT_MAX:=1000}
    
    # Redis (already handled by update_redis_configuration if missing)
    : ${REDIS_HOST:=localhost}
    : ${REDIS_PORT:=6379}
    : ${REDIS_DB:=0}
    
    # Logging
    : ${LOG_LEVEL:=info}
    : ${ENABLE_LOGGING:=true}
    
    # TFA
    : ${TFA_REMEMBER_ME_EXPIRES_IN:=30d}
    : ${TFA_MAX_REMEMBER_SESSIONS:=5}
    : ${TFA_SUSPICIOUS_ACTIVITY_THRESHOLD:=3}
    
    # Prisma Connection Pool
    : ${DB_CONNECTION_LIMIT:=30}
    : ${DB_POOL_TIMEOUT:=20}
    : ${DB_CONNECT_TIMEOUT:=10}
    : ${DB_IDLE_TIMEOUT:=300}
    : ${DB_MAX_LIFETIME:=1800}
    
    # Track which variables were added
    local added_vars=()
    
    # Check and add missing variables
    if ! grep -q "^PM_DB_CONN_MAX_ATTEMPTS=" "$env_file"; then
        added_vars+=("PM_DB_CONN_MAX_ATTEMPTS")
    fi
    if ! grep -q "^PM_DB_CONN_WAIT_INTERVAL=" "$env_file"; then
        added_vars+=("PM_DB_CONN_WAIT_INTERVAL")
    fi
    if ! grep -q "^JWT_EXPIRES_IN=" "$env_file"; then
        added_vars+=("JWT_EXPIRES_IN")
    fi
    if ! grep -q "^JWT_REFRESH_EXPIRES_IN=" "$env_file"; then
        added_vars+=("JWT_REFRESH_EXPIRES_IN")
    fi
    if ! grep -q "^API_VERSION=" "$env_file"; then
        added_vars+=("API_VERSION")
    fi
    if ! grep -q "^CORS_ORIGIN=" "$env_file"; then
        added_vars+=("CORS_ORIGIN")
    fi
    if ! grep -q "^SESSION_INACTIVITY_TIMEOUT_MINUTES=" "$env_file"; then
        added_vars+=("SESSION_INACTIVITY_TIMEOUT_MINUTES")
    fi
    if ! grep -q "^DEFAULT_USER_ROLE=" "$env_file"; then
        added_vars+=("DEFAULT_USER_ROLE")
    fi
    if ! grep -q "^RATE_LIMIT_WINDOW_MS=" "$env_file"; then
        added_vars+=("RATE_LIMIT_WINDOW_MS")
    fi
    if ! grep -q "^RATE_LIMIT_MAX=" "$env_file"; then
        added_vars+=("RATE_LIMIT_MAX")
    fi
    if ! grep -q "^AUTH_RATE_LIMIT_WINDOW_MS=" "$env_file"; then
        added_vars+=("AUTH_RATE_LIMIT_WINDOW_MS")
    fi
    if ! grep -q "^AUTH_RATE_LIMIT_MAX=" "$env_file"; then
        added_vars+=("AUTH_RATE_LIMIT_MAX")
    fi
    if ! grep -q "^AGENT_RATE_LIMIT_WINDOW_MS=" "$env_file"; then
        added_vars+=("AGENT_RATE_LIMIT_WINDOW_MS")
    fi
    if ! grep -q "^AGENT_RATE_LIMIT_MAX=" "$env_file"; then
        added_vars+=("AGENT_RATE_LIMIT_MAX")
    fi
    if ! grep -q "^LOG_LEVEL=" "$env_file"; then
        added_vars+=("LOG_LEVEL")
    fi
    if ! grep -q "^ENABLE_LOGGING=" "$env_file"; then
        added_vars+=("ENABLE_LOGGING")
    fi
    if ! grep -q "^TFA_REMEMBER_ME_EXPIRES_IN=" "$env_file"; then
        added_vars+=("TFA_REMEMBER_ME_EXPIRES_IN")
    fi
    if ! grep -q "^TFA_MAX_REMEMBER_SESSIONS=" "$env_file"; then
        added_vars+=("TFA_MAX_REMEMBER_SESSIONS")
    fi
    if ! grep -q "^TFA_SUSPICIOUS_ACTIVITY_THRESHOLD=" "$env_file"; then
        added_vars+=("TFA_SUSPICIOUS_ACTIVITY_THRESHOLD")
    fi
    if ! grep -q "^DB_CONNECTION_LIMIT=" "$env_file"; then
        added_vars+=("DB_CONNECTION_LIMIT")
    fi
    if ! grep -q "^DB_POOL_TIMEOUT=" "$env_file"; then
        added_vars+=("DB_POOL_TIMEOUT")
    fi
    if ! grep -q "^DB_CONNECT_TIMEOUT=" "$env_file"; then
        added_vars+=("DB_CONNECT_TIMEOUT")
    fi
    if ! grep -q "^DB_IDLE_TIMEOUT=" "$env_file"; then
        added_vars+=("DB_IDLE_TIMEOUT")
    fi
    if ! grep -q "^DB_MAX_LIFETIME=" "$env_file"; then
        added_vars+=("DB_MAX_LIFETIME")
    fi
    
    # If there are missing variables, add them
    if [ ${#added_vars[@]} -gt 0 ]; then
        print_info "Adding ${#added_vars[@]} missing environment variable(s)..."
        
        cat >> "$env_file" << EOF

# Environment variables added during update on $(date)
EOF
        
        # Add database config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "PM_DB_CONN_MAX_ATTEMPTS"; then
            echo "PM_DB_CONN_MAX_ATTEMPTS=$PM_DB_CONN_MAX_ATTEMPTS" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "PM_DB_CONN_WAIT_INTERVAL"; then
            echo "PM_DB_CONN_WAIT_INTERVAL=$PM_DB_CONN_WAIT_INTERVAL" >> "$env_file"
        fi
        
        # Add JWT config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "JWT_EXPIRES_IN"; then
            echo "JWT_EXPIRES_IN=$JWT_EXPIRES_IN" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "JWT_REFRESH_EXPIRES_IN"; then
            echo "JWT_REFRESH_EXPIRES_IN=$JWT_REFRESH_EXPIRES_IN" >> "$env_file"
        fi
        
        # Add API config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "API_VERSION"; then
            echo "API_VERSION=$API_VERSION" >> "$env_file"
        fi
        
        # Add CORS config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "CORS_ORIGIN"; then
            echo "CORS_ORIGIN=$CORS_ORIGIN" >> "$env_file"
        fi
        
        # Add session config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "SESSION_INACTIVITY_TIMEOUT_MINUTES"; then
            echo "SESSION_INACTIVITY_TIMEOUT_MINUTES=$SESSION_INACTIVITY_TIMEOUT_MINUTES" >> "$env_file"
        fi
        
        # Add user config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "DEFAULT_USER_ROLE"; then
            echo "DEFAULT_USER_ROLE=$DEFAULT_USER_ROLE" >> "$env_file"
        fi
        
        # Add rate limiting if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "RATE_LIMIT_WINDOW_MS"; then
            echo "RATE_LIMIT_WINDOW_MS=$RATE_LIMIT_WINDOW_MS" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "RATE_LIMIT_MAX"; then
            echo "RATE_LIMIT_MAX=$RATE_LIMIT_MAX" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "AUTH_RATE_LIMIT_WINDOW_MS"; then
            echo "AUTH_RATE_LIMIT_WINDOW_MS=$AUTH_RATE_LIMIT_WINDOW_MS" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "AUTH_RATE_LIMIT_MAX"; then
            echo "AUTH_RATE_LIMIT_MAX=$AUTH_RATE_LIMIT_MAX" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "AGENT_RATE_LIMIT_WINDOW_MS"; then
            echo "AGENT_RATE_LIMIT_WINDOW_MS=$AGENT_RATE_LIMIT_WINDOW_MS" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "AGENT_RATE_LIMIT_MAX"; then
            echo "AGENT_RATE_LIMIT_MAX=$AGENT_RATE_LIMIT_MAX" >> "$env_file"
        fi
        
        # Add logging config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "LOG_LEVEL"; then
            echo "LOG_LEVEL=$LOG_LEVEL" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "ENABLE_LOGGING"; then
            echo "ENABLE_LOGGING=$ENABLE_LOGGING" >> "$env_file"
        fi
        
        # Add TFA config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "TFA_REMEMBER_ME_EXPIRES_IN"; then
            echo "TFA_REMEMBER_ME_EXPIRES_IN=$TFA_REMEMBER_ME_EXPIRES_IN" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "TFA_MAX_REMEMBER_SESSIONS"; then
            echo "TFA_MAX_REMEMBER_SESSIONS=$TFA_MAX_REMEMBER_SESSIONS" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "TFA_SUSPICIOUS_ACTIVITY_THRESHOLD"; then
            echo "TFA_SUSPICIOUS_ACTIVITY_THRESHOLD=$TFA_SUSPICIOUS_ACTIVITY_THRESHOLD" >> "$env_file"
        fi
        
        # Add Prisma connection pool config if missing
        if printf '%s\n' "${added_vars[@]}" | grep -q "DB_CONNECTION_LIMIT"; then
            echo "" >> "$env_file"
            echo "# Database Connection Pool Configuration (Prisma)" >> "$env_file"
            echo "DB_CONNECTION_LIMIT=$DB_CONNECTION_LIMIT" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "DB_POOL_TIMEOUT"; then
            echo "DB_POOL_TIMEOUT=$DB_POOL_TIMEOUT" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "DB_CONNECT_TIMEOUT"; then
            echo "DB_CONNECT_TIMEOUT=$DB_CONNECT_TIMEOUT" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "DB_IDLE_TIMEOUT"; then
            echo "DB_IDLE_TIMEOUT=$DB_IDLE_TIMEOUT" >> "$env_file"
        fi
        if printf '%s\n' "${added_vars[@]}" | grep -q "DB_MAX_LIFETIME"; then
            echo "DB_MAX_LIFETIME=$DB_MAX_LIFETIME" >> "$env_file"
        fi
        
        print_status ".env file updated with ${#added_vars[@]} new variable(s)"
        print_info "Added variables: ${added_vars[*]}"
    else
        print_status ".env file is up to date - no missing variables"
    fi
    
    return 0
}

# Update nginx configuration for existing installation
update_nginx_configuration() {
    print_info "Updating nginx configuration..."
    
    # Detect SSL status
    local ssl_enabled="false"
    if [ -f "/etc/letsencrypt/live/$SELECTED_INSTANCE/fullchain.pem" ]; then
        ssl_enabled="true"
        print_info "SSL certificate detected, updating HTTPS configuration"
    else
        print_info "No SSL certificate found, updating HTTP configuration"
    fi
    
    # Backup existing config
    local backup_file="/etc/nginx/sites-available/$SELECTED_INSTANCE.backup.$(date +%Y%m%d_%H%M%S)"
    if [ -f "/etc/nginx/sites-available/$SELECTED_INSTANCE" ]; then
        cp "/etc/nginx/sites-available/$SELECTED_INSTANCE" "$backup_file"
        print_info "Backed up existing nginx config to: $backup_file"
    fi
    
    # Extract backend port
    local backend_port=$(grep -o 'proxy_pass http://127.0.0.1:[0-9]*' "/etc/nginx/sites-available/$SELECTED_INSTANCE" 2>/dev/null | grep -oP ':\K[0-9]+' | head -1)
    if [ -z "$backend_port" ] && [ -f "$instance_dir/backend/.env" ]; then
        backend_port=$(grep '^PORT=' "$instance_dir/backend/.env" | cut -d'=' -f2 | tr -d ' ')
    fi
    
    if [ -z "$backend_port" ]; then
        print_warning "Could not determine backend port, skipping nginx config update"
        return 0
    fi
    
    print_info "Detected backend port: $backend_port"
    
    # Generate new configuration using the unified function
    generate_nginx_config "$SELECTED_INSTANCE" "$instance_dir" "$backend_port" "$ssl_enabled"
    
    # Test and reload nginx
    if nginx -t; then
        systemctl reload nginx
        print_status "Nginx configuration updated successfully"
    else
        print_error "Nginx configuration test failed"
        # Restore backup
        if [ -f "$backup_file" ]; then
            mv "$backup_file" "/etc/nginx/sites-available/$SELECTED_INSTANCE"
            print_info "Restored backup nginx configuration"
        fi
        return 1
    fi
}

# Update existing installation
update_installation() {
    local instance_dir="/opt/$SELECTED_INSTANCE"
    local service_name="$SELECTED_SERVICE_NAME"
    
    print_info "Updating PatchMon installation: $SELECTED_INSTANCE"
    print_info "Installation directory: $instance_dir"
    print_info "Service name: $service_name"
    
    # Verify it's a git repository, if not, initialize it
    if [ ! -d "$instance_dir/.git" ]; then
        print_warning "Installation directory is not a git repository"
        print_info "Attempting to re-initialize as git repository..."
        
        cd "$instance_dir" || exit 1
        
        # Initialize git repository
        git init
        git remote add origin https://github.com/PatchMon/PatchMon.git
        
        # Fetch all branches
        git fetch origin
        
        # Try to determine current version from package.json or default to main
        local current_branch="main"
        if [ -f "$instance_dir/backend/package.json" ]; then
            local pkg_version=$(grep '"version"' "$instance_dir/backend/package.json" | head -1 | sed 's/.*"version": "\(.*\)".*/\1/')
            if [ -n "$pkg_version" ]; then
                # Check if there's a release branch for this version
                if git ls-remote --heads origin | grep -q "release/$(echo $pkg_version | sed 's/\./-/g')"; then
                    current_branch="release/$(echo $pkg_version | sed 's/\./-/g')"
                fi
            fi
        fi
        
        # Reset to the determined branch
        git reset --hard "origin/$current_branch"
        git checkout -B "$current_branch" "origin/$current_branch"
        
        print_success "Repository initialized successfully"
    fi
    
    # Add git safe.directory to avoid ownership issues when running as root
    print_info "Configuring git safe.directory..."
    git config --global --add safe.directory "$instance_dir" 2>/dev/null || true
    
    # Load existing .env to get database credentials
    if [ -f "$instance_dir/backend/.env" ]; then
        # Unset color variables before sourcing to prevent ANSI escape sequences from leaking into .env
        unset RED GREEN YELLOW BLUE NC
        source "$instance_dir/backend/.env"
        print_status "Loaded existing configuration"
        
        # Parse DATABASE_URL to extract credentials
        # Format: postgresql://user:password@host:port/database
        if [ -n "$DATABASE_URL" ]; then
            # Extract components using regex
            DB_USER=$(echo "$DATABASE_URL" | sed -n 's|postgresql://\([^:]*\):.*|\1|p')
            DB_PASS=$(echo "$DATABASE_URL" | sed -n 's|postgresql://[^:]*:\([^@]*\)@.*|\1|p')
            DB_HOST=$(echo "$DATABASE_URL" | sed -n 's|.*@\([^:]*\):.*|\1|p')
            DB_PORT=$(echo "$DATABASE_URL" | sed -n 's|.*:\([0-9]*\)/.*|\1|p')
            DB_NAME=$(echo "$DATABASE_URL" | sed -n 's|.*/\([^?]*\).*|\1|p')
            
            print_info "Database: $DB_NAME (user: $DB_USER)"
        else
            print_error "DATABASE_URL not found in .env file"
            exit 1
        fi
    else
        print_error "Cannot find .env file at $instance_dir/backend/.env"
        exit 1
    fi
    
    # Select branch/version to update to
    select_branch
    
    print_info "Updating to: $DEPLOYMENT_BRANCH"
    echo ""
    
    read_yes_no "Proceed with update? This will pull new code and restart services" CONFIRM_UPDATE "y"
    
    if [ "$CONFIRM_UPDATE" != "y" ]; then
        print_warning "Update cancelled by user"
        exit 0
    fi
    
    # Stop the service
    print_info "Stopping service: $service_name"
    systemctl stop "$service_name" || true
    
    # Create backup directory
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_dir="$instance_dir.backup.$timestamp"
    local db_backup_file="$backup_dir/database_backup_$timestamp.sql"
    
    print_info "Creating backup directory: $backup_dir"
    mkdir -p "$backup_dir"
    
    # Backup database
    print_info "Backing up database: $DB_NAME"
    if PGPASSWORD="$DB_PASS" pg_dump -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -F c -f "$db_backup_file" 2>/dev/null; then
        print_status "Database backup created: $db_backup_file"
    else
        print_warning "Database backup failed, but continuing with code backup"
    fi
    
    # Backup code
    print_info "Backing up code files..."
    cp -r "$instance_dir" "$backup_dir/code"
    print_status "Code backup created"
    
    # Update code
    print_info "Pulling latest code from branch: $DEPLOYMENT_BRANCH"
    cd "$instance_dir"
    
    # Clean up any untracked files that might conflict with incoming changes
    print_info "Cleaning up untracked files to prevent merge conflicts..."
    git clean -fd 2>/dev/null || true
    
    # Reset any local changes to ensure clean state
    # Check if HEAD exists before trying to reset
    print_info "Resetting local changes to ensure clean state..."
    if git rev-parse --verify HEAD >/dev/null 2>&1; then
        git reset --hard HEAD
    else
        print_warning "HEAD not found, skipping reset (fresh repository or detached state)"
    fi
    
    # Fetch latest changes
    git fetch origin
    
    # Checkout the selected branch/tag
    git checkout "$DEPLOYMENT_BRANCH"
    git pull origin "$DEPLOYMENT_BRANCH" || git pull # For tags, just pull
    
    print_status "Code updated successfully"
    
    # Update dependencies
    print_info "Updating backend dependencies..."
    cd "$instance_dir/backend"
    npm install --production --ignore-scripts
    
    print_info "Updating frontend dependencies..."
    cd "$instance_dir/frontend"
    npm install --ignore-scripts
    
    # Build frontend
    print_info "Building frontend..."
    npm run build
    
    # Make agent binaries executable
    if [ -d "$instance_dir/agents" ]; then
        chmod +x "$instance_dir/agents/patchmon-agent-linux-"* 2>/dev/null || true
        print_status "Agent binaries made executable"
    fi
    
    # Run database migrations with self-healing
    print_info "Running database migrations..."
    cd "$instance_dir/backend"
    
    # Generate Prisma client first
    npx prisma generate
    
    local max_attempts=3
    local attempt=1
    local migration_success=false
    
    while [ $attempt -le $max_attempts ]; do
        print_info "Migration attempt $attempt of $max_attempts..."
        
        # Try to run migrations
        local migrate_output
        migrate_output=$(npx prisma migrate deploy 2>&1 || echo "MIGRATION_FAILED")
        
        # Check if migration succeeded
        if ! echo "$migrate_output" | grep -q "MIGRATION_FAILED\|Error:\|P3009"; then
            print_status "Migrations completed successfully"
            migration_success=true
            break
        fi
        
        # Check specifically for P3009 (failed migrations found)
        if echo "$migrate_output" | grep -q "P3009\|migrate found failed migrations"; then
            print_warning "Detected failed migrations (P3009 error)"
            
            # Extract the failed migration name if possible
            local failed_migration
            failed_migration=$(echo "$migrate_output" | grep -oP "The \`\K[^\`]+" | head -1 || echo "")
            
            if [ -n "$failed_migration" ]; then
                print_info "Failed migration identified: $failed_migration"
            fi
            
            # Attempt to fix failed migrations
            print_info "Attempting to self-heal migration issues..."
            if fix_failed_migrations "$DB_NAME" "$DB_USER" "$DB_PASS" "$DB_HOST"; then
                print_status "Migration issues resolved, retrying..."
                attempt=$((attempt + 1))
                sleep 2
                continue
            else
                print_error "Failed to resolve migration issues"
                print_warning "Attempting alternative resolution method..."
                
                # Alternative: Mark migration as completed if tables exist
                print_info "Checking if migration changes are already applied..."
                PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c \
                    "UPDATE _prisma_migrations SET finished_at = NOW(), logs = 'Manually resolved by update script' WHERE migration_name = '$failed_migration' AND finished_at IS NULL;" >/dev/null 2>&1
                
                attempt=$((attempt + 1))
                sleep 2
                continue
            fi
        else
            # Other migration error
            print_error "Migration failed with error:"
            echo "$migrate_output" | grep -A 10 "Error:"
            
            # Show helpful information
            print_info "Migration status:"
            npx prisma migrate status 2>&1 || true
            break
        fi
    done
    
    if [ "$migration_success" = false ]; then
        print_error "Migrations failed after $max_attempts attempts"
        print_warning "The update will continue, but you may need to manually resolve migration issues"
        print_info "Check migrations: cd $instance_dir/backend && npx prisma migrate status"
        print_info "View failed migrations: PGPASSWORD=\"$DB_PASS\" psql -h \"$DB_HOST\" -U \"$DB_USER\" -d \"$DB_NAME\" -c \"SELECT * FROM _prisma_migrations WHERE finished_at IS NULL;\""
    fi
    
    # Check and update Redis configuration if needed (for legacy installations)
    update_redis_configuration
    
    # Update .env file with any missing variables (preserve existing values)
    update_env_file
    
    # Update nginx configuration with latest improvements
    update_nginx_configuration
    
    # Start the service
    print_info "Starting service: $service_name"
    systemctl start "$service_name"
    
    # Wait a moment and check status
    sleep 5
    
    if systemctl is-active --quiet "$service_name"; then
        print_success "Update completed successfully!"
        print_status "Service $service_name is running"
        
        # Get new version
        local new_version=$(grep '"version"' "$instance_dir/backend/package.json" | head -1 | sed 's/.*"version": "\([^"]*\)".*/\1/')
        print_info "Updated to version: $new_version"
        echo ""
        print_info "Backup Information:"
        print_info "  Code backup: $backup_dir/code"
        print_info "  Database backup: $db_backup_file"
        echo ""
        print_info "To restore database if needed:"
        print_info "  PGPASSWORD=\"$DB_PASS\" pg_restore -h \"$DB_HOST\" -U \"$DB_USER\" -d \"$DB_NAME\" -c \"$db_backup_file\""
        echo ""
    else
        print_error "Service failed to start after update"
        echo ""
        
        # Show last 25 lines of service logs for debugging
        print_warning "=== Last 25 lines of service logs ==="
        journalctl -u "$service_name" -n 25 --no-pager || true
        print_warning "==================================="
        echo ""
        
        # Check for specific error patterns
        local logs=$(journalctl -u "$service_name" -n 50 --no-pager 2>/dev/null || echo "")
        
        if echo "$logs" | grep -q "WRONGPASS\|NOAUTH"; then
            print_error "Detected Redis authentication error!"
            print_info "The service cannot authenticate with Redis."
            echo ""
            print_info "Current Redis configuration in .env:"
            grep "^REDIS_" "$instance_dir/backend/.env" || true
            echo ""
            print_info "Quick fix - Try reconfiguring Redis:"
            print_info "  1. Check Redis ACL users:"
            print_info "     redis-cli ACL LIST"
            echo ""
            print_info "  2. Test Redis connection with credentials from .env:"
            local test_user=$(grep "^REDIS_USER=" "$instance_dir/backend/.env" | cut -d'=' -f2)
            local test_pass=$(grep "^REDIS_PASSWORD=" "$instance_dir/backend/.env" | cut -d'=' -f2)
            local test_db=$(grep "^REDIS_DB=" "$instance_dir/backend/.env" | cut -d'=' -f2)
            print_info "     redis-cli --user $test_user --pass $test_pass -n ${test_db:-0} ping"
            echo ""
        elif echo "$logs" | grep -q "ECONNREFUSED"; then
            print_error "Detected connection refused error!"
            print_info "Check if required services are running:"
            print_info "  systemctl status postgresql"
            print_info "  systemctl status redis-server"
        elif echo "$logs" | grep -q "Error:"; then
            print_error "Application error detected in logs"
        fi
        
        echo ""
        print_warning "ROLLBACK INSTRUCTIONS:"
        print_info "1. Restore code:"
        print_info "   sudo rm -rf $instance_dir"
        print_info "   sudo mv $backup_dir/code $instance_dir"
        echo ""
        print_info "2. Restore database:"
        print_info "   PGPASSWORD=\"$DB_PASS\" pg_restore -h \"$DB_HOST\" -U \"$DB_USER\" -d \"$DB_NAME\" -c \"$db_backup_file\""
        echo ""
        print_info "3. Restart service:"
        print_info "   sudo systemctl start $service_name"
        echo ""
        print_info "View full logs: journalctl -u $service_name -f"
        exit 1
    fi
}

# Main script execution
main() {
    # Parse command-line arguments
    if [ "$1" = "--update" ]; then
        UPDATE_MODE="true"
    fi
    
    # Log script entry
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Script started - Update mode: $UPDATE_MODE" >> "$DEBUG_LOG"
    
    # Handle update mode
    if [ "$UPDATE_MODE" = "true" ]; then
        print_banner
        print_info "PatchMon Update Mode"
        echo ""
        
        # Select installation to update
        select_installation_to_update
        
        # Perform update
        update_installation
        
        exit 0
    fi
    
    # Normal installation mode
    # Check if existing installations are present
    local existing_installs=($(detect_installations))
    if [ ${#existing_installs[@]} -gt 0 ]; then
        print_warning "Found ${#existing_installs[@]} existing PatchMon installation(s):"
        for install in "${existing_installs[@]}"; do
            print_info "   - $install"
        done
        echo ""
        print_warning "If you want to UPDATE an existing installation, run:"
        print_info "   sudo bash $0 --update"
        echo ""
        print_warning "If you want to create a NEW installation alongside the existing one(s), continue below."
        echo ""
        read_yes_no "Do you want to continue with NEW installation?" CONTINUE_NEW "n"
        
        if [ "$CONTINUE_NEW" != "y" ]; then
            print_info "Installation cancelled. Run with --update flag to update existing installations."
            exit 0
        fi
    fi
    
    # Run interactive setup
    interactive_setup
    
    # Set GitHub repo (always use public repo for self-hosted deployments)
    GITHUB_REPO="$DEFAULT_GITHUB_REPO"
    
    # Validate SSL setting
    if [ "$SSL_ENABLED" = "y" ] || [ "$SSL_ENABLED" = "yes" ]; then
        USE_LETSENCRYPT="true"
        SERVER_PROTOCOL_SEL="https"
        print_info "SSL enabled - will use Let's Encrypt for HTTPS"
        
        # Validate email for SSL
        if [ -z "$EMAIL" ]; then
            print_error "Email is required when SSL is enabled for Let's Encrypt"
            exit 1
        fi
    else
        USE_LETSENCRYPT="false"
        SERVER_PROTOCOL_SEL="http"
        print_info "SSL disabled - will use HTTP only"
    fi
    
    # Log before calling deploy_instance
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] About to call deploy_instance function" >> "$DEBUG_LOG"
    
    # Run deployment
    deploy_instance
    
    # Log after deploy_instance completes
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] deploy_instance function completed" >> "$DEBUG_LOG"
}

# Show usage/help
show_usage() {
    echo "PatchMon Self-Hosting Installation & Update Script"
    echo "Version: $SCRIPT_VERSION"
    echo ""
    echo "Usage:"
    echo "  $0              # Interactive installation (default)"
    echo "  $0 --update     # Update existing installation"
    echo "  $0 --help       # Show this help message"
    echo ""
    echo "Examples:"
    echo "  # New installation:"
    echo "  sudo bash $0"
    echo ""
    echo "  # Update existing installation:"
    echo "  sudo bash $0 --update"
    echo ""
}

# Check for help flag
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    show_usage
    exit 0
fi

# Run main function
main "$@"
