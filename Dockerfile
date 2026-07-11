# syntax=docker/dockerfile:1

FROM golang:1.26-bookworm AS builder

WORKDIR /src

# Native amd64 toolchain is already present in the golang image.
# Install only the ARM64 cross-compiler so we can build linux/arm64
# without QEMU emulation.
RUN apt-get update && apt-get install -y --no-install-recommends \
        gcc-aarch64-linux-gnu \
        libc6-dev-arm64-cross \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$TARGETARCH" = "arm64" ]; then \
      export CC=aarch64-linux-gnu-gcc; \
      export CXX=aarch64-linux-gnu-g++; \
    fi && \
    CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH go build \
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
