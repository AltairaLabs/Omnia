# syntax=docker/dockerfile:1
#
# Reference custom facade image (#1773). Build from the REPOSITORY ROOT so the
# Go module (go.mod + pkg/) is in the build context:
#
#   docker build -t reference-custom-facade:test \
#     -f examples/custom-facade/Dockerfile .
#
# The reference facade only depends on the PUBLIC contract packages
# (pkg/policy, pkg/runtime/v1) plus the published gRPC / JWT libraries — it does
# not import the built-in facade's internal auth or session packages, so it is a
# faithful starting point for a third-party bring-your-own facade.

FROM --platform=$BUILDPLATFORM golang:1.26 AS builder

WORKDIR /workspace
ENV GOWORK=off

# Copy go mod files first for better layer caching.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the module source the facade compiles against.
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
    -o custom-facade \
    ./examples/custom-facade

# Distroless/static provides ca-certificates, tzdata, /tmp and the nonroot user.
FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY --from=builder /workspace/custom-facade .

USER 65532:65532

# 8080  - external data-plane (POST /chat)
# 8081  - health endpoints (/healthz, /readyz) probed by the operator
# 18080 - management-plane twin (enabled when OMNIA_MGMT_PLANE_JWKS_URL is set)
EXPOSE 8080 8081 18080

ENTRYPOINT ["/custom-facade"]
