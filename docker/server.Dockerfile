# Development stage - run with go run, source can be volume-mounted for live reload
FROM golang:1.26-alpine AS development

RUN apk add --no-cache git ca-certificates tzdata curl nodejs npm

WORKDIR /app

# Copy agent binaries (run `make build-all-for-docker` in agent-source-code if agents-prebuilt is missing).
# Scripts are not copied: they are go:embed'ed into the server binary.
COPY --chmod=755 agents-prebuilt/patchmon-agent-* ./agents/

# Build frontend for embed
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --ignore-scripts --legacy-peer-deps 2>/dev/null || true
COPY frontend/ ./
RUN npm run build

WORKDIR /app/server
COPY server-source-code/ ./
RUN mkdir -p cmd/server/static/frontend && cp -r /app/frontend/dist cmd/server/static/frontend/

RUN go mod download

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --retries=5 \
  CMD curl -f http://localhost:${PORT:-3000}/health || exit 1

# Default: run server. Override CMD or use volume mount for live reload
ENV AGENTS_DIR=/app/agents
ENV PORT=3000
CMD ["go", "run", "./cmd/server"]

# SSG content stage — download ComplianceAsCode datastream files at build time.
# Pass --build-arg SSG_VERSION=0.1.80 to pin a specific version; otherwise
# the latest GitHub release is resolved automatically.
#
# Pinned to $BUILDPLATFORM for the same reason as frontend-builder: the payload
# is ssg-*-ds.xml datastream files, which are architecture-independent. Left
# unpinned, this stage downloaded and unpacked the same ~30s archive once per
# target platform, and did the unpacking under QEMU for the non-native one.
FROM --platform=$BUILDPLATFORM alpine:3.23 AS ssg-content
ARG SSG_VERSION=""
# Use shell variable VER to avoid Docker ARG substitution in the wget URL.
# Docker substitutes ${SSG_VERSION} at parse time; when empty, the URL would be
# .../v/scap-security-guide-.zip. VER is set once from ARG, then expanded by the shell.
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

# Production stage — hardened Alpine runtime (no -dev; no shell/apk). Use 3.23 for production.
FROM dhi.io/alpine-base:3.23

# Runtime image has no apk; ca-certificates/tzdata are in the base. No RUN needed.

WORKDIR /app

# Prebuilt by CI or by docker/build.sh, the same way agents-prebuilt/ is. The
# version, community counts, frontend and release notes are all baked in at
# compile time, so this image and the release assets are the same binary.
ARG TARGETARCH
COPY --chmod=755 server-prebuilt/patchmon-server-linux-${TARGETARCH} ./patchmon-server

# Copy SSG content (SCAP datastream files for compliance scanning)
COPY --from=ssg-content /ssg-content ./ssg-content/

# Copy agent binaries to /app/agents (in-image, read-only; no volume).
# Scripts are not copied: they are go:embed'ed into the server binary.
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
