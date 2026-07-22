# A/V-preprocessing custom runtime

A minimal, **conformant** custom Omnia runtime that demonstrates an
audio/video-preprocessing seam. It serves the `omnia.runtime.v1.RuntimeService`
gRPC contract and passes the [conformance suite](../../../pkg/runtime/conformance),
and shows how to wire PromptKit's real video-to-frames stage in front of a model.

See the **Authoring a custom runtime** how-to
(`docs/src/content/docs/how-to/security/authoring-a-custom-runtime.md`) for the
full contract this example implements.

## What it does

- Serves `Health`, `Converse`, `Invoke`, and `HasConversation`.
- Advertises **only** the capabilities it implements (`invoke`) — advertising a
  capability it could not back would fail conformance.
- Sends a `RuntimeHello` as its first `ServerMessage` on every `Converse` stream.
- Constructs PromptKit's real `VideoToFramesStage` (`buildVideoPreprocessStage`)
  as the A/V-preprocessing seam — where a production runtime extracts frames from
  inbound video before invoking its model.

## Run it

```sh
# from the repo root
env GOWORK=off go run ./examples/custom-runtime/av-preprocessor
# serves omnia.runtime.v1 on :9000 (override with OMNIA_GRPC_PORT)
```

## Verify conformance

```sh
env GOWORK=off go build -o runtime-conformance ./cmd/runtime-conformance
./runtime-conformance --addr localhost:9000
```

Expected: every check passes, `duplex-honesty` skips (this runtime does not
advertise `duplex_audio`), and the process exits `0`. The in-repo test
`TestExampleRuntime_IsConformant` asserts the same thing on a bufconn — no
network or ffmpeg needed, because conformance is protocol-only.

## FFmpeg base-image requirement

PromptKit's video-to-frames stage shells out to an **`ffmpeg` binary on `PATH`**
to extract frames. Omnia's default runtime image is distroless and does **not**
carry `ffmpeg`, so a runtime that actually performs video preprocessing must
supply a media-capable base image. The included `Dockerfile` installs `ffmpeg`
to make the requirement concrete.

```sh
docker build -t av-preprocessor -f examples/custom-runtime/av-preprocessor/Dockerfile .
```

Then reference the image from an AgentRuntime:

```yaml
spec:
  framework:
    type: custom
    image: av-preprocessor:latest
```
