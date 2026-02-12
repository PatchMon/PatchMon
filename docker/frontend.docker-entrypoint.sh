#!/bin/sh
# Initialize branding assets volume with built files from the image.
# This runs as part of nginx-unprivileged's entrypoint.d chain (before template processing).
# This script runs as root (because we don't set USER 101 in Dockerfile) allowing it to
# fix permissions on the shared volume. Nginx itself still runs as user 101.
#
# Strategy: Copy ALL assets (including new JS/CSS from upgrades), but preserve user branding.

if [ -d "/usr/share/nginx/html/assets_backup" ]; then
    # Ensure destination directory exists
    mkdir -p /usr/share/nginx/html/assets/ 2>/dev/null || true
    
    # Backup user-uploaded branding files (if they exist) to temp location
    TEMP_DIR="/tmp/user-branding-$$"
    mkdir -p "$TEMP_DIR"
    
    # Preserve custom logos and favicon (common branding files)
    for file in logo_dark.png logo_dark.svg logo_dark.jpg \
                logo_light.png logo_light.svg logo_light.jpg \
                favicon.svg favicon.ico; do
        if [ -f "/usr/share/nginx/html/assets/$file" ]; then
            # Check if file is different from default (user uploaded a custom one)
            if [ -f "/usr/share/nginx/html/assets_backup/$file" ]; then
                if ! cmp -s "/usr/share/nginx/html/assets/$file" "/usr/share/nginx/html/assets_backup/$file"; then
                    cp -a "/usr/share/nginx/html/assets/$file" "$TEMP_DIR/" 2>/dev/null || true
                fi
            else
                # File doesn't exist in backup, so it's user-uploaded
                cp -a "/usr/share/nginx/html/assets/$file" "$TEMP_DIR/" 2>/dev/null || true
            fi
        fi
    done
    
    # Copy all assets from backup (overwrites everything, gets new JS/CSS)
    cp -a /usr/share/nginx/html/assets_backup/* /usr/share/nginx/html/assets/ 2>/dev/null || true
    
    # Restore user-uploaded branding files
    if [ -d "$TEMP_DIR" ] && [ "$(ls -A $TEMP_DIR 2>/dev/null)" ]; then
        cp -a "$TEMP_DIR"/* /usr/share/nginx/html/assets/ 2>/dev/null || true
    fi
    
    # Cleanup temp directory
    rm -rf "$TEMP_DIR" 2>/dev/null || true
    
    # Set ownership to backend user (1000:1000) and permissions to 1777 (world-writable + sticky)
    # This allows both backend (UID 1000) to write logos and frontend (UID 101) to read/serve them
    chown -R 1000:1000 /usr/share/nginx/html/assets/ 2>/dev/null || true
    chmod -R 1777 /usr/share/nginx/html/assets/ 2>/dev/null || true
fi

# Don't exec - let the entrypoint chain continue to process templates and start nginx
