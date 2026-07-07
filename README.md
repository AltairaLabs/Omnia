# Omnia

[![CI](https://github.com/AltairaLabs/Omnia/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/AltairaLabs/Omnia/actions/workflows/ci.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=AltairaLabs_Omnia&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=AltairaLabs_Omnia)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=AltairaLabs_Omnia&metric=coverage)](https://sonarcloud.io/summary/new_code?id=AltairaLabs_Omnia)
[![Go Report Card](https://goreportcard.com/badge/github.com/AltairaLabs/Omnia)](https://goreportcard.com/report/github.com/AltairaLabs/Omnia)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**The Kubernetes platform for deploying AI agents**

Omnia is a Kubernetes operator that deploys, scales, and manages AI agents as Kubernetes resources. Deploy intelligent assistants that can safely access private, proprietary information — all within your existing infrastructure.

**Open core:** the core platform is free and open source under Apache 2.0. Advanced production and scale features ship in the Enterprise edition under the [Functional Source License](https://github.com/AltairaLabs/Omnia/blob/main/ee/LICENSE) in `ee/`. See [Licensing & Features](https://omnia.altairalabs.ai/explanation/licensing/) for what's included in each edition.

## Features

- **Kubernetes-Native**: Deploy AI agents as custom resources with full GitOps support
- **Multiple LLM Providers**: Support for Claude, OpenAI, and Gemini with per-agent provider selection
- **Autoscaling**: Scale-to-zero with KEDA or standard HPA based on active connections
- **Tool Integration**: HTTP, gRPC, and MCP tool adapters for extending agent capabilities
- **Session Management**: Redis or in-memory session stores for conversation persistence
- **Observability**: Integrated Prometheus metrics, Grafana dashboards, Loki logs, and Tempo traces
- **Production-Ready**: Health checks, graceful shutdown, and RBAC

## Quick Start

### Prerequisites

- Kubernetes 1.28+
- Helm 3.x
- kubectl configured for your cluster

### Install the Operator

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  --devel \
  --namespace omnia-system \
  --create-namespace \
  --set dashboard.auth.mode=builtin \
  --set dashboard.auth.sessionSecret="$(openssl rand -base64 32)"
```

> `--devel` is required while Omnia ships pre-release (beta) charts. The dashboard
> uses **builtin** auth — after install, open it and register the first user.
> See [Dashboard Auth](https://omnia.altairalabs.ai/how-to/configure-dashboard-auth/).

### Deploy Your First Agent

1. Create a PromptPack with compiled prompts:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-prompts
data:
  # Compiled PromptPack JSON (use `packc` to compile from YAML source)
  pack.json: |
    {
      "$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
      "id": "my-assistant",
      "name": "My Assistant",
      "version": "1.0.0",
      "template_engine": {"version": "v1", "syntax": "{{variable}}"},
      "prompts": {
        "main": {
          "id": "main",
          "name": "Main Assistant",
          "version": "1.0.0",
          "system_template": "You are a helpful AI assistant. Be concise and accurate.",
          "parameters": {"temperature": 0.7, "max_tokens": 4096}
        }
      }
    }
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: my-pack
spec:
  version: "1.0.0"
  source:
    type: configmap
    configMapRef:
      name: my-prompts
```

> **Tip**: Use [packc](https://promptkit.altairalabs.ai/packc/reference/) to compile PromptPacks from YAML source files with validation.

2. Create a Provider for LLM credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
stringData:
  ANTHROPIC_API_KEY: "sk-ant-..."
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-provider
spec:
  type: claude
  model: claude-sonnet-4-20250514
  secretRef:
    name: llm-credentials
```

3. Deploy an AgentRuntime:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  promptPackRef:
    name: my-pack
  providerRef:
    name: claude-provider
  facades:
    - type: websocket
      port: 8080
```

4. Connect to your agent:

```bash
kubectl port-forward svc/my-agent 8080:8080
websocat ws://localhost:8080/ws
```

## Custom Resources

| CRD | Description |
|-----|-------------|
| **AgentRuntime** | Deploys and manages an AI agent with its facade, sessions, and scaling |
| **PromptPack** | Defines agent prompts and system instructions |
| **ToolRegistry** | Configures tools available to agents (HTTP, gRPC, MCP) |
| **Provider** | Reusable LLM provider configuration with credentials |

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     Omnia Operator                            │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │
│  │ AgentRuntime│  │ PromptPack  │  │ToolRegistry │           │
│  │ Controller  │  │ Controller  │  │ Controller  │           │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘           │
└─────────┼────────────────┼────────────────┼──────────────────┘
          │                │                │
          ▼                ▼                ▼
┌──────────────────────────────────────────────────────────────┐
│                      Agent Pod                                │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐              │
│  │  Facade    │  │  Runtime   │  │   Tools    │              │
│  │ (WebSocket)│◄─┤  (gRPC)    │──┤  Adapter   │              │
│  └────────────┘  └────────────┘  └────────────┘              │
└──────────────────────────────────────────────────────────────┘
```

## Documentation

Full documentation is available at [omnia.altairalabs.ai](https://omnia.altairalabs.ai).

- [Getting Started](https://omnia.altairalabs.ai/tutorials/getting-started/)
- [Local Development](https://omnia.altairalabs.ai/how-to/local-development/)
- [CRD Reference](https://omnia.altairalabs.ai/reference/agentruntime/)
- [Scaling Agents](https://omnia.altairalabs.ai/how-to/scale-agents/)
- [Observability Setup](https://omnia.altairalabs.ai/how-to/setup-observability/)

## Ecosystem

Omnia is part of the AltairaLabs open-source ecosystem:

| Project | Description |
|---------|-------------|
| [PromptKit](https://github.com/AltairaLabs/PromptKit) | Go SDK for building AI agents with tool use and streaming |
| [PromptPack](https://promptpack.org) | Specification for portable, testable AI agent definitions |
| **Omnia** | Kubernetes platform for deploying PromptKit agents at scale |

## Security

To report a vulnerability, see our [Security Policy](https://github.com/AltairaLabs/Omnia/blob/main/SECURITY.md) (please use
private disclosure — not a public issue).

Vulnerabilities are scanned continuously and triaged in the
[Security tab](https://github.com/AltairaLabs/Omnia/security/code-scanning):
**govulncheck** (reachability-based Go stdlib + module CVEs, gating CI),
**Trivy** (container images), **CodeQL** (SAST), **Dependabot** +
`dependency-review` (dependencies), and **OpenSSF Scorecard** (supply-chain
posture). All service images build statically onto `distroless/static:nonroot`,
keeping the base-layer CVE surface minimal.

## Contributing

Contributions are welcome. Please read our [Contributing Guide](https://github.com/AltairaLabs/Omnia/blob/main/CONTRIBUTING.md) for details.

## License

Omnia is **open core**:

- The core platform is licensed under **Apache 2.0** — see [LICENSE](https://github.com/AltairaLabs/Omnia/blob/main/LICENSE).
- Enterprise features under `ee/` are licensed under the **Functional Source License 1.1 (FSL-1.1)** — see [ee/LICENSE](https://github.com/AltairaLabs/Omnia/blob/main/ee/LICENSE).

See [Licensing & Features](https://omnia.altairalabs.ai/explanation/licensing/) for the Open Core vs Enterprise comparison.

---

Built with care by [AltairaLabs](https://altairalabs.ai)
