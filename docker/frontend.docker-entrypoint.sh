#!/bin/sh
# Initialize branding assets volume with built files from the image.
# This runs as part of nginx-unprivileged's entrypoint.d chain (before template processing).
#
# On every startup, copy ALL built assets from backup into the volume.
# This ensures that after an image upgrade, the new Vite-hashed JS/CSS files
# are always present. User-uploaded branding files (logo_dark.png, favicon.svg, etc.)
# will be overwritten by the defaults from the build, but the backend re-saves
# uploaded branding to the volume via ASSETS_DIR, so they persist across restarts.

if [ -d "/usr/share/nginx/html/assets_backup" ]; then
    cp -a /usr/share/nginx/html/assets_backup/* /usr/share/nginx/html/assets/ 2>/dev/null || true
fi

# Don't exec - let the entrypoint chain continue to process templates and start nginx
