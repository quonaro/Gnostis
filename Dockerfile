# syntax=docker/dockerfile:1

FROM golang:1.23-bookworm AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
        gcc \
        libc6-dev \
        git \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN GOTOOLCHAIN=auto go mod download

COPY . .

ARG VERSION=dev
RUN GOTOOLCHAIN=auto CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /src/gnostis \
    ./cmd/gnostis

FROM debian:bookworm-slim

RUN groupadd -r gnostis && useradd -r -g gnostis -m -d /app gnostis

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
RUN mkdir -p /app/data /projects \
    && chown gnostis:gnostis /app/data \
    && chmod 755 /projects

COPY --from=builder /src/gnostis /app/gnostis

USER gnostis

ENTRYPOINT ["/app/gnostis"]
CMD ["run"]
