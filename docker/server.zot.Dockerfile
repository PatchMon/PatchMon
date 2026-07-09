# Adapted from docker/server.Dockerfile for building outside CI without access
# to Docker Hardened Images (dhi.io). The two production base images are swapped
# for their public equivalents:
#   dhi.io/node:22-debian13-dev  -> node:22
#   dhi.io/alpine-base:3.23      -> alpine:3.23 (+ ca-certificates/tzdata, which
#                                   the hardened base bundled but plain alpine
#                                   does not — required for outbound HTTPS such
#                                   as the CVE->kernel NVD lookup).
# Everything else is identical to the upstream Dockerfile.

# Frontend builder stage for production
FROM node:22 AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package*.json ./

RUN echo "=== Starting npm install ===" &&\
    npm cache clean --force &&\
    rm -rf node_modules ~/.npm /root/.npm package-lock.json &&\
    echo "=== npm install ===" &&\
    npm install --ignore-scripts --legacy-peer-deps --no-audit --force &&\
    echo "=== pin react-icons (newer 5.x dropped SiSlack) ===" &&\
    npm install --ignore-scripts --legacy-peer-deps --no-audit --save-exact react-icons@5.6.0 &&\
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

# SSG content stage — download ComplianceAsCode datastream files at build time.
FROM alpine:3.23 AS ssg-content
ARG SSG_VERSION=""
RUN apk add --no-cache wget unzip jq \
    && VER="${SSG_VERSION}" \
    && if [ -z "${VER}" ]; then \
         VER=$(wget -qO- https://api.github.com/repos/ComplianceAsCode/content/releases/latest | jq -r '.tag_name' | sed 's/^v//'); \
         echo "Resolved latest SSG version from GitHub API: ${VER}"; \
       else \
         echo "Using pinned SSG version: ${VER}"; \
       fi \
    && if [ -z "${VER}" ] || [ "${VER}" = "null" ]; then \
         echo "ERROR: Could not resolve SSG version (GitHub API may be rate-limited). Pass --build-arg SSG_VERSION=x.y.z to pin." >&2; exit 1; \
       fi \
    && wget -q "https://github.com/ComplianceAsCode/content/releases/download/v${VER}/scap-security-guide-${VER}.zip" -O /tmp/ssg.zip \
    && mkdir -p /tmp/ssg-extract /ssg-content \
    && unzip -q /tmp/ssg.zip -d /tmp/ssg-extract \
    && find /tmp/ssg-extract -name 'ssg-*-ds.xml' -exec cp {} /ssg-content/ \; \
    && echo "${VER}" > /ssg-content/.ssg-version \
    && rm -rf /tmp/ssg.zip /tmp/ssg-extract

# Production stage — public Alpine runtime.
FROM alpine:3.23

# The hardened base shipped ca-certificates/tzdata; plain alpine does not.
# ca-certificates is required for the server's outbound HTTPS (CVE->kernel NVD
# lookup); tzdata for timezone handling.
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary (migrations and frontend are embedded in the binary)
COPY --from=builder /app/patchmon-server ./

# Copy SSG content (SCAP datastream files for compliance scanning)
COPY --from=ssg-content /ssg-content ./ssg-content/

# Copy agent scripts and binaries to /app/agents (in-image, read-only; no volume)
COPY agents ./agents/
COPY --chmod=755 agents-prebuilt/patchmon-agent-* ./agents/

# Entrypoint starts server (no volume copy; agents served from image)
COPY --chmod=755 docker/backend.docker-entrypoint.sh ./entrypoint.sh

ENV PORT=3000
ENV AGENTS_DIR=/app/agents
ENV SSG_CONTENT_DIR=/app/ssg-content
# Cap Go heap to reduce RAM (override at runtime if needed, e.g. GOMEMLIMIT=128MiB)
ENV GOMEMLIMIT=256MiB

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
  CMD wget -q -O /dev/null http://localhost:${PORT:-3000}/health || exit 1

ENTRYPOINT ["./entrypoint.sh"]
