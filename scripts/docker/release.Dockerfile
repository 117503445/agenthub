# syntax=docker/dockerfile:1.7

ARG NODE_IMAGE=node:25-alpine
ARG GO_IMAGE=golang:1.26-alpine
ARG ALPINE_IMAGE=alpine:3.23
ARG PNPM_VERSION=10.33.0

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS fe-build
ARG PNPM_VERSION
WORKDIR /src/fe
RUN npm install -g "pnpm@${PNPM_VERSION}"
COPY fe/package.json fe/pnpm-lock.yaml ./
RUN --mount=type=cache,id=agenthub-pnpm-store,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store && \
    pnpm install --frozen-lockfile
COPY fe/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS go-src
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN --mount=type=cache,id=agenthub-go-mod,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=fe-build /src/fe/dist ./cmd/web/dist

FROM go-src AS binary-build
ARG BUILD_TIME=
ARG BUILD_DIR=/src
ARG GIT_BRANCH=
ARG GIT_COMMIT=
ARG GIT_DIRTY=false
ARG GIT_TAG=
ARG GIT_VERSION=dev
ARG BUILT_PACKAGE=github.com/117503445/agenthub/internal/buildinfo
RUN --mount=type=cache,id=agenthub-go-mod,target=/go/pkg/mod \
    --mount=type=cache,id=agenthub-go-build,target=/root/.cache/go-build \
    set -eux; \
    ldflags="-s -w \
      -X ${BUILT_PACKAGE}.BuildTime=${BUILD_TIME} \
      -X ${BUILT_PACKAGE}.BuildDir=${BUILD_DIR} \
      -X ${BUILT_PACKAGE}.GitBranch=${GIT_BRANCH} \
      -X ${BUILT_PACKAGE}.GitCommit=${GIT_COMMIT} \
      -X ${BUILT_PACKAGE}.GitDirty=${GIT_DIRTY} \
      -X ${BUILT_PACKAGE}.GitTag=${GIT_TAG} \
      -X ${BUILT_PACKAGE}.GitVersion=${GIT_VERSION}"; \
    mkdir -p /out; \
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
      os="${target%/*}"; \
      arch="${target#*/}"; \
      GOOS="${os}" GOARCH="${arch}" go build -trimpath -ldflags "${ldflags}" -o "/out/agenthub-${os}-${arch}" ./cmd/web; \
    done; \
    cd /out; \
    sha256sum agenthub-* > SHA256SUMS

FROM scratch AS artifacts
COPY --from=binary-build /out/ /

FROM ${ALPINE_IMAGE} AS runtime
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY --from=binary-build /out/agenthub-${TARGETOS}-${TARGETARCH} /app/agenthub
ENV AGENTHUB_PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/agenthub"]
