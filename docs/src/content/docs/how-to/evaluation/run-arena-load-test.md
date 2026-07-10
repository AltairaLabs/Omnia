---
title: "Run an Arena Load Test"
description: "Drive concurrent load at agents and providers with virtual users, ramps, budgets, and SLO gating"
sidebar:
  order: 10
---

:::note[Enterprise Feature]
Arena is part of Omnia Enterprise. It requires a valid license — see [Install a License](/how-to/operations/install-license/).
:::

This guide shows how to run a load test with Arena. A load-test `ArenaJob` replays your scenarios at controlled concurrency against LLM providers or full agents, evaluates the results against SLO thresholds, and can fail the job for CI/CD gating.

Load tests share the same building blocks as evaluation jobs — an `ArenaSource` for scenarios and providers, a worker pool, and a results destination. The difference is `spec.type: loadtest` and the `spec.loadTest` settings block, which turns the scenario × provider matrix into a sustained, concurrency-controlled workload.

## Minimal Load Test

Set the job type to `loadtest` and add a `loadTest` block:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: checkout-loadtest
  namespace: default
spec:
  sourceRef:
    name: checkout-config
  type: loadtest
  trials: 500              # total load volume (see "How load volume is computed")
  workers:
    replicas: 4            # 4 worker pods
  loadTest:
    concurrency: 20        # at most 20 work items in flight at once
    vusPerWorker: 8        # 8 virtual users (goroutines) per worker pod
    thresholds:
      - metric: latency_avg
        operator: "<"
        value: "3s"
      - metric: error_rate
        operator: "<"
        value: "0.01"
```

## How Load Volume Is Computed

Arena partitions work into discrete **work items** — one per `scenario × provider × trial` combination. The total number of work items is the load volume for the run:

```
work items = scenarios × providers × trials
```

`spec.trials` sets the trial count for load tests (overriding any per-scenario `trials` in the scenario YAML). For a load test, treat `trials` as the total volume you want to push through under concurrency control, not a statistical-confidence knob. Use [`spec.scenarios`](/reference/evaluation/arenajob/) include/exclude filters to control which scenarios contribute, and `spec.providers` to control which providers each scenario runs against.

## Virtual Users and Concurrency

Two settings control how hard Arena pushes, and they operate at different levels:

| Setting | Level | Meaning |
|---------|-------|---------|
| `workers.replicas` | Pods | Number of worker pods that pull from the shared queue |
| `loadTest.vusPerWorker` | Per pod | Virtual users (concurrent goroutines) inside each worker pod. Each VU independently pops, executes, and reports a work item |
| `loadTest.concurrency` | Global | Ceiling on the number of work items in flight across **all** workers combined |

The raw parallelism a run *could* reach is `replicas × vusPerWorker`, but `concurrency` is a global cap that every VU checks before popping a new item. Set `concurrency` to the actual in-flight load you want to sustain, then provide enough `replicas × vusPerWorker` to reach it. If `concurrency` is lower than the available VUs, the extra VUs idle until an in-flight item completes — this is the intended back-pressure mechanism.

Both `concurrency` and `vusPerWorker` default to `1` (a serial run).

## Ramp Up and Ramp Down

By default the test jumps straight to full concurrency. Add a `ramp` block to change concurrency linearly over time:

```yaml
spec:
  loadTest:
    concurrency: 50
    ramp:
      up: "2m"     # linearly ramp 0 → 50 over the first 2 minutes
      down: "30s"  # linearly ramp 50 → 0 at the end
```

- **`ramp.up`** — the duration over which concurrency climbs from 0 to the target at the start of the run. Use it to warm caches and avoid a thundering-herd spike.
- **`ramp.down`** — the duration over which concurrency falls back to 0. Ramp-down is triggered automatically when the remaining pending items drop below `concurrency × 2`.

Both are Go duration strings (e.g. `"90s"`, `"5m"`).

## SLO Thresholds for CI/CD Gating

`loadTest.thresholds` define SLO targets that are evaluated **after** the load test finishes. If any threshold is violated the ArenaJob transitions to `Failed`, which lets you gate a pipeline on load-test results.

Each threshold has a `metric`, an `operator` (`<`, `>`, `<=`, `>=`), and a target `value`:

```yaml
spec:
  loadTest:
    thresholds:
      - metric: latency_avg
        operator: "<"
        value: "3s"        # duration string for latency/ttft metrics
      - metric: error_rate
        operator: "<"
        value: "0.01"      # ratio 0..1 for rate metrics
      - metric: pass_rate
        operator: ">="
        value: "0.95"
      - metric: total_cost
        operator: "<"
        value: "50.00"     # numeric string for cost metrics
```

### Supported metrics

| Metric | Kind | Notes |
|--------|------|-------|
| `latency_avg` | Latency | Average end-to-end turn latency |
| `latency_p50` / `p90` / `p95` / `p99` | Latency | Latency percentiles |
| `ttft_avg` | Latency | Average time-to-first-token |
| `ttft_p50` / `p90` / `p95` / `p99` | Latency | Time-to-first-token percentiles |
| `error_rate` | Rate (0..1) | Failed items ÷ total items |
| `pass_rate` | Rate (0..1) | Passed items ÷ total items |
| `rate_limit_rate` | Rate (0..1) | Share of requests that hit provider rate limits |
| `total_cost` | Cost | Accumulated cost in `budgetCurrency` |

**Value formats:** latency and TTFT metrics take a duration string (`"3s"`, `"500ms"`); rate metrics take a float string (`"0.01"`, `"0.95"`); cost takes a numeric string (`"50.00"`).

:::caution
Percentile (`_p50`…`_p99`) and TTFT thresholds cannot be computed from the job's running counters. When a metric is unavailable it is reported as `unavailable` and treated as **passing** rather than failing the job — so a percentile threshold will not block a run today. `latency_avg`, `error_rate`, `pass_rate`, and `total_cost` are computed from the accumulated stats and are enforced. Ground your CI gates on those four.
:::

## Budget Limits

Long or high-concurrency runs against paid providers can accumulate real spend. Set a hard budget ceiling so the controller cancels remaining work once the accumulated cost crosses the limit:

```yaml
spec:
  loadTest:
    budgetLimit: "25.00"
    budgetCurrency: "USD"   # default is USD
```

The controller checks the cost accumulator periodically and stops the job when `budgetLimit` is exceeded. This is a safety cap on total spend, distinct from a `total_cost` threshold — the threshold *fails* the job after the fact for gating, while `budgetLimit` *stops* it mid-flight to cap spend.

## Targeting Agents vs. Providers

Each entry in `spec.providers` can reference either a `Provider` CRD or an `AgentRuntime`:

- **`providerRef`** — drives load directly at the LLM provider.
- **`agentRef`** — drives load at a full agent. The worker connects to the agent's facade over WebSocket ("fleet mode"), so the load exercises the entire agent pipeline (prompt assembly, tools, guardrails), not just the raw model. Time-to-first-token and latency reflect the agent's real behaviour.

Agents and providers are interchangeable in the scenario × provider matrix, so a single job can load-test both an agent and its underlying model side by side. Multi-turn conversations come from the scenario definitions themselves — each scenario supplies the user turns that are replayed against the target.

## Session Recording

By default a load test does **not** write session data. `spec.sessionRecording` is `false`, so no facade sessions are created and no events are recorded — this keeps session-api out of the hot path during high-volume runs. Telemetry and traces are unaffected either way.

When you *do* want load-test runs to land as real sessions — for example to inspect individual conversations in the dashboard — set `sessionRecording: true`:

```yaml
spec:
  type: loadtest
  sessionRecording: true
  # ...
```

With recording enabled, each fleet-mode conversation is assigned a facade session ID and its turns are unified into the same session store the dashboard reads, so a load-test run's conversations show up alongside ordinary agent traffic. Leave it off for pure throughput/SLO runs where per-conversation detail isn't needed.

## Watching a Run

Load-test jobs surface progress through the same `ArenaJob` status as evaluation jobs:

```bash
kubectl get arenajob checkout-loadtest
kubectl get arenajob checkout-loadtest -o yaml
```

The job reaches `Succeeded` when all thresholds pass and `Failed` when any enforced threshold is violated (or a hard error occurs). See [Monitor Arena Jobs](/how-to/evaluation/monitor-arena-jobs/) for tracking progress and reading results, and the [ArenaJob reference](/reference/evaluation/arenajob/) for the full field list.

## See Also

- [ArenaJob reference](/reference/evaluation/arenajob/) — complete `spec.loadTest` field reference
- [Monitor Arena Jobs](/how-to/evaluation/monitor-arena-jobs/) — track progress and results
- [Set Up Arena Scheduled Jobs](/how-to/evaluation/setup-arena-scheduled-jobs/) — run recurring load tests
- [Troubleshoot Arena Fleet](/how-to/evaluation/troubleshoot-arena/) — diagnose stuck or failing jobs
