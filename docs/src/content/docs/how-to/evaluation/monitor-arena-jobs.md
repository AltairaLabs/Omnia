---
title: "Monitor Arena jobs"
description: "Track Arena Fleet job progress and view evaluation results"
enterprise: true
sidebar:
  order: 12
---

This guide covers how to monitor Arena Fleet jobs in real-time, track progress, and access evaluation results.

## Checking job status

### Basic status

View the current state of an ArenaJob:

```bash
kubectl get arenajob my-eval
```

Output:

```
NAME      PHASE      PROGRESS   WORKERS   AGE
my-eval   Running    45/100     3/3       2m
```

### Detailed status

Get full status details:

```bash
kubectl get arenajob my-eval -o yaml
```

Key status fields:

```yaml
status:
  phase: Running
  progress:
    total: 100      # Total scenarios to evaluate
    completed: 45   # Successfully completed
    failed: 2       # Failed evaluations
    pending: 53     # Waiting to run
  activeWorkers: 3
  startTime: "2025-01-18T10:00:00Z"
  conditions:
    - type: Ready
      status: "True"
    - type: Progressing
      status: "True"
      message: "45/100 scenarios completed"
```

### Watch progress in real-time

Monitor job progress as it runs:

```bash
kubectl get arenajob my-eval -w
```

Or use `watch` for periodic updates:

```bash
watch -n 5 kubectl get arenajob my-eval
```

## Understanding job phases

| Phase | Description |
|-------|-------------|
| `Pending` | Job created, waiting to start |
| `Running` | Workers are actively processing scenarios |
| `Succeeded` | All scenarios completed successfully |
| `Failed` | Job failed (threshold exceeded or error) |
| `Cancelled` | Job was manually cancelled |

## Viewing worker status

### List worker pods

```bash
kubectl get pods -l arena.omnia.altairalabs.ai/job=my-eval
```

Output:

```
NAME                    READY   STATUS    RESTARTS   AGE
my-eval-worker-abc12    1/1     Running   0          2m
my-eval-worker-def34    1/1     Running   0          2m
my-eval-worker-ghi56    1/1     Running   0          2m
```

### View worker logs

Stream logs from all workers:

```bash
kubectl logs -l arena.omnia.altairalabs.ai/job=my-eval -f
```

Logs from a specific worker:

```bash
kubectl logs my-eval-worker-abc12 -f
```

### Check worker resource usage

```bash
kubectl top pods -l arena.omnia.altairalabs.ai/job=my-eval
```

## Accessing results

### From job status

For completed jobs, results are summarized in the status:

```bash
kubectl get arenajob my-eval -o jsonpath='{.status.result.summary}'
```

### Result URL

If output storage is configured, get the result location:

```bash
kubectl get arenajob my-eval -o jsonpath='{.status.result.url}'
```

### Download results

For S3 storage:

```bash
# Get the result prefix
RESULT_URL=$(kubectl get arenajob my-eval -o jsonpath='{.status.result.url}')
aws s3 cp $RESULT_URL/results.json ./results.json
```

For PVC storage:

```bash
# Port-forward or exec into a pod to access PVC
kubectl cp <pod>:/path/to/results ./results
```

## Prometheus metrics

Arena Fleet exposes metrics for monitoring with Prometheus.

### Key metrics

| Metric | Description |
|--------|-------------|
| `arena_job_phase` | Current job phase (gauge) |
| `arena_job_progress_total` | Total scenarios in job |
| `arena_job_progress_completed` | Completed scenarios |
| `arena_job_progress_failed` | Failed scenarios |
| `arena_job_duration_seconds` | Job execution duration |
| `arena_scenario_latency_seconds` | Per-scenario LLM latency |
| `arena_scenario_tokens_total` | Token usage per scenario |

### Example Prometheus queries

Total running jobs:

```promql
count(arena_job_phase{phase="Running"})
```

Job completion rate:

```promql
arena_job_progress_completed / arena_job_progress_total
```

Average scenario latency:

```promql
avg(arena_scenario_latency_seconds) by (job_name, provider)
```

Failed scenario rate:

```promql
rate(arena_job_progress_failed[5m])
```

## Grafana dashboard

If Grafana is enabled, Arena metrics are available for visualization.

### Sample dashboard panels

**Job Progress**:
```promql
arena_job_progress_completed{job_name="$job"}
```

**Scenario Latency Histogram**:
```promql
histogram_quantile(0.95, arena_scenario_latency_seconds_bucket)
```

**Token Usage Over Time**:
```promql
sum(rate(arena_scenario_tokens_total[5m])) by (provider)
```

## Event monitoring

View events related to Arena jobs:

```bash
kubectl get events --field-selector involvedObject.name=my-eval
```

Key events to watch for:

| Event | Meaning |
|-------|---------|
| `JobStarted` | Job execution began |
| `WorkersCreated` | Worker pods created |
| `ScenarioCompleted` | Individual scenario finished |
| `JobSucceeded` | Job completed successfully |
| `JobFailed` | Job failed |
| `RetryScheduled` | Failed scenario being retried |

## Setting up alerts

### Alert on job failure

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: arena-alerts
spec:
  groups:
    - name: arena
      rules:
        - alert: ArenaJobFailed
          expr: arena_job_phase{phase="Failed"} == 1
          for: 1m
          labels:
            severity: warning
          annotations:
            summary: "Arena job {{ $labels.job_name }} failed"
            description: "Job has been in Failed state for more than 1 minute"
```

### Alert on high failure rate

```yaml
- alert: ArenaHighFailureRate
  expr: |
    (arena_job_progress_failed / arena_job_progress_total) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Arena job {{ $labels.job_name }} has >10% failure rate"
```

### Alert on slow evaluations

```yaml
- alert: ArenaSlowEvaluation
  expr: |
    avg(arena_scenario_latency_seconds) by (job_name) > 60
  for: 10m
  labels:
    severity: info
  annotations:
    summary: "Arena job {{ $labels.job_name }} has slow evaluations (>60s avg)"
```

## Cancelling a job

Stop a running job:

```bash
kubectl delete arenajob my-eval
```

Or patch to cancel while preserving the resource:

```bash
kubectl patch arenajob my-eval --type=merge -p '{"spec":{"suspend":true}}'
```

## Debugging failed jobs

### Check job conditions

```bash
kubectl describe arenajob my-eval | grep -A 10 Conditions
```

### View failed scenarios

Check worker logs for failures:

```bash
kubectl logs -l arena.omnia.altairalabs.ai/job=my-eval | grep -i error
```

### Common failure reasons

| Reason | Resolution |
|--------|------------|
| `ConfigNotReady` | Verify the source bundle contains a valid `config.arena.yaml` |
| `SourceFetchFailed` | Verify ArenaSource can fetch bundle |
| `ProviderError` | Check provider credentials and limits |
| `Timeout` | Increase evaluation timeout |
| `AssertionFailed` | Expected behavior - check test assertions |

## Related resources

- **[Troubleshoot Arena](/how-to/evaluation/troubleshoot-arena/)**: Debug common issues
- **[ArenaJob Reference](/reference/evaluation/arenajob/)**: Complete status field documentation
- **[Set Up Observability](/how-to/observability/setup-observability/)**: Configure Prometheus and Grafana
