# Running an Omnia runtime without a cluster

Every Omnia binary that builds its Kubernetes client through
`pkg/k8s.NewClient()` — the runtime, the facade, the arena workers — can run
against a **directory of YAML manifests** instead of a real cluster. Set
`OMNIA_CONFIG_DIR` to a devroot and `NewClient()` returns a controller-runtime
fake client seeded from every `*.yaml` / `*.yml` in it. Unset the variable and
you get the normal in-cluster client, unchanged.

This is a local-dev convenience — the fake client is in-memory, not a real
control plane.

## What the devroot provides

`OMNIA_CONFIG_DIR` covers only what the runtime reads from the **Kubernetes
API**: `AgentRuntime`, `Provider`, `Secret`, `Workspace`, and the agent's
`Namespace`. See [`devroot/manifests.yaml`](./devroot/manifests.yaml) for a
minimal, fully-offline example (a `mock` provider, so no real LLM key).

The PromptPack and tools config are **not** CRDs — they are already mounted
files, so they keep their own env vars (`OMNIA_PROMPTPACK_PATH`,
`OMNIA_TOOLS_CONFIG_PATH`).

## Run it

```bash
export OMNIA_CONFIG_DIR=./examples/custom-runtime/devroot
export OMNIA_AGENT_NAME=demo
export OMNIA_NAMESPACE=dev
export OMNIA_PROMPTPACK_PATH=/path/to/pack.json   # your mounted pack
go run ./cmd/runtime
```

Or in a container, mounting the devroot:

```bash
docker run --rm \
  -e OMNIA_CONFIG_DIR=/devroot \
  -e OMNIA_AGENT_NAME=demo \
  -e OMNIA_NAMESPACE=dev \
  -e OMNIA_PROMPTPACK_PATH=/etc/omnia/pack/pack.json \
  -v "$PWD/examples/custom-runtime/devroot:/devroot" \
  -v "$PWD/pack.json:/etc/omnia/pack/pack.json" \
  ghcr.io/altairalabs/omnia-runtime:latest
```

To point at a real provider instead of `mock`, add a `Secret` to the devroot
and reference it from the `Provider` exactly as you would in-cluster — the
runtime reads it through the same code path.
