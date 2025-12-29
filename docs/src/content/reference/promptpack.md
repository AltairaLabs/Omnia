---
title: "PromptPack CRD"
description: "Complete reference for the PromptPack custom resource"
order: 2
---

# PromptPack CRD Reference

The PromptPack custom resource defines a versioned collection of prompts for AI agents.

## API Version

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
```

## Spec Fields

### `source`

Source of the prompt content.

| Field | Type | Required |
|-------|------|----------|
| `source.configMapRef.name` | string | Yes |
| `source.configMapRef.key` | string | No |

```yaml
spec:
  source:
    configMapRef:
      name: my-prompts
      key: prompts.yaml  # Optional, uses all keys if not specified
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

The referenced ConfigMap should contain prompt files:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-prompts
data:
  system.txt: |
    You are a helpful AI assistant.
    Be concise and accurate.

  greeting.txt: |
    Hello! How can I help you today?

  error.txt: |
    I apologize, but I encountered an error.
    Please try again.
```

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

Complete PromptPack example:

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
