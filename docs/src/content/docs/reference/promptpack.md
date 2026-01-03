---
title: "PromptPack CRD"
description: "Complete reference for the PromptPack custom resource"
sidebar:
  order: 2
---


The PromptPack custom resource defines a versioned collection of prompts for AI agents. The ConfigMap it references must contain a valid [PromptPack](https://promptpack.org/docs/spec/schema-reference) - a structured JSON/YAML format for packaging multi-prompt conversational systems.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
```

## Spec Fields

### `source`

Source of the compiled PromptPack content.

| Field | Type | Required |
|-------|------|----------|
| `source.configMapRef.name` | string | Yes |
| `source.configMapRef.key` | string | Yes |

```yaml
spec:
  source:
    configMapRef:
      name: my-prompts
      key: promptpack.json  # Key containing the compiled PromptPack
```

### `rollout`

Rollout strategy for prompt updates.

| Field | Type | Default | Required |
|-------|------|---------|----------|
| `rollout.strategy` | string | immediate | No |
| `rollout.canary.weight` | integer | - | No |

```yaml
spec:
  rollout:
    strategy: canary
    canary:
      weight: 20  # 20% of traffic uses new prompts
```

Strategies:
- `immediate` - Updates apply immediately to all agents
- `canary` - Gradual rollout with traffic splitting

## Status Fields

### `phase`

Current phase of the PromptPack.

| Value | Description |
|-------|-------------|
| `Pending` | Validating source |
| `Active` | Prompts are valid and in use |
| `Canary` | Canary rollout in progress |
| `Failed` | Source validation failed |

### `activeVersion`

The currently active prompt version (content hash).

### `canaryVersion`

The canary version during rollout (if applicable).

### `conditions`

| Type | Description |
|------|-------------|
| `SourceValid` | ConfigMap exists and has valid content |
| `AgentsNotified` | Referencing agents have been notified |

## ConfigMap Format

The referenced ConfigMap must contain a compiled PromptPack following the [PromptPack specification](https://promptpack.org/docs/spec/schema-reference):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-prompts
data:
  promptpack.json: |
    {
      "id": "customer-service",
      "name": "Customer Service Assistant",
      "version": "1.0.0",
      "template_engine": {
        "version": "v1",
        "syntax": "{{variable}}"
      },
      "prompts": {
        "support": {
          "id": "support",
          "name": "General Support",
          "version": "1.0.0",
          "system_template": "You are a helpful customer service agent for {{company_name}}. Be professional, empathetic, and solution-oriented.",
          "variables": [
            {
              "name": "company_name",
              "type": "string",
              "required": true,
              "description": "Name of the company"
            }
          ],
          "parameters": {
            "temperature": 0.7,
            "max_tokens": 1024
          },
          "validators": {
            "banned_words": ["competitor", "lawsuit"]
          }
        }
      },
      "metadata": {
        "domain": "customer-service",
        "language": "en",
        "tags": ["support", "helpdesk"]
      }
    }
```

### PromptPack Structure

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique identifier (lowercase, hyphens allowed) |
| `name` | string | Yes | Human-readable name |
| `version` | string | Yes | Semantic version (MAJOR.MINOR.PATCH) |
| `template_engine` | object | Yes | Template configuration |
| `prompts` | object | Yes | Map of prompt definitions |
| `metadata` | object | No | Domain, language, tags |
| `tools` | object | No | Tool definitions for function calling |
| `fragments` | object | No | Reusable template fragments |

For the complete specification, see [promptpack.org](https://promptpack.org/docs/spec/schema-reference).

## Canary Rollout

### Start Canary

```yaml
spec:
  rollout:
    strategy: canary
    canary:
      weight: 10  # Start with 10%
```

### Increase Traffic

Update the weight to increase canary traffic:

```yaml
spec:
  rollout:
    canary:
      weight: 50  # Increase to 50%
```

### Promote to Active

Set weight to 100 to promote canary:

```yaml
spec:
  rollout:
    canary:
      weight: 100  # Promotes canary to active
```

The status will transition from `Canary` to `Active`.

## Example

Complete PromptPack example with canary rollout:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: customer-service
  namespace: agents
spec:
  source:
    configMapRef:
      name: cs-prompts-v2
      key: promptpack.json
  rollout:
    strategy: canary
    canary:
      weight: 25
```

Status after deployment:

```yaml
status:
  phase: Canary
  activeVersion: "abc123"
  canaryVersion: "def456"
  conditions:
    - type: SourceValid
      status: "True"
    - type: AgentsNotified
      status: "True"
      message: "Notified 3 AgentRuntimes"
```

## Authoring PromptPacks

PromptPacks can be authored in YAML for readability and compiled to JSON:

```yaml
id: customer-service
name: Customer Service Assistant
version: 1.0.0

template_engine:
  version: v1
  syntax: "{{variable}}"

prompts:
  support:
    id: support
    name: General Support
    version: 1.0.0
    system_template: |
      You are a helpful customer service agent for {{company_name}}.
      Be professional, empathetic, and solution-oriented.

      Guidelines:
      - Always greet the customer warmly
      - Listen actively and acknowledge concerns
      - Provide clear, actionable solutions
    variables:
      - name: company_name
        type: string
        required: true
    parameters:
      temperature: 0.7
      max_tokens: 1024

metadata:
  domain: customer-service
  language: en
```

Compile with [packc](https://promptpack.org):

```bash
packc compile customer-service.promptpack.yaml -o promptpack.json
```
