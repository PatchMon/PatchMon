# Development target
FROM node:lts-alpine AS development

ENV NODE_ENV=development \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    ENABLE_LOGGING=true \
    LOG_LEVEL=info \
    PM_LOG_TO_CONSOLE=true \
    PORT=3001

RUN apk add --no-cache openssl tini curl libc6-compat

USER node

WORKDIR /app

COPY --chown=node:node package*.json ./
COPY --chown=node:node backend/ ./backend/
COPY --chown=node:node agents ./agents_backup
COPY --chown=node:node agents ./agents
COPY --chmod=755 docker/backend.docker-entrypoint.sh ./entrypoint.sh

RUN npm install --workspace=backend --ignore-scripts && cd backend && npx prisma generate

EXPOSE 3001

VOLUME [ "/app/agents" ]

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
  CMD curl -f http://localhost:3001/health || exit 1

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/entrypoint.sh"]

# Builder stage for production
# Use Debian-based Node for better QEMU ARM64 compatibility
FROM node:lts-slim AS builder

# Install OpenSSL for Prisma
RUN apt-get update -y && apt-get install -y openssl && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --chown=node:node package*.json ./
COPY --chown=node:node backend/ ./backend/

RUN npm cache clean --force &&\
    rm -rf node_modules ~/.npm /root/.npm &&\
    npm install --workspace=backend --ignore-scripts --legacy-peer-deps --no-audit --prefer-online --fetch-retries=3 --fetch-retry-mintimeout=20000 --fetch-retry-maxtimeout=120000 &&\
    cd backend && npx prisma generate &&\
    cd .. && npm prune --omit=dev --workspace=backend &&\
    npm cache clean --force

# Production stage
FROM node:lts-alpine

ENV NODE_ENV=production \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    ENABLE_LOGGING=true \
    LOG_LEVEL=info \
    PM_LOG_TO_CONSOLE=true \
    PORT=3001 \
    JWT_EXPIRES_IN=1h \
    JWT_REFRESH_EXPIRES_IN=7d \
    SESSION_INACTIVITY_TIMEOUT_MINUTES=30

RUN apk add --no-cache openssl tini curl libc6-compat

USER node

WORKDIR /app

COPY --from=builder --chown=node:node /app/backend ./backend
COPY --from=builder --chown=node:node /app/node_modules ./node_modules
COPY --chown=node:node agents ./agents_backup
COPY --chown=node:node agents ./agents
COPY --chmod=755 docker/backend.docker-entrypoint.sh ./entrypoint.sh

WORKDIR /app/backend

EXPOSE 3001

VOLUME [ "/app/agents" ]

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
  CMD curl -f http://localhost:3001/health || exit 1

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/entrypoint.sh"]
