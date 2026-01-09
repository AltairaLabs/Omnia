---
title: "Provider CRD"
description: "Complete reference for the Provider custom resource"
sidebar:
  order: 4
---


The Provider custom resource defines a reusable LLM provider configuration that can be referenced by multiple AgentRuntimes. This enables centralized credential management and consistent model configuration across agents.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
```

## Spec Fields

### `type`

The LLM provider type.

| Value | Description | Requires Secret |
|-------|-------------|-----------------|
| `claude` | Anthropic's Claude models | Yes |
| `openai` | OpenAI's GPT models | Yes |
| `gemini` | Google's Gemini models | Yes |
| `ollama` | Local Ollama models (for development) | No |
| `mock` | Mock provider (for testing) | No |
| `auto` | Auto-detect based on available credentials | Yes |

```yaml
spec:
  type: claude
```

> **Note**: For `ollama` and `mock` providers, `secretRef` is not required. Use inline `provider` configuration in AgentRuntime instead of the Provider CRD for these types.

### `model`

The model identifier to use. If not specified, the provider's default model is used.

| Provider | Example Models |
|----------|----------------|
| Claude | `claude-sonnet-4-20250514`, `claude-opus-4-20250514` |
| OpenAI | `gpt-4o`, `gpt-4-turbo`, `gpt-3.5-turbo` |
| Gemini | `gemini-pro`, `gemini-1.5-pro` |
| Ollama | `llava:13b`, `llama3.2-vision:11b`, `llama3:8b` |

```yaml
spec:
  type: claude
  model: claude-sonnet-4-20250514
```

### `secretRef`

Reference to a Secret containing API credentials.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secretRef.name` | string | Yes | Name of the Secret |
| `secretRef.key` | string | No | Specific key to use (auto-detected if omitted) |

```yaml
spec:
  secretRef:
    name: llm-credentials
```

If `key` is not specified, the controller looks for provider-appropriate keys:
- **Claude**: `ANTHROPIC_API_KEY` or `api-key`
- **OpenAI**: `OPENAI_API_KEY` or `api-key`
- **Gemini**: `GEMINI_API_KEY` or `api-key`

### `baseURL`

Override the provider's default API endpoint. Useful for proxies, Azure OpenAI, or self-hosted models.

```yaml
spec:
  type: openai
  baseURL: https://my-openai-proxy.internal/v1
```

### `defaults`

Tuning parameters applied to all requests using this provider.

| Field | Type | Range | Description |
|-------|------|-------|-------------|
| `temperature` | string | 0.0-2.0 | Controls randomness (lower = more focused) |
| `topP` | string | 0.0-1.0 | Nucleus sampling threshold |
| `maxTokens` | integer | - | Maximum tokens in response |

```yaml
spec:
  defaults:
    temperature: "0.7"
    topP: "0.9"
    maxTokens: 4096
```

### `pricing`

Custom pricing for cost tracking. If not specified, PromptKit's built-in pricing is used.

| Field | Type | Description |
|-------|------|-------------|
| `inputCostPer1K` | string | Cost per 1000 input tokens |
| `outputCostPer1K` | string | Cost per 1000 output tokens |
| `cachedCostPer1K` | string | Cost per 1000 cached tokens |

```yaml
spec:
  pricing:
    inputCostPer1K: "0.003"
    outputCostPer1K: "0.015"
    cachedCostPer1K: "0.0003"
```

### `validateCredentials`

When enabled, the controller validates credentials with the provider during reconciliation.

```yaml
spec:
  validateCredentials: true
```

## Status Fields

### `phase`

| Value | Description |
|-------|-------------|
| `Ready` | Provider is configured and credentials are valid |
| `Error` | Configuration error or invalid credentials |

### `conditions`

| Type | Description |
|------|-------------|
| `Ready` | Overall readiness of the Provider |
| `SecretValid` | Referenced Secret exists and contains required key |
| `CredentialsValidated` | Credentials validated with provider (if enabled) |

### `lastValidatedAt`

Timestamp of the last successful credential validation (only set when `validateCredentials: true`).

## Complete Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: anthropic-credentials
  namespace: agents
stringData:
  ANTHROPIC_API_KEY: "sk-ant-api03-..."
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-production
  namespace: agents
spec:
  type: claude
  model: claude-sonnet-4-20250514

  secretRef:
    name: anthropic-credentials

  defaults:
    temperature: "0.7"
    maxTokens: 4096

  pricing:
    inputCostPer1K: "0.003"
    outputCostPer1K: "0.015"

  validateCredentials: true
```

## Using Provider in AgentRuntime

Reference a Provider from an AgentRuntime using `providerRef`:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  promptPackRef:
    name: my-prompts

  providerRef:
    name: claude-production
    namespace: agents  # Optional, defaults to same namespace

  facade:
    type: websocket
    port: 8080
```

## Multiple Providers

You can create multiple Provider resources for different use cases:

```yaml
# Production provider with Claude Sonnet
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-production
spec:
  type: claude
  model: claude-sonnet-4-20250514
  secretRef:
    name: prod-credentials
  defaults:
    temperature: "0.3"  # More deterministic
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-development
spec:
  type: claude
  model: claude-haiku-20250514
  secretRef:
    name: dev-credentials
  defaults:
    temperature: "0.7"
```

## Cross-Namespace References

Providers can be referenced across namespaces:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
  namespace: app-team
spec:
  providerRef:
    name: shared-claude-provider
    namespace: shared-providers  # Provider in different namespace
```

Note: Ensure appropriate RBAC permissions are configured for cross-namespace access.
