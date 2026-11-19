# syntax=docker/dockerfile:1

ARG GO_IMAGE=golang:1.26.10-alpine3.24
ARG NODE_IMAGE=node:22-alpine3.24
ARG VERSION=0.0.0
# CI can reuse container-built release artifacts; local builds use source.
ARG SERVER_SOURCE=server-builder
ARG AGENT_SOURCE=agent-builder

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS frontend-builder
WORKDIR /src
COPY package.json package-lock.json ./
COPY frontend/package.json ./frontend/package.json
RUN --mount=type=cache,target=/root/.npm \
    npm ci --workspace=frontend --include=dev --ignore-scripts
COPY frontend/ ./frontend/
RUN npm run build --workspace=frontend

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS go-builder
RUN apk add --no-cache git ca-certificates
ENV CGO_ENABLED=0

# All server image architectures bundle the same ten cross-compiled agents.
FROM go-builder AS agent-builder
WORKDIR /src/agent
COPY agent-source-code/go.mod agent-source-code/go.sum ./
RUN go mod download
COPY agent-source-code/ ./
ARG VERSION
# Preserve linker metadata used to inspect agent versions across platforms.
RUN --mount=type=cache,id=patchmon-agent-go-build,target=/root/.cache/go-build \
    set -eu; \
    mkdir -p /out/agents; \
    for target in linux/amd64 linux/arm64 linux/386 linux/arm \
                  freebsd/amd64 freebsd/arm64 freebsd/386 freebsd/arm \
                  windows/amd64 windows/arm64; do \
      target_os="${target%/*}"; target_arch="${target#*/}"; suffix=""; \
      if [ "$target_os" = windows ]; then suffix=".exe"; fi; \
      GOOS="$target_os" GOARCH="$target_arch" GOARM=6 \
        go build -buildvcs=false \
          -ldflags="-s -w -X patchmon-agent/internal/pkgversion.Version=${VERSION}" \
          -o "/out/agents/patchmon-agent-${target_os}-${target_arch}${suffix}" ./cmd/patchmon-agent; \
    done

# Export targets let CI publish these same binaries as release assets.
FROM scratch AS agent-binaries
COPY --from=agent-builder /out/agents/ /

FROM go-builder AS server-source
WORKDIR /src/server
COPY server-source-code/go.mod server-source-code/go.sum ./
RUN go mod download
COPY server-source-code/ ./
COPY --from=frontend-builder /src/frontend/dist ./cmd/server/static/frontend/dist
COPY --chmod=755 docker/build-server.sh /usr/local/bin/build-patchmon-server
ARG VERSION
ARG GITHUB_STARS=""
ARG DISCORD_MEMBERS=""
ARG YOUTUBE_SUBSCRIBERS=""
ARG LINKEDIN_FOLLOWERS=""

# Compile natively for the requested image platform without compiler emulation.
FROM server-source AS server-builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN --mount=type=cache,id=patchmon-server-go-build,target=/root/.cache/go-build \
    test "$TARGETOS" = linux \
    && build-patchmon-server /out "$TARGETARCH"

FROM server-source AS server-binaries-builder
ARG SERVER_ARCHES="amd64 arm64"
RUN --mount=type=cache,id=patchmon-server-go-build,target=/root/.cache/go-build \
    build-patchmon-server /out "$SERVER_ARCHES"

FROM scratch AS server-binaries
COPY --from=server-binaries-builder /out/ /

# Optional CI inputs: BuildKit skips these stages for normal source builds.
FROM scratch AS agents-prebuilt
COPY --chmod=755 agents-prebuilt/patchmon-agent-* /out/agents/

FROM scratch AS server-prebuilt
ARG TARGETARCH
COPY --chmod=755 server-prebuilt/patchmon-server-linux-${TARGETARCH} /out/patchmon-server-linux-${TARGETARCH}

FROM ${AGENT_SOURCE} AS agent-artifacts
FROM ${SERVER_SOURCE} AS server-artifacts

# Development retains Go and source on the requested runtime platform.
FROM ${GO_IMAGE} AS development
RUN apk add --no-cache git ca-certificates tzdata curl
WORKDIR /app/server
COPY server-source-code/go.mod server-source-code/go.sum ./
RUN go mod download
COPY server-source-code/ ./
COPY --from=frontend-builder /src/frontend/dist ./cmd/server/static/frontend/dist
COPY --from=agent-artifacts /out/agents/ /app/agents/
ENV PORT=3000
ENV AGENTS_DIR=/app/agents
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=5s --start-period=60s --retries=5 \
    CMD curl -f http://localhost:${PORT:-3000}/health || exit 1
CMD ["go", "run", "./cmd/server"]

# SCAP datastreams are architecture-independent and downloaded only once.
FROM --platform=$BUILDPLATFORM alpine:3.24 AS ssg-content
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
         echo "ERROR: Could not resolve SSG version. Pass --build-arg SSG_VERSION=x.y.z to pin." >&2; exit 1; \
       fi \
    && wget -q "https://github.com/ComplianceAsCode/content/releases/download/v${VER}/scap-security-guide-${VER}.zip" -O /tmp/ssg.zip \
    && mkdir -p /tmp/ssg-extract /ssg-content \
    && unzip -q /tmp/ssg.zip -d /tmp/ssg-extract \
    && find /tmp/ssg-extract -name 'ssg-*-ds.xml' -exec cp {} /ssg-content/ \; \
    && echo "${VER}" > /ssg-content/.ssg-version \
    && rm -rf /tmp/ssg.zip /tmp/ssg-extract

# Runtime has the compiled payload and the shell/wget used by startup/health.
FROM alpine:3.24 AS production
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ARG TARGETARCH
COPY --from=server-artifacts --chmod=755 /out/patchmon-server-linux-${TARGETARCH} ./patchmon-server
COPY --from=agent-artifacts --chmod=755 /out/agents/ ./agents/
COPY --from=ssg-content /ssg-content ./ssg-content/
COPY --chmod=755 docker/backend.docker-entrypoint.sh ./entrypoint.sh
ENV PORT=3000
ENV AGENTS_DIR=/app/agents
ENV SSG_CONTENT_DIR=/app/ssg-content
ENV GOMEMLIMIT=256MiB
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 \
    CMD wget -q -O /dev/null http://localhost:${PORT:-3000}/health || exit 1
ENTRYPOINT ["./entrypoint.sh"]
