# syntax=docker/dockerfile:1
#
# Reference A/V-preprocessing custom runtime. Build from the REPOSITORY ROOT so
# the Go module (go.mod + pkg/) is in the build context:
#
#   docker build -t av-preprocessor:test \
#     -f examples/custom-runtime/av-preprocessor/Dockerfile .
#
# Unlike Omnia's distroless runtime image, the final stage carries an `ffmpeg`
# binary: PromptKit's video-to-frames stage shells out to it on PATH, so a
# runtime that performs video preprocessing MUST supply a media-capable base
# image. That is the whole point of this example's Dockerfile.

FROM --platform=$BUILDPLATFORM golang:1.26 AS builder

WORKDIR /workspace
ENV GOWORK=off

# Copy go mod files first for better layer caching.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the module source the runtime compiles against.
COPY pkg/ pkg/
COPY api/ api/
COPY internal/ internal/
COPY ee/ ee/
COPY examples/ examples/

ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -o av-preprocessor \
    ./examples/custom-runtime/av-preprocessor

# Media-capable base: ffmpeg is required by PromptKit's video-to-frames stage.
FROM debian:stable-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /
COPY --from=builder /workspace/av-preprocessor /av-preprocessor

# Run as a non-root user.
RUN useradd --uid 65532 --create-home nonroot
USER 65532:65532

# 9000 - omnia.runtime.v1 gRPC (OMNIA_GRPC_PORT)
EXPOSE 9000

ENTRYPOINT ["/av-preprocessor"]
