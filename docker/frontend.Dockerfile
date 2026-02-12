# Development target
FROM node:lts-alpine AS development

WORKDIR /app

COPY package*.json ./
COPY frontend/ ./frontend/

RUN npm install --workspace=frontend --ignore-scripts

WORKDIR /app/frontend

EXPOSE 3000

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "3000"]

# Builder stage for production
# Use Debian-based Node for better QEMU ARM64 compatibility
FROM node:lts-slim AS builder

WORKDIR /app/frontend

COPY frontend/package*.json ./

RUN echo "=== Starting npm install ===" &&\
    npm cache clean --force &&\
    rm -rf node_modules ~/.npm /root/.npm package-lock.json &&\
    echo "=== npm install ===" &&\
    npm install --ignore-scripts --legacy-peer-deps --no-audit --force &&\
    echo "=== npm install completed ===" &&\
    npm cache clean --force

COPY frontend/ ./

RUN npm run build

# Production stage
FROM nginxinc/nginx-unprivileged:alpine

ENV BACKEND_HOST=backend \
    BACKEND_PORT=3001

COPY --from=builder /app/frontend/dist /usr/share/nginx/html

# Preserve built assets in backup so the branding_assets volume can be initialized on first run
USER root
RUN cp -a /usr/share/nginx/html/assets /usr/share/nginx/html/assets_backup && \
    chown -R 101:101 /usr/share/nginx/html/assets_backup

COPY docker/nginx.conf.template /etc/nginx/templates/default.conf.template
# Add init script to nginx-unprivileged's entrypoint.d (runs before template processing)
# Script runs as root (we don't switch to USER 101) so it can fix volume permissions
COPY --chmod=755 docker/frontend.docker-entrypoint.sh /docker-entrypoint.d/10-init-assets.sh

EXPOSE 3000

# Note: We stay as root here, but nginx-unprivileged's entrypoint will run nginx as user 101
CMD ["nginx", "-g", "daemon off;"]
