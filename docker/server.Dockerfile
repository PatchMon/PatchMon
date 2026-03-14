# Development stage - run with go run, source can be volume-mounted for live reload
FROM golang:1.26-alpine AS development

RUN apk add --no-cache git ca-certificates tzdata curl node npm

WORKDIR /app

# Copy agent scripts and binaries (same layout as production; run `make build-all-for-docker` in agent-source-code if agents-prebuilt is missing)
COPY agents ./agents/
COPY --chmod=755 agents-prebuilt/patchmon-agent-* ./agents/

# Build frontend for embed
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --ignore-scripts --legacy-peer-deps 2>/dev/null || true
COPY frontend/ ./
RUN npm run build 2>/dev/null || mkdir -p dist && echo '<!DOCTYPE html><html><body>Build frontend first</body></html>' > dist/index.html

WORKDIR /app/server
COPY server-source-code/ ./
RUN mkdir -p cmd/server/static/frontend && cp -r /app/frontend/dist cmd/server/static/frontend/

RUN go mod download

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --retries=5 \
  CMD curl -f http://localhost:3000/health || exit 1

# Default: run server. Override CMD or use volume mount for live reload
ENV AGENTS_DIR=/app/agents
ENV PORT=3000
CMD ["go", "run", "./cmd/server"]

# Frontend builder stage for production
FROM dhi.io/node:22-debian13-dev AS frontend-builder

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

# Build stage - server (runs on amd64, cross-compiles for target platform)
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy server source
COPY server-source-code/ ./server/
# Copy built frontend into embed directory
COPY --from=frontend-builder /app/frontend/dist ./server/cmd/server/static/frontend/dist

WORKDIR /app/server

ARG TARGETOS
ARG TARGETARCH
RUN go mod download && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -buildvcs=false -ldflags="-s -w" -o /app/patchmon-server ./cmd/server

# Production stage — hardened Alpine runtime (no -dev; no shell/apk). Use 3.23 for production.
FROM dhi.io/alpine-base:3.23

# Runtime image has no apk; ca-certificates/tzdata are in the base. No RUN needed.

WORKDIR /app

# Copy binary (migrations and frontend are embedded in the binary)
COPY --from=builder /app/patchmon-server ./

# Copy agent scripts and binaries to /app/agents (in-image, read-only; no volume)
COPY agents ./agents/
COPY --chmod=755 agents-prebuilt/patchmon-agent-* ./agents/

# Entrypoint starts server (no volume copy; agents served from image)
COPY --chmod=755 docker/backend.docker-entrypoint.sh ./entrypoint.sh

ENV PORT=3000
ENV AGENTS_DIR=/app/agents
# Cap Go heap to reduce RAM (override at runtime if needed, e.g. GOMEMLIMIT=128MiB)
ENV GOMEMLIMIT=256MiB

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
  CMD wget -q -O /dev/null http://localhost:3000/health || exit 1

ENTRYPOINT ["./entrypoint.sh"]
