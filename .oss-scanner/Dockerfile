# Build image for Anthropic's OSS Scanner.
#
# The scanner builds this file with the repository as the context, then removes
# network access. Fetch every dependency here. The checkout must remain at /src.
#
# The repository .dockerignore excludes .git from this build. The scanner may
# restore that history afterwards.
# Database tests skip unless PATCHMON_TEST_DATABASE_URL is set. There is no
# Postgres or Redis in this image. See .oss-scanner/threat_model.md.

FROM node:22-bookworm AS nodejs

FROM golang:1.26-bookworm

ENV DEBIAN_FRONTEND=noninteractive \
    GOTOOLCHAIN=auto \
    GOPROXY=https://proxy.golang.org,direct \
    CGO_ENABLED=0

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates git build-essential python3 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=nodejs /usr/local/bin/node /usr/local/bin/node
COPY --from=nodejs /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && node --version && npm --version

# scanner contract: the checkout lives inside the image, at /src
COPY . /src
WORKDIR /src

# Same frontend build as CI. The server embeds this tree, and it is not in git.
# npm ci also installs the frontend workspace from the root lockfile.
RUN npm ci \
    && npm run build --workspace=frontend \
    && mkdir -p server-source-code/cmd/server/static/frontend \
    && cp -a frontend/dist server-source-code/cmd/server/static/frontend/dist

# go.mod asks for a toolchain patch that the base image may not ship yet.
# Download it, and the modules, while the network is still available.
RUN cd server-source-code && go mod download \
    && cd /src/agent-source-code && go mod download \
    && go version

# Symbols stay in. Production builds strip them. These are for auditing.
RUN mkdir -p /src/build \
    && cd server-source-code \
    && go build -buildvcs=false -gcflags=all="-N -l" -o /src/build/patchmon-server ./cmd/server \
    && go build -buildvcs=false -gcflags=all="-N -l" -o /src/build/migrate ./cmd/migrate \
    && cd /src/agent-source-code \
    && go build -buildvcs=false -gcflags=all="-N -l" -o /src/build/patchmon-agent ./cmd/patchmon-agent

# A failing test must not fail the image build. Database tests skip on their own.
RUN (cd /src/server-source-code && (go test ./... -count=1 -timeout 15m || echo "WARNING: server tests reported failures")) \
    && (cd /src/agent-source-code && (go test ./... -count=1 -timeout 15m || echo "WARNING: agent tests reported failures")) \
    && (cd /src && (npm run test:run --workspace=frontend || echo "WARNING: frontend tests reported failures"))
