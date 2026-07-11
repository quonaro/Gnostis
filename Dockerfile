# syntax=docker/dockerfile:1

# Download Go modules once on the build host architecture and share the module
# cache with every target platform.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

FROM golang:1.26-bookworm AS builder

WORKDIR /src

ARG TARGETARCH
# The native amd64 toolchain is already present in the golang image. Install
# the ARM64 cross-compiler when the target architecture is arm64.
RUN if [ "$TARGETARCH" = "arm64" ]; then \
      apt-get update && apt-get install -y --no-install-recommends \
          gcc-aarch64-linux-gnu \
          libc6-dev-arm64-cross \
      && rm -rf /var/lib/apt/lists/*; \
    fi

COPY . .

ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
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
