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

The provider wire protocol (message/response format).

| Value | Description | Requires Credentials |
|-------|-------------|----------------------|
| `claude` | Anthropic Claude wire protocol | Yes (unless hosted via `platform`) |
| `openai` | OpenAI chat completions wire protocol | Yes (unless hosted via `platform`) |
| `gemini` | Google Gemini wire protocol | Yes (unless hosted via `platform`) |
| `vllm` | vLLM-served OpenAI-compatible endpoint | No (auth via custom `headers`) |
| `voyageai` | Voyage AI embedding models | Yes (`VOYAGE_API_KEY`) |
| `ollama` | Local Ollama models (for development) | No |
| `mock` | Mock provider (for testing) | No |

```yaml
spec:
  type: claude
```

> **Hyperscaler hosting** (AWS Bedrock, Azure AI Foundry, GCP Vertex AI) is expressed by setting `spec.platform` (and `spec.auth`) on a `claude`, `openai`, or `gemini` provider — not as a separate provider type. See [`platform`](#platform) below.

### `model`

The model identifier to use. If not specified, the provider's default model is used.

| Provider | Example Models |
|----------|----------------|
| Claude (direct) | `claude-sonnet-4-20250514`, `claude-opus-4-20250514` |
| Claude on Bedrock | `claude-sonnet-4-20250514` — auto-mapped to the Bedrock model ID |
| OpenAI (direct) | `gpt-4o`, `gpt-4-turbo`, `gpt-3.5-turbo` |
| OpenAI on Azure | Your Azure deployment name (e.g., `gpt-4o`) |
| Gemini (direct) | `gemini-pro`, `gemini-1.5-pro` |
| Gemini on Vertex | `gemini-1.5-pro`, `gemini-1.5-flash` |
| vLLM / Ollama | Model name served by the endpoint (e.g., `llama3:8b`) |

```yaml
spec:
  type: claude
  model: claude-sonnet-4-20250514
```

### `credential`

Provider credential configuration. Exactly one sub-field must be specified (or omit `credential` entirely for providers that don't need credentials, e.g. `mock`, `ollama`, `vllm`).

| Field | Type | Description |
|-------|------|-------------|
| `credential.secretRef.name` | string | Name of a Kubernetes Secret |
| `credential.secretRef.key` | string | Specific key within the Secret (auto-detected if omitted) |
| `credential.envVar` | string | Environment variable name containing the credential |
| `credential.filePath` | string | Path to a file containing the credential |

#### Using a Kubernetes Secret

Reference to a Kubernetes Secret containing the credential value:

```yaml
spec:
  credential:
    secretRef:
      name: anthropic-credentials
      key: ANTHROPIC_API_KEY  # optional
```

#### Using an environment variable

For CI/CD pipelines or environments where credentials are pre-injected as environment variables:

```yaml
spec:
  credential:
    envVar: ANTHROPIC_API_KEY
```

The variable must be available in the runtime pod. The controller cannot validate its presence — a `CredentialConfigured` condition is set with reason `EnvVar`.

#### Using a mounted file

For credentials mounted as files (e.g., via a volume mount or CSI driver):

```yaml
spec:
  credential:
    filePath: /var/secrets/api-key
```

The file must be mounted in the runtime pod. The controller cannot validate its presence — a `CredentialConfigured` condition is set with reason `FilePath`.

### `platform`

Hyperscaler-specific configuration. Required for provider types `bedrock`, `vertex`, and `azure-ai`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `platform.type` | string | Yes | Hyperscaler hosting platform: `bedrock`, `vertex`, or `azure` |
| `platform.region` | string | No | Cloud region (e.g., `us-east-1`, `us-central1`). Required for `bedrock` and `vertex`. |
| `platform.project` | string | No | GCP project ID — required when `platform.type` is `vertex`. |
| `platform.endpoint` | string | No | Override the default platform API endpoint — required when `platform.type` is `azure`. |

**Provider × platform combinations:**

Six pairs of wire protocol (`spec.type`) and hosting platform (`spec.platform.type`) are supported — the CRD admits them and the PromptKit runtime (v1.4.6+) routes requests correctly:

| provider type | supported platforms        |
|---------------|----------------------------|
| `claude`      | `bedrock`, `vertex`, `azure` |
| `openai`      | `azure`, `bedrock`           |
| `gemini`      | `vertex`                     |

Three pairs are rejected at admission because the hyperscaler does not host that model vendor as a partner endpoint:

- `openai × vertex` — Vertex AI does not host OpenAI.
- `gemini × bedrock` — AWS Bedrock does not host Gemini.
- `gemini × azure` — Azure AI Foundry does not host Gemini.

Setting `spec.platform` on any other provider type (e.g., `vllm`, `ollama`, `mock`) is also rejected at admission. Setting `spec.platform` without `spec.auth` (or vice versa) is likewise rejected.

The authentication method is determined by the platform (see [`auth`](#auth) below):

| platform | allowed auth methods                    |
|----------|-----------------------------------------|
| bedrock  | `workloadIdentity`, `accessKey`         |
| vertex   | `workloadIdentity`, `serviceAccount`    |
| azure    | `workloadIdentity`, `servicePrincipal`  |

#### Claude on AWS Bedrock

```yaml
spec:
  type: claude
  model: claude-sonnet-4-20250514   # auto-mapped to the Bedrock model ID
  platform:
    type: bedrock
    region: us-east-1
```

#### Gemini on GCP Vertex AI

```yaml
spec:
  type: gemini
  model: gemini-1.5-pro
  platform:
    type: vertex
    region: us-central1
    project: my-gcp-project
```

#### OpenAI on Azure AI Foundry

```yaml
spec:
  type: openai
  model: gpt-4o
  platform:
    type: azure
    endpoint: https://my-resource.openai.azure.com
```

### `auth`

Authentication configuration for platform-hosted providers. Required when `spec.platform` is set; forbidden otherwise.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `auth.type` | string | Yes | Authentication method (see matrix below) |
| `auth.roleArn` | string | No | AWS IAM role ARN for IRSA (only when `platform.type` is `bedrock`) |
| `auth.serviceAccountEmail` | string | No | GCP service account email (only when `platform.type` is `vertex`) |
| `auth.credentialsSecretRef` | SecretKeyRef | No | Secret containing platform credentials (required for static auth) |

#### Platform × auth matrix (enforced by CEL)

| `platform.type` | allowed `auth.type` |
|-----------------|---------------------|
| `bedrock`       | `workloadIdentity`, `accessKey` |
| `vertex`        | `workloadIdentity`, `serviceAccount` |
| `azure`         | `workloadIdentity`, `servicePrincipal` |

**Expected secret keys per static auth type:**

| `auth.type`       | Required keys in `credentialsSecretRef` |
|-------------------|------------------------------------------|
| `accessKey`       | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (optional `AWS_SESSION_TOKEN`) |
| `serviceAccount`  | `credentials.json` (or a custom key set via `credentialsSecretRef.key`) |
| `servicePrincipal`| `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` |

**CEL rules:**
- `credentialsSecretRef` is **required** for `accessKey`, `serviceAccount`, and `servicePrincipal`.
- `credentialsSecretRef` is **disallowed** for `workloadIdentity` (workload identity relies on pod-level identity).

#### Workload identity (recommended)

```yaml
spec:
  type: claude
  platform:
    type: bedrock
    region: us-east-1
  auth:
    type: workloadIdentity
    roleArn: arn:aws:iam::123456789012:role/omnia-bedrock-role
```

#### Access key

```yaml
spec:
  type: claude
  platform:
    type: bedrock
    region: us-east-1
  auth:
    type: accessKey
    credentialsSecretRef:
      name: aws-credentials
```

### `baseURL`

Override the provider's default API endpoint. Useful for proxies, gateways (OpenRouter), or self-hosted models.

```yaml
spec:
  type: openai
  baseURL: https://my-openai-proxy.internal/v1
```

### `headers`

Custom HTTP headers included on every provider request. Typical use: gateway providers that require attribution headers (OpenRouter's `HTTP-Referer` and `X-Title`), or shared vLLM deployments that use tenant routing headers.

```yaml
spec:
  type: openai
  baseURL: https://openrouter.ai/api/v1
  headers:
    HTTP-Referer: https://example.com
    X-Title: omnia
  credential:
    secretRef:
      name: openrouter-credentials
```

Collisions with built-in provider headers are rejected by PromptKit at request time.

### `capabilities`

Lists what modalities and features this provider supports. Used for capability-based filtering when binding Arena providers. The field is optional — existing providers continue to work without it.

| Value | Description |
|-------|-------------|
| `text` | Text generation |
| `streaming` | Streaming responses |
| `vision` | Image/visual input |
| `tools` | Tool/function calling |
| `json` | Structured JSON output |
| `audio` | Audio input/output |
| `video` | Video input |
| `documents` | Document (PDF) input |
| `duplex` | Full-duplex (bidirectional streaming) |

```yaml
spec:
  capabilities:
    - text
    - streaming
    - vision
    - tools
    - json
```

### `defaults`

Tuning parameters applied to all requests using this provider.

| Field | Type | Range | Description |
|-------|------|-------|-------------|
| `temperature` | string | 0.0-2.0 | Controls randomness (lower = more focused) |
| `topP` | string | 0.0-1.0 | Nucleus sampling threshold |
| `maxTokens` | integer | - | Maximum tokens in response |
| `contextWindow` | integer | - | Model's maximum context size in tokens. When conversation history exceeds this budget, truncation is applied. If not specified, no automatic truncation is performed. |
| `truncationStrategy` | string | - | How to handle context overflow: `sliding` (default — remove oldest messages first), `summarize` (summarize old messages before removing), `custom` (delegate to custom runtime implementation) |

```yaml
spec:
  defaults:
    temperature: "0.7"
    topP: "0.9"
    maxTokens: 4096
    contextWindow: 200000
    truncationStrategy: sliding
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
| `SecretFound` | Referenced Secret exists and contains required key |
| `CredentialConfigured` | Credential source is configured (secretRef, envVar, or filePath) |
| `AuthConfigured` | Auth configuration is valid (hyperscaler providers only) |

## Complete Examples

### API key provider

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

  credential:
    secretRef:
      name: anthropic-credentials

  capabilities:
    - text
    - streaming
    - vision
    - tools
    - json

  defaults:
    temperature: "0.7"
    maxTokens: 4096
    contextWindow: 200000
    truncationStrategy: sliding

  pricing:
    inputCostPer1K: "0.003"
    outputCostPer1K: "0.015"
```

### Claude on AWS Bedrock with workload identity

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: bedrock-production
  namespace: agents
spec:
  type: claude
  model: claude-sonnet-4-20250514   # auto-mapped to the corresponding Bedrock model ID

  platform:
    type: bedrock
    region: us-east-1

  auth:
    type: workloadIdentity
    roleArn: arn:aws:iam::123456789012:role/omnia-bedrock-role

  capabilities:
    - text
    - streaming
    - vision
    - tools

  defaults:
    temperature: "0.7"
    maxTokens: 4096
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
  credential:
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
  credential:
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

## Related Guides

- [Configure AWS Bedrock Provider](/how-to/configure-bedrock-provider/)
- [Configure GCP Vertex AI Provider](/how-to/configure-vertex-provider/)
- [Configure Azure AI Provider](/how-to/configure-azure-ai-provider/)
