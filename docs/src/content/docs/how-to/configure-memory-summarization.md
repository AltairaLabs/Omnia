---
title: "Configure Memory Summarization"
description: "Turn on real LLM-based compaction for a memory-api by deploying a summarizer agent + CronJob."
sidebar:
  order: 30
---

Omnia's memory-api ships with an always-on compaction worker that caps
memory growth by superseding old observations with synthetic summaries.
The default `NoopSummarizer` produces a `"Summary of N observations"`
placeholder — it bounds the dataset but adds no recall value.

Real summarization is opt-in per memory-service. Because summarization
strategy depends on the data domain (a support workspace and a
medical-records workspace want very different prompts and providers),
Omnia configures it via a **regular agent**, invoked on a schedule by a
Kubernetes CronJob. You author the prompt like any other agent prompt,
pick a provider like any other agent provider, and evaluate it through
the same tooling.

## How it fits together

```
CronJob (6h)  ──►  a2a-invoker  ──A2A──►  summarizer AgentRuntime
                                                │
                                             uses tools
                                                ▼
                                          memory-api
                                       /api/v1/compaction/...
```

The summarizer agent gets two tools: one that lists compaction
candidates (buckets of stale observations), and one that writes the
summary and marks the originals superseded. On each tick, the agent
runs through every bucket, reads the observations, produces a summary,
and persists it.

## Prerequisites

- A memory-api already deployed in your workspace.
- A `Provider` CRD you can use for summarization. Cheap capable models
  work well here (Anthropic Haiku, gpt-5-nano, a local Ollama model).
- A `docker pull` path for `ghcr.io/altairalabs/omnia-a2a-invoker` —
  the small tool image the CronJob runs.

## Step 1: Deploy the summarizer bundle

Copy the reference bundle and substitute your values:

```bash
# Start from the sample and rewrite the three placeholders.
cp config/samples/omnia_v1alpha1_memory_summarizer.yaml \
   /tmp/memory-summarizer.yaml

sed -i \
  -e 's|<workspace-namespace>|workspace-support|g' \
  -e 's|<memory-api-host>|omnia-memory-api.workspace-support.svc.cluster.local|g' \
  -e 's|<provider-name>|claude-haiku|g' \
  /tmp/memory-summarizer.yaml

kubectl apply -f /tmp/memory-summarizer.yaml
```

The bundle creates:

- A `ConfigMap` holding the compiled `PromptPack` JSON with the
  summarization prompt.
- A `PromptPack` CRD pointing at that ConfigMap.
- A `ToolRegistry` CRD whose two HTTP handlers point at the
  memory-api's `/api/v1/compaction/candidates` (GET) and
  `/api/v1/compaction/summaries` (POST) endpoints.
- An `AgentRuntime` CRD that ties the PromptPack, ToolRegistry, and
  Provider together. `facade.type: a2a` means no browser WebSocket —
  this agent exists to be called by other agents / cron.
- A `ServiceAccount` and `CronJob` that runs `a2a-invoker` every
  6 hours.

## Step 2: Verify the agent is up

```bash
kubectl -n workspace-support get agentruntime memory-summarizer-agent
kubectl -n workspace-support get pods -l app.kubernetes.io/instance=memory-summarizer-agent
```

The runtime pod should be `Running` and the AgentRuntime `.status.phase`
should be `Ready`.

## Step 3: Trigger it manually the first time

CronJobs run on schedule, but you don't want to wait 6 hours to find
out you fat-fingered a placeholder. Create a one-off Job from the
CronJob template:

```bash
kubectl -n workspace-support create job \
  --from=cronjob/memory-summarizer memory-summarizer-manual-$(date +%s)
```

Follow the logs:

```bash
kubectl -n workspace-support logs -l job-name=memory-summarizer-manual-XXX
```

On success you'll see:

- `invoking agent` — the invoker resolved the AgentRuntime's A2A URL.
- `agent responded` with a `taskID` and `state=completed`.
- The agent's final report printed on stdout (e.g.
  `Summarized 4 buckets, skipped 1.`).

## Tuning the prompt

The default prompt under `config/samples/omnia_v1alpha1_memory_summarizer.yaml`
is conservative — it preserves durable facts and drops conversational
scaffolding. Different data domains want different tradeoffs:

- **Support conversations**: the default is probably right.
- **Regulatory / audit memory**: tighten "never invent facts" and add a
  "prefer verbatim quotes" instruction.
- **Marketing / outreach memory**: loosen to allow the model to
  synthesize sentiment and intent.

The prompt lives in the `ConfigMap.data."pack.json"` under
`prompts.summarize.system_template`. Edit, reapply, and the next
CronJob tick picks it up.

Use the PromptArena tooling to A/B-test changes before rolling them
out: it treats the summarizer exactly like any other prompt pack, so
scenarios you already author for customer-facing agents apply here too.

## Tuning the schedule and budget

Three knobs control cost:

1. **CronJob `schedule`** — how often the agent runs. Default 6 hours.
   Shorter = faster compaction, more LLM spend.
2. **`max_per_bucket`** — caps how many observations the agent sees
   per bucket. The tool-registry default is 50; the memory-api caps
   it at 500. Lower = cheaper per call, risk of losing nuance across
   long conversations.
3. **Provider choice** — switching from a frontier model to a cheap
   model typically drops cost 10-50x with a modest quality impact on
   this task.

Compaction is idempotent; if a scheduled run fails, the next one picks
up the same buckets. If two workers try to summarize the same bucket,
the second gets a `409 Conflict` from the memory-api and treats it as
a no-op.

## Running multiple summarizers per workspace

Workspaces can have **zero or more memory services** (one per service
group). Each memory-service that needs real summarization gets its own
copy of the bundle — each trio of PromptPack, AgentRuntime, ToolRegistry
plus its CronJob. The tool URLs in the ToolRegistry are how each agent
is bound to a specific memory-api: the invoker's `--agent` flag selects
which summarizer to kick.

There is no cross-service coordination. Each pair runs on its own
schedule, writes to its own Postgres, under its own Provider / cost
ledger.

## Troubleshooting

- **CronJob runs but nothing summarized**: check the agent's response
  text in the Job's stdout. The agent may report "no buckets matched"
  — the default `older_than_hours` is 720 (30d) and
  `min_group_size` is 10. Tight workspaces with infrequent writes
  won't have eligible buckets until they accumulate.

- **`ErrCompactionRaced` in the agent logs**: another writer beat the
  agent to one or more buckets. Always safe to ignore.

- **`403` from the tool calls**: the memory-api rejected the request.
  Confirm the ToolRegistry endpoint URL and that the CronJob's
  `ServiceAccount` has a projected token that the memory-api trusts.

- **The agent hallucinates facts in summaries**: tighten the prompt
  ("Only state facts literally present in the observations. When in
  doubt, skip.") and re-evaluate with a lower-temperature provider.

## See also

- Plan document: `docs/local-backlog/2026-04-23-memory-summarization-via-agent.md`
- Memory-api OpenAPI: `GET /api/v1/openapi.yaml` on any running
  memory-api pod, or browse the Scalar UI at `/docs`.
- Sample bundle: `config/samples/omnia_v1alpha1_memory_summarizer.yaml`
