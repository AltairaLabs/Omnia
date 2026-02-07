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

| Value | Description | Requires Credentials |
|-------|-------------|----------------------|
| `claude` | Anthropic's Claude models | Yes |
| `openai` | OpenAI's GPT models | Yes |
| `gemini` | Google's Gemini models | Yes |
| `bedrock` | AWS Bedrock | No — uses `platform` + `auth` |
| `vertex` | Google Vertex AI | No — uses `platform` + `auth` |
| `azure-ai` | Azure AI Services | No — uses `platform` + `auth` |
| `ollama` | Local Ollama models (for development) | No |
| `mock` | Mock provider (for testing) | No |

```yaml
spec:
  type: claude
```

> **Note**: For `ollama` and `mock` providers, credentials are not required. Hyperscaler types (`bedrock`, `vertex`, `azure-ai`) use platform-native authentication via the `platform` and `auth` fields instead of API key credentials.

### `model`

The model identifier to use. If not specified, the provider's default model is used.

| Provider | Example Models |
|----------|----------------|
| Claude | `claude-sonnet-4-20250514`, `claude-opus-4-20250514` |
| OpenAI | `gpt-4o`, `gpt-4-turbo`, `gpt-3.5-turbo` |
| Gemini | `gemini-pro`, `gemini-1.5-pro` |
| Bedrock | `anthropic.claude-3-5-sonnet-20241022-v2:0`, `amazon.titan-text-express-v1` |
| Vertex | `gemini-1.5-pro`, `gemini-1.5-flash` |
| Azure AI | `gpt-4o`, `gpt-4-turbo` |
| Ollama | `llava:13b`, `llama3.2-vision:11b`, `llama3:8b` |

```yaml
spec:
  type: claude
  model: claude-sonnet-4-20250514
```

### `secretRef`

:::caution[Deprecated]
The top-level `secretRef` field is deprecated. Use `credential.secretRef` instead for new providers. See the [migration guide](/how-to/migrate-provider-credentials/) for details.
:::

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

### `credential`

Flexible credential configuration supporting multiple credential strategies. Mutually exclusive with `secretRef`. Exactly one sub-field must be specified.

| Field | Type | Description |
|-------|------|-------------|
| `credential.secretRef.name` | string | Name of a Kubernetes Secret |
| `credential.secretRef.key` | string | Specific key within the Secret (auto-detected if omitted) |
| `credential.envVar` | string | Environment variable name containing the credential |
| `credential.filePath` | string | Path to a file containing the credential |

#### Using a Kubernetes Secret

Equivalent to the legacy `secretRef` field, but nested under `credential`:

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

> **Migration from `secretRef`**: The legacy `secretRef` field continues to work, but new providers should use `credential.secretRef` instead. Setting both `secretRef` and `credential` on the same Provider is rejected by CEL validation.

### `platform`

Hyperscaler-specific configuration. Required for provider types `bedrock`, `vertex`, and `azure-ai`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `platform.type` | string | Yes | Cloud platform: `aws`, `gcp`, or `azure` |
| `platform.region` | string | No | Cloud region (e.g., `us-east-1`, `us-central1`, `eastus`) |
| `platform.project` | string | No | GCP project ID (required for Vertex AI) |
| `platform.endpoint` | string | No | Override the default platform API endpoint |

A CEL validation rule enforces that `platform` is present when `type` is `bedrock`, `vertex`, or `azure-ai`.

#### AWS Bedrock

```yaml
spec:
  type: bedrock
  platform:
    type: aws
    region: us-east-1
```

#### GCP Vertex AI

```yaml
spec:
  type: vertex
  platform:
    type: gcp
    region: us-central1
    project: my-gcp-project
```

#### Azure AI

```yaml
spec:
  type: azure-ai
  platform:
    type: azure
    region: eastus
    endpoint: https://my-resource.openai.azure.com
```

### `auth`

Authentication configuration for hyperscaler providers. Only valid for provider types `bedrock`, `vertex`, and `azure-ai`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `auth.type` | string | Yes | Authentication method (see table below) |
| `auth.roleArn` | string | No | AWS IAM role ARN for IRSA (only when `platform.type` is `aws`) |
| `auth.serviceAccountEmail` | string | No | GCP service account email (only when `platform.type` is `gcp`) |
| `auth.credentialsSecretRef` | SecretKeyRef | No | Secret containing platform credentials |

#### Authentication methods

| Value | Platform | Description |
|-------|----------|-------------|
| `workloadIdentity` | All | Kubernetes-native identity federation (IRSA, GCP WI, Azure AD WI) |
| `accessKey` | AWS | Static AWS access key credentials |
| `serviceAccount` | GCP | GCP service account key |
| `servicePrincipal` | Azure | Azure service principal credentials |

**CEL validation rules:**
- `credentialsSecretRef` is **required** for `accessKey`, `serviceAccount`, and `servicePrincipal` auth types.
- `credentialsSecretRef` is **disallowed** for `workloadIdentity` (workload identity relies on pod-level identity, not secrets).

#### Workload identity (recommended)

```yaml
spec:
  type: bedrock
  platform:
    type: aws
    region: us-east-1
  auth:
    type: workloadIdentity
    roleArn: arn:aws:iam::123456789012:role/omnia-bedrock-role
```

#### Access key

```yaml
spec:
  type: bedrock
  platform:
    type: aws
    region: us-east-1
  auth:
    type: accessKey
    credentialsSecretRef:
      name: aws-credentials
```

### `baseURL`

Override the provider's default API endpoint. Useful for proxies, Azure OpenAI, or self-hosted models.

```yaml
spec:
  type: openai
  baseURL: https://my-openai-proxy.internal/v1
```

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
| `SecretFound` | Referenced Secret exists and contains required key |
| `CredentialConfigured` | Credential source is configured (secretRef, envVar, or filePath) |
| `CredentialsValid` | Credentials validated with provider (if `validateCredentials` enabled) |
| `AuthConfigured` | Auth configuration is valid (hyperscaler providers only) |

### `lastValidatedAt`

Timestamp of the last successful credential validation (only set when `validateCredentials: true`).

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

  validateCredentials: true
```

### AWS Bedrock with workload identity

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: bedrock-production
  namespace: agents
spec:
  type: bedrock
  model: anthropic.claude-3-5-sonnet-20241022-v2:0

  platform:
    type: aws
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
- [Migrate Provider Credentials](/how-to/migrate-provider-credentials/)
