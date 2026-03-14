#!/bin/bash
# PatchMon Diagnostics Collection Script
# Collects system information, logs, and configuration for troubleshooting
# Usage: sudo bash diagnostics.sh [instance-name]

# Note: Not using 'set -e' because we want to continue even if some commands fail
set -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_status() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}🎉 $1${NC}"
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    print_error "This script must be run as root"
    print_info "Please run: sudo bash $0"
    exit 1
fi

# Function to sanitize sensitive information
sanitize_sensitive() {
    local input="$1"
    # Replace passwords, secrets, and tokens with [REDACTED]
    echo "$input" | \
        sed -E 's/(PASSWORD|SECRET|TOKEN|KEY|PASS)=[^"]*$/\1=[REDACTED]/gi' | \
        sed -E 's/(PASSWORD|SECRET|TOKEN|KEY|PASS)="[^"]*"/\1="[REDACTED]"/gi' | \
        sed -E 's/(password|secret|token|key|pass)": *"[^"]*"/\1": "[REDACTED]"/gi' | \
        sed -E 's/(>)[a-zA-Z0-9+\/=]{20,}/\1[REDACTED]/g' | \
        sed -E 's|postgresql://([^:]+):([^@]+)@|postgresql://\1:[REDACTED]@|g' | \
        sed -E 's|mysql://([^:]+):([^@]+)@|mysql://\1:[REDACTED]@|g' | \
        sed -E 's|mongodb://([^:]+):([^@]+)@|mongodb://\1:[REDACTED]@|g'
}

# Function to detect PatchMon installations
detect_installations() {
    local installations=()
    
    if [ ! -d "/opt" ]; then
        print_error "/opt directory does not exist"
        return 1
    fi
    
    for dir in /opt/*/; do
        # Skip if no directories found
        [ -d "$dir" ] || continue
        
        local dirname=$(basename "$dir")
        
        # Skip backup directories
        if [[ "$dirname" =~ \.backup\. ]]; then
            continue
        fi
        
        # Check if it's a PatchMon installation
        if [ -f "$dir/backend/package.json" ]; then
            if grep -q "patchmon" "$dir/backend/package.json" 2>/dev/null; then
                installations+=("$dirname")
            fi
        fi
    done
    
    echo "${installations[@]}"
}

# Function to select installation
select_installation() {
    local installations=($(detect_installations))
    
    if [ ${#installations[@]} -eq 0 ]; then
        print_error "No PatchMon installations found in /opt" >&2
        exit 1
    fi
    
    if [ -n "$1" ]; then
        # Use provided instance name
        if [[ " ${installations[@]} " =~ " $1 " ]]; then
            echo "$1"
            return 0
        else
            print_error "Instance '$1' not found" >&2
            exit 1
        fi
    fi
    
    # Send status messages to stderr so they don't contaminate the return value
    print_info "Found ${#installations[@]} installation(s):" >&2
    echo "" >&2
    
    local i=1
    declare -A install_map
    for install in "${installations[@]}"; do
        # Get service status
        local status="unknown"
        if systemctl is-active --quiet "$install" 2>/dev/null; then
            status="${GREEN}running${NC}"
        elif systemctl is-enabled --quiet "$install" 2>/dev/null; then
            status="${RED}stopped${NC}"
        fi
        
        printf "%2d. %-30s (%b)\n" "$i" "$install" "$status" >&2
        install_map[$i]="$install"
        i=$((i + 1))
    done
    
    echo "" >&2
    
    # If only one installation, select it automatically
    if [ ${#installations[@]} -eq 1 ]; then
        print_info "Only one installation found, selecting automatically: ${installations[0]}" >&2
        echo "${installations[0]}"
        return 0
    fi
    
    # Multiple installations - prompt user
    printf "${BLUE}Select installation number [1]: ${NC}" >&2
    read -r selection </dev/tty
    
    selection=${selection:-1}
    
    if [[ "$selection" =~ ^[0-9]+$ ]] && [ -n "${install_map[$selection]}" ]; then
        echo "${install_map[$selection]}"
        return 0
    else
        print_error "Invalid selection" >&2
        exit 1
    fi
}

# Main script
main() {
    # Capture the directory where script is run from at the very start
    ORIGINAL_DIR=$(pwd)
    
    echo -e "${BLUE}====================================================${NC}"
    echo -e "${BLUE}        PatchMon Diagnostics Collection${NC}"
    echo -e "${BLUE}====================================================${NC}"
    echo ""
    
    # Select instance
    instance_name=$(select_installation "$1")
    instance_dir="/opt/$instance_name"
    
    print_info "Selected instance: $instance_name"
    print_info "Directory: $instance_dir"
    echo ""
    
    # Create single diagnostics file in the original directory
    timestamp=$(date +%Y%m%d_%H%M%S)
    diag_file="${ORIGINAL_DIR}/patchmon_diagnostics_${instance_name}_${timestamp}.txt"
    
    print_info "Collecting diagnostics to: $diag_file"
    echo ""
    
    # Initialize the diagnostics file with header
    cat > "$diag_file" << EOF
===================================================
PatchMon Diagnostics Report
===================================================
Instance: $instance_name
Generated: $(date)
Hostname: $(hostname)
Generated from: ${ORIGINAL_DIR}
===================================================

EOF
    
    # ========================================
    # 1. System Information
    # ========================================
    print_info "Collecting system information..."
    
    cat >> "$diag_file" << EOF
=== System Information ===
OS: $(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'"' -f2 || echo "Unknown")
Kernel: $(uname -r)
Uptime: $(uptime)

=== CPU Information ===
$(lscpu | grep -E "Model name|CPU\(s\)|Thread|Core" || echo "Not available")

=== Memory Information ===
$(free -h)

=== Disk Usage ===
$(df -h | grep -E "Filesystem|/dev/|/opt")

=== Network Interfaces ===
$(ip -br addr)

===================================================
EOF
    
    # ========================================
    # 2. PatchMon Instance Information
    # ========================================
    print_info "Collecting instance information..."
    
    cat >> "$diag_file" << EOF

=== PatchMon Instance Information ===

=== Directory Structure ===
$(ls -lah "$instance_dir" 2>/dev/null || echo "Cannot access directory")

=== Backend Package Info ===
$(cat "$instance_dir/backend/package.json" 2>/dev/null | grep -E "name|version" || echo "Not found")

=== Frontend Package Info ===
$(cat "$instance_dir/frontend/package.json" 2>/dev/null | grep -E "name|version" || echo "Not found")

=== Deployment Info ===
$(cat "$instance_dir/deployment-info.txt" 2>/dev/null || echo "No deployment-info.txt found")

===================================================
EOF
    
    # ========================================
    # 3. Environment Configuration (Sanitized)
    # ========================================
    print_info "Collecting environment configuration (sanitized)..."
    
    echo "" >> "$diag_file"
    echo "=== Backend Environment Configuration (Sanitized) ===" >> "$diag_file"
    if [ -f "$instance_dir/backend/.env" ]; then
        sanitize_sensitive "$(cat "$instance_dir/backend/.env")" >> "$diag_file"
    else
        echo "Backend .env file not found" >> "$diag_file"
    fi
    echo "" >> "$diag_file"
    
    # ========================================
    # 4. Service Status and Configuration
    # ========================================
    print_info "Collecting service information..."
    
    cat >> "$diag_file" << EOF

=== Service Status and Configuration ===

=== Service Status ===
$(systemctl status "$instance_name" 2>/dev/null || echo "Service not found")

=== Service File ===
$(cat "/etc/systemd/system/${instance_name}.service" 2>/dev/null || echo "Service file not found")

=== Service is-enabled ===
$(systemctl is-enabled "$instance_name" 2>/dev/null || echo "unknown")

=== Service is-active ===
$(systemctl is-active "$instance_name" 2>/dev/null || echo "unknown")

===================================================
EOF
    
    # ========================================
    # 5. Service Logs
    # ========================================
    print_info "Collecting service logs..."
    
    echo "" >> "$diag_file"
    echo "=== Service Logs (last 500 lines) ===" >> "$diag_file"
    journalctl -u "$instance_name" -n 500 --no-pager >> "$diag_file" 2>&1 || \
        echo "Could not retrieve service logs" >> "$diag_file"
    echo "" >> "$diag_file"
    
    # ========================================
    # 6. Nginx Configuration
    # ========================================
    print_info "Collecting nginx configuration..."
    
    cat >> "$diag_file" << EOF

=== Nginx Configuration ===

=== Nginx Status ===
$(systemctl status nginx 2>/dev/null | head -20 || echo "Nginx not found")

=== Site Configuration ===
$(cat "/etc/nginx/sites-available/$instance_name" 2>/dev/null || echo "Nginx config not found")

=== Nginx Error Log (last 100 lines) ===
$(tail -100 /var/log/nginx/error.log 2>/dev/null || echo "Error log not accessible")

=== Nginx Access Log (last 50 lines) ===
$(tail -50 /var/log/nginx/access.log 2>/dev/null || echo "Access log not accessible")

=== Nginx Test ===
$(nginx -t 2>&1 || echo "Nginx test failed")

===================================================
EOF
    
    # ========================================
    # 7. Database Connection Test
    # ========================================
    print_info "Testing database connection..."
    
    echo "" >> "$diag_file"
    echo "=== Database Information ===" >> "$diag_file"
    echo "" >> "$diag_file"
    
    if [ -f "$instance_dir/backend/.env" ]; then
        # Load .env
        set -a
        source "$instance_dir/backend/.env"
        set +a
        
        # Parse DATABASE_URL
        if [ -n "$DATABASE_URL" ]; then
            DB_USER=$(echo "$DATABASE_URL" | sed -n 's|postgresql://\([^:]*\):.*|\1|p')
            DB_PASS=$(echo "$DATABASE_URL" | sed -n 's|postgresql://[^:]*:\([^@]*\)@.*|\1|p')
            DB_HOST=$(echo "$DATABASE_URL" | sed -n 's|.*@\([^:]*\):.*|\1|p')
            DB_PORT=$(echo "$DATABASE_URL" | sed -n 's|.*:\([0-9]*\)/.*|\1|p')
            DB_NAME=$(echo "$DATABASE_URL" | sed -n 's|.*/\([^?]*\).*|\1|p')
            
            cat >> "$diag_file" << EOF
=== Database Connection Details ===
Host: $DB_HOST
Port: $DB_PORT
Database: $DB_NAME
User: $DB_USER

=== PostgreSQL Status ===
$(systemctl status postgresql 2>/dev/null | head -20 || echo "PostgreSQL status not available")

=== Connection Test ===
EOF
            
            if PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "SELECT version();" >> "$diag_file" 2>&1; then
                echo "✅ Database connection: SUCCESSFUL" >> "$diag_file"
            else
                echo "❌ Database connection: FAILED" >> "$diag_file"
            fi
            
            echo "" >> "$diag_file"
            echo "=== Database Size ===" >> "$diag_file"
            PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "
                SELECT 
                    pg_size_pretty(pg_database_size('$DB_NAME')) as database_size;
            " >> "$diag_file" 2>&1 || echo "Could not get database size" >> "$diag_file"
            
            echo "" >> "$diag_file"
            echo "=== Table Sizes ===" >> "$diag_file"
            PGPASSWORD="$DB_PASS" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "
                SELECT 
                    schemaname,
                    tablename,
                    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
                FROM pg_tables
                WHERE schemaname = 'public'
                ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
                LIMIT 10;
            " >> "$diag_file" 2>&1 || echo "Could not get table sizes" >> "$diag_file"
            
            echo "" >> "$diag_file"
            echo "=== Migration Status ===" >> "$diag_file"
            cd "$instance_dir/backend"
            npx prisma migrate status >> "$diag_file" 2>&1 || echo "Could not get migration status" >> "$diag_file"
            
            echo "===================================================" >> "$diag_file"
        else
            echo "DATABASE_URL not found in .env" >> "$diag_file"
        fi
    else
        echo ".env file not found" >> "$diag_file"
    fi
    
    # ========================================
    # 8. Redis Connection Test
    # ========================================
    print_info "Testing Redis connection..."
    
    if [ -f "$instance_dir/backend/.env" ]; then
        # Load .env
        set -a
        source "$instance_dir/backend/.env"
        set +a
        
        cat >> "$diag_file" << EOF
===================================================
Redis Information
===================================================

=== Redis Connection Details ===
Host: ${REDIS_HOST:-localhost}
Port: ${REDIS_PORT:-6379}
User: ${REDIS_USER:-(none)}
Database: ${REDIS_DB:-0}

=== Redis Status ===
$(systemctl status redis-server 2>/dev/null | head -20 || echo "Redis status not available")

=== Connection Test ===
EOF
        
        # Test connection
        if [ -n "$REDIS_USER" ] && [ -n "$REDIS_PASSWORD" ]; then
            if redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" --user "$REDIS_USER" --pass "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" ping >> "$diag_file" 2>&1; then
                echo "✅ Redis connection (with user): SUCCESSFUL" >> "$diag_file"
                
                echo "" >> "$diag_file"
                echo "=== Redis INFO ===" >> "$diag_file"
                redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" --user "$REDIS_USER" --pass "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" INFO >> "$diag_file" 2>&1
                
                echo "" >> "$diag_file"
                echo "=== Redis Database Size ===" >> "$diag_file"
                redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" --user "$REDIS_USER" --pass "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" DBSIZE >> "$diag_file" 2>&1
            else
                echo "❌ Redis connection (with user): FAILED" >> "$diag_file"
            fi
        elif [ -n "$REDIS_PASSWORD" ]; then
            if redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -a "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" ping >> "$diag_file" 2>&1; then
                echo "✅ Redis connection (requirepass): SUCCESSFUL" >> "$diag_file"
                
                echo "" >> "$diag_file"
                echo "=== Redis INFO ===" >> "$diag_file"
                redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -a "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" INFO >> "$diag_file" 2>&1
                
                echo "" >> "$diag_file"
                echo "=== Redis Database Size ===" >> "$diag_file"
                redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -a "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" DBSIZE >> "$diag_file" 2>&1
            else
                echo "❌ Redis connection (requirepass): FAILED" >> "$diag_file"
            fi
        else
            if redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -n "${REDIS_DB:-0}" ping >> "$diag_file" 2>&1; then
                echo "✅ Redis connection (no auth): SUCCESSFUL" >> "$diag_file"
            else
                echo "❌ Redis connection: FAILED" >> "$diag_file"
            fi
        fi
        
        echo "" >> "$diag_file"
        echo "=== Redis ACL Users ===" >> "$diag_file"
        if [ -n "$REDIS_USER" ] && [ -n "$REDIS_PASSWORD" ]; then
            redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" --user "$REDIS_USER" --pass "$REDIS_PASSWORD" --no-auth-warning ACL LIST >> "$diag_file"
        elif [ -n "$REDIS_PASSWORD" ]; then
            redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -a "$REDIS_PASSWORD" --no-auth-warning ACL LIST >> "$diag_file"
        fi
        
        echo "===================================================" >> "$diag_file"
    else
        echo ".env file not found" >> "$diag_file"
    fi
    
    # ========================================
    # 9. Network and Port Information
    # ========================================
    print_info "Collecting network information..."
    
    # Get backend port from .env
    local backend_port=$(grep '^PORT=' "$instance_dir/backend/.env" 2>/dev/null | cut -d'=' -f2 | tr -d ' ' || echo "3000")
    
    cat >> "$diag_file" << EOF
===================================================
Network and Port Information
===================================================

=== Listening Ports ===
$(ss -tlnp | grep -E "LISTEN|nginx|node|postgres|redis" || netstat -tlnp | grep -E "LISTEN|nginx|node|postgres|redis" || echo "Could not get port information")

=== Active Connections ===
$(ss -tn state established | head -20 || echo "Could not get connection information")

=== Backend Port Connections (Port $backend_port) ===
Total connections to backend: $(ss -tn | grep ":$backend_port" | wc -l || echo "0")
$(ss -tn | grep ":$backend_port" | head -10 || echo "No connections found")

=== PostgreSQL Connections ===
EOF

    # Get PostgreSQL connection count
    if [ -n "$DB_PASS" ] && [ -n "$DB_USER" ] && [ -n "$DB_NAME" ]; then
        PGPASSWORD="$DB_PASS" psql -h "${DB_HOST:-localhost}" -U "$DB_USER" -d "$DB_NAME" -c "
            SELECT 
                count(*) as total_connections,
                count(*) FILTER (WHERE state = 'active') as active_connections,
                count(*) FILTER (WHERE state = 'idle') as idle_connections
            FROM pg_stat_activity 
            WHERE datname = '$DB_NAME';
        " >> "$diag_file" 2>&1 || echo "Could not get PostgreSQL connection stats" >> "$diag_file"
        
        echo "" >> "$diag_file"
        echo "=== PostgreSQL Connection Details ===" >> "$diag_file"
        PGPASSWORD="$DB_PASS" psql -h "${DB_HOST:-localhost}" -U "$DB_USER" -d "$DB_NAME" -c "
            SELECT 
                pid,
                usename,
                application_name,
                client_addr,
                state,
                query_start,
                state_change
            FROM pg_stat_activity 
            WHERE datname = '$DB_NAME'
            ORDER BY query_start DESC
            LIMIT 20;
        " >> "$diag_file" 2>&1 || echo "Could not get connection details" >> "$diag_file"
    else
        echo "Database credentials not available" >> "$diag_file"
    fi
    
    echo "" >> "$diag_file"
    echo "=== Redis Connections ===" >> "$diag_file"
    
    # Get Redis connection count
    if [ -n "$REDIS_USER" ] && [ -n "$REDIS_PASSWORD" ]; then
        redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" --user "$REDIS_USER" --pass "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" INFO clients >> "$diag_file" 2>&1 || echo "Could not get Redis connection info" >> "$diag_file"
    elif [ -n "$REDIS_PASSWORD" ]; then
        redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -a "$REDIS_PASSWORD" --no-auth-warning -n "${REDIS_DB:-0}" INFO clients >> "$diag_file" 2>&1 || echo "Could not get Redis connection info" >> "$diag_file"
    fi
    
    cat >> "$diag_file" << EOF

=== Firewall Status (UFW) ===
$(ufw status 2>/dev/null || echo "UFW not available")

=== Firewall Status (iptables) ===
$(iptables -L -n | head -50 2>/dev/null || echo "iptables not available")

===================================================
EOF
    
    # ========================================
    # 10. Process Information
    # ========================================
    print_info "Collecting process information..."
    
    cat >> "$diag_file" << EOF
===================================================
Process Information
===================================================

=== PatchMon Node Processes ===
$(ps aux | grep -E "node.*$instance_dir|PID" | grep -v grep || echo "No processes found")

=== Top Processes (CPU) ===
$(ps aux --sort=-%cpu | head -15)

=== Top Processes (Memory) ===
$(ps aux --sort=-%mem | head -15)

===================================================
EOF
    
    # ========================================
    # 11. SSL Certificate Information
    # ========================================
    print_info "Collecting SSL certificate information..."
    
    cat >> "$diag_file" << EOF
===================================================
SSL Certificate Information
===================================================

=== Certbot Certificates ===
$(certbot certificates 2>/dev/null || echo "Certbot not available or no certificates")

=== SSL Certificate Files ===
$(ls -lh /etc/letsencrypt/live/$instance_name/ 2>/dev/null || echo "No SSL certificates found for $instance_name")

===================================================
EOF
    
    # ========================================
    # 12. Recent System Logs
    # ========================================
    print_info "Collecting recent system logs..."
    
    journalctl -n 200 --no-pager >> "$diag_file" 2>&1 || \
        echo "Could not retrieve system logs" >> "$diag_file"
    
    # ========================================
    # 13. Installation Log (if exists)
    # ========================================
    print_info "Collecting installation log..."
    
    echo "" >> "$diag_file"
    echo "=== Installation Log (last 200 lines) ===" >> "$diag_file"
    if [ -f "$instance_dir/patchmon-install.log" ]; then
        tail -200 "$instance_dir/patchmon-install.log" >> "$diag_file" 2>&1
    else
        echo "No installation log found" >> "$diag_file"
    fi
    echo "" >> "$diag_file"
    
    # ========================================
    # 14. Node.js and npm Information
    # ========================================
    print_info "Collecting Node.js information..."
    
    cat >> "$diag_file" << EOF
===================================================
Node.js and npm Information
===================================================

=== Node.js Version ===
$(node --version 2>/dev/null || echo "Node.js not found")

=== npm Version ===
$(npm --version 2>/dev/null || echo "npm not found")

=== Backend Dependencies ===
$(cd "$instance_dir/backend" && npm list --depth=0 2>/dev/null || echo "Could not list backend dependencies")

===================================================
EOF
    
    # ========================================
    # Finalize diagnostics file
    # ========================================
    print_info "Finalizing diagnostics file..."
    
    echo "" >> "$diag_file"
    echo "====================================================" >> "$diag_file"
    echo "END OF DIAGNOSTICS REPORT" >> "$diag_file"
    echo "====================================================" >> "$diag_file"
    echo "" >> "$diag_file"
    echo "IMPORTANT: Sensitive Information" >> "$diag_file"
    echo "Passwords, secrets, and tokens have been sanitized" >> "$diag_file"
    echo "and replaced with [REDACTED]. However, please review" >> "$diag_file"
    echo "before sharing to ensure no sensitive data is included." >> "$diag_file"
    echo "====================================================" >> "$diag_file"
    
    print_status "Diagnostics file created: $diag_file"
    
    # ========================================
    # Display summary
    # ========================================
    echo ""
    echo -e "${GREEN}====================================================${NC}"
    echo -e "${GREEN}     Diagnostics Collection Complete!${NC}"
    echo -e "${GREEN}====================================================${NC}"
    echo ""
    
    # Get service statuses and file size
    local service_status=$(systemctl is-active "$instance_name" 2>/dev/null || echo "unknown")
    local nginx_status=$(systemctl is-active nginx 2>/dev/null || echo "unknown")
    local postgres_status=$(systemctl is-active postgresql 2>/dev/null || echo "unknown")
    local redis_status=$(systemctl is-active redis-server 2>/dev/null || echo "unknown")
    local file_size=$(du -h "$diag_file" 2>/dev/null | cut -f1 || echo "unknown")
    local line_count=$(wc -l < "$diag_file" 2>/dev/null || echo "unknown")
    
    # Get connection counts for summary
    local backend_port=$(grep '^PORT=' "$instance_dir/backend/.env" 2>/dev/null | cut -d'=' -f2 | tr -d ' ' || echo "3000")
    local backend_conn_count=$(ss -tn 2>/dev/null | grep ":$backend_port" | wc -l || echo "0")
    
    local db_conn_count="N/A"
    if [ -n "$DB_PASS" ] && [ -n "$DB_USER" ] && [ -n "$DB_NAME" ]; then
        db_conn_count=$(PGPASSWORD="$DB_PASS" psql -h "${DB_HOST:-localhost}" -U "$DB_USER" -d "$DB_NAME" -t -A -c "SELECT count(*) FROM pg_stat_activity WHERE datname = '$DB_NAME';" 2>/dev/null || echo "N/A")
    fi
    
    local redis_conn_count="N/A"
    if [ -n "$REDIS_USER" ] && [ -n "$REDIS_PASSWORD" ]; then
        redis_conn_count=$(redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" --user "$REDIS_USER" --pass "$REDIS_PASSWORD" --no-auth-warning INFO clients 2>/dev/null | grep "connected_clients:" | cut -d':' -f2 | tr -d '\r' || echo "N/A")
    elif [ -n "$REDIS_PASSWORD" ]; then
        redis_conn_count=$(redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" -a "$REDIS_PASSWORD" --no-auth-warning INFO clients 2>/dev/null | grep "connected_clients:" | cut -d':' -f2 | tr -d '\r' || echo "N/A")
    fi
    
    # Compact, copyable summary
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}DIAGNOSTICS SUMMARY (copy-paste friendly)${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo "Instance: $instance_name"
    echo "File: $diag_file"
    echo "Size: $file_size ($line_count lines)"
    echo "Generated: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "---"
    echo "Service Status: $service_status"
    echo "Nginx Status: $nginx_status"
    echo "PostgreSQL: $postgres_status"
    echo "Redis: $redis_status"
    echo "---"
    echo "Backend Port: $backend_port (Active Connections: $backend_conn_count)"
    echo "Database Connections: $db_conn_count"
    echo "Redis Connections: $redis_conn_count"
    echo "---"
    echo "View: cat $(basename "$diag_file")"
    echo "Or: less $(basename "$diag_file")"
    echo "Share: Send $(basename "$diag_file") to support"
    echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
    echo ""
    print_warning "Review file before sharing - sensitive data has been sanitized"
    echo ""
    
    print_success "Done!"
}

# Run main function
main "$@"

