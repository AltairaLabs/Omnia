---
title: "Configure Realtime Evals"
description: "Set up continuous quality evaluation for live agent conversations"
sidebar:
  order: 20
  badge:
    text: Evals
    variant: note
---


This guide walks through enabling and configuring realtime evals on an AgentRuntime so that live conversations are continuously evaluated against the eval definitions in your PromptPack.

## Prerequisites

Before enabling realtime evals, ensure:

- **Session-api is running** with PostgreSQL storage — eval results are stored in the `eval_results` table managed by session-api
- **Redis is available** — used for event publishing between session-api and the eval worker (Pattern A)
- **Provider CRDs exist** for any LLM judges you plan to use — these supply the credentials for judge API calls

## Enable Evals

Add `evals.enabled: true` to your AgentRuntime spec:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  promptPackRef:
    name: my-prompts
  providers:
    - name: default
      providerRef:
        name: claude-sonnet
  facade:
    type: websocket
  evals:
    enabled: true
```

With just `enabled: true` and no other settings, evals use these defaults:

| Setting | Default |
|---------|---------|
| `inline.groups` | `["fast-running"]` |
| `worker.groups` | `["long-running", "external"]` |
| `sampling.defaultRate` | 100 (all evals run) |
| `sampling.extendedRate` | 10 (10% of extended evals run) |
| `rateLimit.maxEvalsPerSecond` | 50 |
| `rateLimit.maxConcurrentJudgeCalls` | 5 |
| `sessionCompletion.inactivityTimeout` | 5m |

Evals will only execute if the referenced PromptPack contains eval definitions. The `inline` and `worker` defaults are deliberately disjoint: lightweight deterministic evals (contains, regex, custom scorers) run synchronously in the runtime; LLM judges and external API checks run out-of-band in the eval-worker. See [Customize Eval Routing](#customize-eval-routing) for how to override.

## Configure Judge Providers

LLM judge evals need an LLM to act as the judge. Create a Provider CRD for the judge model and add it to the AgentRuntime's `providers` list.

### 1. Create a Provider CRD

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-haiku
spec:
  type: claude
  model: claude-haiku-4-5-20251001
  secretRef:
    name: anthropic-api-key
```

### 2. Add the Judge Provider to AgentRuntime

Add a named provider entry for the judge alongside your default provider:

```yaml
spec:
  providers:
    - name: default
      providerRef:
        name: claude-sonnet       # Primary LLM for the agent
    - name: judge
      providerRef:
        name: claude-haiku        # Cheap/fast model for eval judging
  evals:
    enabled: true
```

The eval worker resolves provider credentials from the AgentRuntime's `spec.providers` list. The provider name (e.g., `"judge"`) can be referenced in PromptPack eval definitions.

## Define Evals in PromptPack

Eval definitions live in your PromptPack's `pack.json`. Add an `evals` array to the prompt that should be evaluated:

```json
{
  "prompts": {
    "customer-support": {
      "system": "You are a helpful customer support agent...",
      "evals": [
        {
          "id": "helpfulness",
          "type": "llm_judge_turn",
          "trigger": "every_turn",
          "params": {
            "judge": "fast-judge",
            "criteria": "Is the response helpful, accurate, and on-topic?",
            "rubric": "1-5 scale"
          }
        },
        {
          "id": "no-competitor-mentions",
          "type": "content_includes",
          "trigger": "every_turn",
          "params": {
            "pattern": "competitor-name",
            "should_match": false
          }
        },
        {
          "id": "resolution-check",
          "type": "llm_judge_turn",
          "trigger": "on_session_complete",
          "params": {
            "judge": "strong-judge",
            "criteria": "Did the agent fully resolve the customer's issue?"
          }
        }
      ]
    }
  }
}
```

### Available Eval Types

| Type | What it does | Cost |
|------|-------------|------|
| `llm_judge_turn` | LLM evaluates the response against criteria | LLM API call |
| `content_includes` | Regex/string match on response content | Free |
| `guardrail_triggered` | Checks if a specific validator fired | Free |

### Available Triggers

| Trigger | When it fires |
|---------|--------------|
| `every_turn` | After each assistant message |
| `on_session_complete` | When session ends or times out |
| `on_n_turns` | Every N assistant messages |

## Customize Eval Routing

Each eval runs on exactly one path by default: inline (synchronous, in the runtime) or worker (asynchronous, in the eval-worker Deployment). PromptKit auto-classifies every eval into built-in groups — `fast-running` (deterministic handlers like `contains`, `regex`), `long-running` (LLM calls), and `external` (calls to external APIs) — and each path filters by group:

```yaml
spec:
  evals:
    enabled: true
    inline:
      groups: ["fast-running"]               # default
    worker:
      groups: ["long-running", "external"]   # default
```

An eval runs on a path when at least one of its groups appears in the path's filter. Eval authors can add custom groups via the `groups` field on an `EvalDef`, then route a specific agent's worker to include that group:

```yaml
spec:
  evals:
    enabled: true
    worker:
      groups: ["long-running", "external", "custom-safety"]
```

To disable evals entirely for an agent, set `spec.evals.enabled: false`. An empty list in `groups` is **not** an off-switch — PromptKit's `FilterByGroups` treats an empty list as "no filter" and falls back to the built-in default.

Rows in the `eval_results` table include a `source` column identifying which path produced them: `"runtime-inline"` for inline, `"worker"` for the eval-worker. If you deliberately overlap the two filters (e.g. run the same eval on both paths to compare), you'll see two rows per turn per eval, one with each source tag.

## Control Costs with Sampling

For high-traffic agents, you may not want to run expensive LLM judge evals on every session. Configure sampling rates to control cost:

```yaml
spec:
  evals:
    sampling:
      defaultRate: 100    # Run all lightweight evals (fast, free)
      extendedRate: 10    # Only run extended evals on 10% of eligible turns
```

Sampling is deterministic — the same `sessionID:turnIndex` combination always produces the same sampling decision. This means results are consistent across retries and you get an evenly distributed sample.

**Cost estimation example:**

| Traffic | LLM Judge Rate | Judge Calls/Day | Estimated Cost/Day |
|---------|---------------|-----------------|-------------------|
| 500 sessions/day | 10% | ~100 | ~$0.05 (Haiku) |
| 5,000 sessions/day | 10% | ~1,000 | ~$0.50 (Haiku) |
| 50,000 sessions/day | 5% | ~5,000 | ~$2.50 (Haiku) |

## Set Rate Limits

Rate limits provide a hard ceiling on eval throughput, protecting against unexpected traffic spikes:

```yaml
spec:
  evals:
    rateLimit:
      maxEvalsPerSecond: 50          # Overall eval throughput limit
      maxConcurrentJudgeCalls: 5     # Concurrent LLM API calls
```

If the rate limit is reached, evals are queued rather than dropped. Increase these values for high-throughput agents where eval latency matters.

## Configure Session Completion

The `inactivityTimeout` controls how long the system waits after the last message before considering a session complete and running `on_session_complete` evals:

```yaml
spec:
  evals:
    sessionCompletion:
      inactivityTimeout: 10m   # Wait 10 minutes of silence
```

Set this based on your expected conversation patterns:

- **Chatbots with quick exchanges**: `2m` to `5m`
- **Complex support conversations**: `10m` to `15m`
- **Long-running async workflows**: `30m` or more

## View Eval Results

### Dashboard

The dashboard provides two views:

1. **Session detail** — open any session to see eval scores inline next to each assistant message
2. **Quality view** — aggregate pass rates and score trends across agents, viewable from the agent list

### API

Query eval results directly via session-api:

```bash
# Get eval results for a specific session
curl http://session-api:8080/api/v1/sessions/SESSION_ID/eval-results

# List eval results for an agent
curl "http://session-api:8080/api/v1/eval-results?agentName=my-agent&namespace=default"

# Get aggregate statistics
curl "http://session-api:8080/api/v1/eval-results/summary?agentName=my-agent"
```

## Verify Evals Are Running

### Check the Eval Worker Pod

For non-PromptKit agents (Pattern A), the eval worker must be deployed via Helm (see [Eval Worker Helm values](/reference/helm-values/#eval-worker-configuration)):

```bash
# Check if the eval worker is running
kubectl get deploy -l app.kubernetes.io/component=eval-worker

# View eval worker logs
kubectl logs -l app.kubernetes.io/component=eval-worker --tail=50
```

In multi-namespace mode, a single eval worker watches multiple namespaces. Check its logs to verify all namespaces are being consumed.

### Check Eval Results

Verify that results are being written:

```bash
# Query recent eval results via the API
curl "http://session-api:8080/api/v1/eval-results?limit=5"
```

### Check Agent Configuration

Verify the AgentRuntime has evals enabled:

```bash
kubectl get agentruntime my-agent -o jsonpath='{.spec.evals}'
```

## Complete Example

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: customer-support
  namespace: production
spec:
  promptPackRef:
    name: customer-support-pack
    track: stable

  providers:
    - name: default
      providerRef:
        name: claude-sonnet
    - name: judge
      providerRef:
        name: claude-haiku

  facade:
    type: websocket

  session:
    type: postgres
    storeRef:
      name: session-db

  evals:
    enabled: true
    inline:
      groups: ["fast-running"]
    worker:
      groups: ["long-running", "external"]
    sampling:
      defaultRate: 100
      extendedRate: 10
    rateLimit:
      maxEvalsPerSecond: 50
      maxConcurrentJudgeCalls: 5
    sessionCompletion:
      inactivityTimeout: 5m

  runtime:
    replicas: 3
```
