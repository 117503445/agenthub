ARG PNPM_VERSION=10.33.0

FROM node:25-alpine AS fe
ARG PNPM_VERSION
WORKDIR /src/fe
RUN npm install -g "pnpm@${PNPM_VERSION}"
COPY fe/package.json fe/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY fe/ ./
RUN pnpm build

FROM golang:1.26-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=fe /src/fe/dist ./cmd/web/dist
RUN go build -o /out/web ./cmd/web

FROM alpine:3.23
WORKDIR /app
COPY --from=go-build /out/web /app/web
ENV AGENTHUB_PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/web"]
