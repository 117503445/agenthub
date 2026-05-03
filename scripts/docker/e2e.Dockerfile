# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.26-bookworm
ARG NODE_IMAGE=node:25-bookworm
ARG PNPM_VERSION=10.33.0

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS go-toolchain

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS e2e-runner
ARG PNPM_VERSION
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOCACHE=/tmp/go-build-cache
ENV CGO_ENABLED=0
WORKDIR /src

COPY --from=go-toolchain /usr/local/go /usr/local/go
RUN npm install -g "pnpm@${PNPM_VERSION}" && \
    npx --yes playwright@1.57.0 install-deps chromium && \
    npx --yes playwright@1.57.0 install chromium && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN --mount=type=cache,id=agenthub-go-mod,target=/go/pkg/mod \
    go mod download

COPY fe/package.json fe/pnpm-lock.yaml ./fe/
WORKDIR /src/fe
RUN --mount=type=cache,id=agenthub-pnpm-store,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store && \
    pnpm install --frozen-lockfile

WORKDIR /src
COPY . .
RUN git init --initial-branch=master && \
    git config user.name "agenthub e2e" && \
    git config user.email "agenthub-e2e@example.invalid" && \
    git add -A && \
    git commit -m "e2e workspace"
WORKDIR /src/fe
RUN pnpm build
WORKDIR /src
RUN --mount=type=cache,id=agenthub-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=agenthub-go-build,target=/tmp/go-build-cache \
    go run ./scripts/go-scripts build-web && \
    go build -o /usr/local/bin/agenthub-scripts ./scripts/go-scripts
RUN --mount=type=cache,id=agenthub-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=agenthub-go-build,target=/tmp/go-build-cache \
    agenthub-scripts e2e-install

ENTRYPOINT ["/src/scripts/docker/run-e2e.sh"]
