# syntax=docker/dockerfile:1
# Dev-only image for the memory-API demo seeder (demos/memory-seeder). Built and
# run by the omnia-demos memory-demo seed Job. NOT a product release artifact.
#
# Unlike the product binaries (distroless), this uses an alpine final stage so
# the seed Job can inject the runtime-resolved workspace/agent UIDs as flags via
# a small `sh -c` wrapper (the UIDs aren't known until the operator stamps them).
FROM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH
ENV GOWORK=off

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -ldflags="-w -s" -o memory-seeder ./demos/memory-seeder

FROM alpine:3.21
RUN adduser -D -u 65532 nonroot
COPY --from=builder /workspace/memory-seeder /usr/local/bin/memory-seeder
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/memory-seeder"]
