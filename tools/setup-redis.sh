#!/bin/bash

# redis-setup.sh - Redis Database and User Setup for PatchMon
# This script creates a dedicated Redis database and user for a PatchMon instance

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default Redis connection details
REDIS_HOST=${REDIS_HOST:-"localhost"}
REDIS_PORT=${REDIS_PORT:-6379}

# Validate REDIS_ADMIN_PASSWORD is set (no insecure defaults)
if [ -z "$REDIS_ADMIN_PASSWORD" ]; then
    echo -e "${RED}❌ ERROR: REDIS_ADMIN_PASSWORD environment variable is required${NC}"
    echo "Please set REDIS_ADMIN_PASSWORD before running this script:"
    echo "  export REDIS_ADMIN_PASSWORD='your-secure-password'"
    exit 1
fi

echo -e "${BLUE}🔧 PatchMon Redis Setup${NC}"
echo "=================================="

# Function to generate random strings
generate_random_string() {
    local length=${1:-16}
    openssl rand -base64 $length | tr -d "=+/" | cut -c1-$length
}

# Function to check if Redis is accessible
check_redis_connection() {
    echo -e "${YELLOW}📡 Checking Redis connection...${NC}"
    
    if [ -n "$REDIS_ADMIN_PASSWORD" ]; then
        # With password - use ACL admin user
        if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user admin --pass "$REDIS_ADMIN_PASSWORD" --no-auth-warning ping > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Redis connection successful${NC}"
            return 0
        else
            echo -e "${RED}❌ Cannot connect to Redis with ACL admin user${NC}"
            echo "Please ensure Redis is running and the admin password is correct"
            return 1
        fi
    else
        # Without password
        if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping > /dev/null 2>&1; then
            echo -e "${GREEN}✅ Redis connection successful${NC}"
            return 0
        else
            echo -e "${RED}❌ Cannot connect to Redis${NC}"
            echo "Please ensure Redis is running"
            return 1
        fi
    fi
}

# Function to find next available database number
find_next_db() {
    echo -e "${YELLOW}🔍 Finding next available database...${NC}" >&2
    
    # Start from database 0 and keep checking until we find an empty one
    local db_num=0
    local max_attempts=100  # Safety limit to prevent infinite loop
    
    while [ $db_num -lt $max_attempts ]; do
        # Test if database is empty
        local key_count
        local redis_output
        
        if [ -n "$REDIS_ADMIN_PASSWORD" ]; then
            # With password - use ACL admin user
            redis_output=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user admin --pass "$REDIS_ADMIN_PASSWORD" --no-auth-warning -n "$db_num" DBSIZE 2>&1)
        else
            # Without password
            redis_output=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$db_num" DBSIZE 2>&1)
        fi
        
        # Check for authentication errors
        if echo "$redis_output" | grep -q "NOAUTH"; then
            echo -e "${RED}❌ Authentication required but REDIS_ADMIN_PASSWORD not set${NC}" >&2
            echo -e "${YELLOW}💡 Please set REDIS_ADMIN_PASSWORD environment variable:${NC}" >&2
            echo -e "${YELLOW}   export REDIS_ADMIN_PASSWORD='your_password'${NC}" >&2
            echo -e "${YELLOW}   Or run: REDIS_ADMIN_PASSWORD='your_password' ./setup-redis.sh${NC}" >&2
            exit 1
        fi
        
        # Check for other errors
        if echo "$redis_output" | grep -q "ERR"; then
            if echo "$redis_output" | grep -q "invalid DB index"; then
                echo -e "${RED}❌ Reached maximum database limit at database $db_num${NC}" >&2
                echo -e "${YELLOW}💡 Redis is configured with $db_num databases maximum.${NC}" >&2
                echo -e "${YELLOW}💡 Increase 'databases' setting in redis.conf or clean up unused databases.${NC}" >&2
                exit 1
            else
                echo -e "${RED}❌ Error checking database $db_num: $redis_output${NC}" >&2
                exit 1
            fi
        fi
        
        key_count="$redis_output"
        
        # If database is empty, use it
        if [ "$key_count" = "0" ]; then
            echo -e "${GREEN}✅ Found available database: $db_num (empty)${NC}" >&2
            echo "$db_num"
            return
        fi
        
        echo -e "${BLUE}   Database $db_num has $key_count keys, checking next...${NC}" >&2
        db_num=$((db_num + 1))
    done
    
    echo -e "${RED}❌ No available databases found (checked 0-$max_attempts)${NC}" >&2
    echo -e "${YELLOW}💡 All checked databases are in use. Consider cleaning up unused databases.${NC}" >&2
    exit 1
}

# Function to create Redis user
create_redis_user() {
    local username="$1"
    local password="$2"
    local db_num="$3"
    
    echo -e "${YELLOW}👤 Creating Redis user: $username for database $db_num${NC}"
    
    # Create user with password and permissions
    # Note: >password syntax is for Redis ACL, we need to properly escape it
    local result
    if [ -n "$REDIS_ADMIN_PASSWORD" ]; then
        # With password - use ACL admin user
        result=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user admin --pass "$REDIS_ADMIN_PASSWORD" --no-auth-warning ACL SETUSER "$username" on ">${password}" ~* +@all 2>&1)
    else
        # Without password
        result=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ACL SETUSER "$username" on ">${password}" ~* +@all 2>&1)
    fi
    
    if [ $? -eq 0 ] && [ "$result" = "OK" ]; then
        echo -e "${GREEN}✅ Redis user '$username' created successfully for database $db_num${NC}"
        
        # Save ACL users to file to persist across restarts
        local save_result
        if [ -n "$REDIS_ADMIN_PASSWORD" ]; then
            save_result=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user admin --pass "$REDIS_ADMIN_PASSWORD" --no-auth-warning ACL SAVE 2>&1)
        else
            save_result=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ACL SAVE 2>&1)
        fi
        
        if [ "$save_result" = "OK" ]; then
            echo -e "${GREEN}✅ Redis ACL users saved to file${NC}"
        else
            echo -e "${YELLOW}⚠️  Failed to save ACL users to file: $save_result${NC}"
        fi
        
        # Verify user was created
        if [ -n "$REDIS_ADMIN_PASSWORD" ]; then
            local verify=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user admin --pass "$REDIS_ADMIN_PASSWORD" --no-auth-warning ACL GETUSER "$username" 2>&1)
        else
            local verify=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ACL GETUSER "$username" 2>&1)
        fi
        
        if [ "$verify" = "(nil)" ]; then
            echo -e "${RED}❌ User creation reported OK but user does not exist${NC}"
            return 1
        fi
        
        return 0
    else
        echo -e "${RED}❌ Failed to create Redis user${NC}"
        echo -e "${RED}Error: $result${NC}"
        return 1
    fi
}

# Function to test user connection
test_user_connection() {
    local username="$1"
    local password="$2"
    local db_num="$3"
    
    echo -e "${YELLOW}🧪 Testing user connection...${NC}"
    
    if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user "$username" --pass "$password" --no-auth-warning -n "$db_num" ping > /dev/null 2>&1; then
        echo -e "${GREEN}✅ User connection test successful${NC}"
        return 0
    else
        echo -e "${RED}❌ User connection test failed${NC}"
        return 1
    fi
}

# Function to mark database as in-use
mark_database_in_use() {
    local db_num="$1"
    
    echo -e "${YELLOW}📝 Marking database as in-use...${NC}"
    
    if [ -n "$REDIS_ADMIN_PASSWORD" ]; then
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" --user admin --pass "$REDIS_ADMIN_PASSWORD" --no-auth-warning -n "$db_num" SET "patchmon:initialized" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > /dev/null
    else
        redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$db_num" SET "patchmon:initialized" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > /dev/null
    fi
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Database marked as in-use${NC}"
        return 0
    else
        echo -e "${RED}❌ Failed to mark database${NC}"
        return 1
    fi
}

# Main execution
main() {
    # Check Redis connection
    if ! check_redis_connection; then
        exit 1
    fi
    
    # Generate random credentials
    USERNAME="patchmon_$(generate_random_string 8)"
    PASSWORD=$(generate_random_string 32)
    DB_NUM=$(find_next_db)
    
    echo ""
    echo -e "${BLUE}📋 Generated Configuration:${NC}"
    echo "Username: $USERNAME"
    echo "Password: $PASSWORD"
    echo "Database: $DB_NUM"
    echo ""
    
    # Create Redis user
    if ! create_redis_user "$USERNAME" "$PASSWORD" "$DB_NUM"; then
        exit 1
    fi
    
    # Test user connection
    if ! test_user_connection "$USERNAME" "$PASSWORD" "$DB_NUM"; then
        exit 1
    fi
    
    # Mark database as in-use to prevent reuse on next run
    if ! mark_database_in_use "$DB_NUM"; then
        exit 1
    fi
    
    # Output .env configuration
    echo ""
    echo -e "${GREEN}🎉 Redis setup completed successfully!${NC}"
    echo ""
    echo -e "${BLUE}📄 Add these lines to your .env file:${NC}"
    echo "=================================="
    echo "REDIS_HOST=$REDIS_HOST"
    echo "REDIS_PORT=$REDIS_PORT"
    echo "REDIS_USER=$USERNAME"
    echo "REDIS_PASSWORD=$PASSWORD"
    echo "REDIS_DB=$DB_NUM"
    echo "=================================="
    echo ""
    
    echo -e "${YELLOW}💡 Copy the configuration above to your .env file${NC}"
}

# Run main function
main "$@"
