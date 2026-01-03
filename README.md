# Omnia

[![CI](https://github.com/AltairaLabs/Omnia/workflows/CI/badge.svg)](https://github.com/AltairaLabs/Omnia/actions/workflows/ci.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=AltairaLabs_Omnia&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=AltairaLabs_Omnia)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=AltairaLabs_Omnia&metric=coverage)](https://sonarcloud.io/summary/new_code?id=AltairaLabs_Omnia)
[![Go Report Card](https://goreportcard.com/badge/github.com/AltairaLabs/Omnia)](https://goreportcard.com/report/github.com/AltairaLabs/Omnia)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

**The Kubernetes Platform for AI Agent Deployment**

Omnia is a Kubernetes operator that makes deploying, scaling, and managing AI agents simple. Deploy intelligent assistants that can safely access private, proprietary information — all within your existing infrastructure.

## Features

- **Kubernetes-Native**: Deploy AI agents as custom resources with full GitOps support
- **Multiple LLM Providers**: Support for Claude, OpenAI, and Gemini with easy provider switching
- **Autoscaling**: Scale-to-zero with KEDA or standard HPA based on active connections
- **Tool Integration**: HTTP, gRPC, and MCP tool adapters for extending agent capabilities
- **Session Management**: Redis or in-memory session stores for conversation persistence
- **Observability**: Integrated Prometheus metrics, Grafana dashboards, Loki logs, and Tempo traces
- **Production-Ready**: Health checks, graceful shutdown, and comprehensive RBAC

## Quick Start

### Prerequisites

- Kubernetes 1.28+
- Helm 3.x
- kubectl configured for your cluster

### Install the Operator

```bash
helm install omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  --create-namespace
```

### Deploy Your First Agent

1. Create a PromptPack with compiled prompts:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-prompts
data:
  # Compiled PromptPack JSON (use `packc` to compile from YAML source)
  promptpack.json: |
    {
      "$schema": "https://promptpack.org/schema/v1/promptpack.schema.json",
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

> **Tip**: Use [packc](https://promptpack.org) to compile PromptPacks from YAML source files with validation.

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
  facade:
    type: websocket
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

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

## License

Apache 2.0 - see [LICENSE](LICENSE) for details.

---

Built with care by [AltairaLabs](https://altairalabs.ai)
