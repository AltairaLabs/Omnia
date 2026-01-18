---
title: "Set Up Arena Scheduled Jobs"
description: "Configure recurring Arena Fleet evaluations using cron schedules"
sidebar:
  order: 11
  badge:
    text: Arena
    variant: note
---

This guide shows how to set up scheduled Arena Fleet evaluations that run automatically on a recurring basis, such as nightly regression tests or weekly performance benchmarks.

## Basic Scheduled Job

Add a `schedule` section to your ArenaJob to enable recurring execution:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: nightly-regression
  namespace: default
spec:
  configRef:
    name: regression-config
  type: evaluation
  workers:
    replicas: 2
  schedule:
    cron: "0 2 * * *"  # Run at 2:00 AM daily
    timezone: "UTC"
```

## Cron Expression Reference

The `cron` field uses standard cron syntax:

```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, Sunday=0)
│ │ │ │ │
* * * * *
```

### Common Schedules

| Schedule | Cron Expression | Description |
|----------|-----------------|-------------|
| Nightly | `0 2 * * *` | Every day at 2:00 AM |
| Hourly | `0 * * * *` | Every hour on the hour |
| Every 6 hours | `0 */6 * * *` | At 0:00, 6:00, 12:00, 18:00 |
| Weekly | `0 3 * * 0` | Sunday at 3:00 AM |
| Weekdays | `0 8 * * 1-5` | Monday-Friday at 8:00 AM |
| Monthly | `0 4 1 * *` | 1st of each month at 4:00 AM |

## Timezone Configuration

By default, schedules use UTC. Specify a timezone for local time scheduling:

```yaml
spec:
  schedule:
    cron: "0 2 * * *"
    timezone: "America/New_York"  # 2:00 AM Eastern Time
```

Common timezones:
- `UTC` (default)
- `America/New_York`
- `America/Los_Angeles`
- `Europe/London`
- `Asia/Tokyo`

## Concurrency Policy

Control what happens when a scheduled run would start while a previous run is still active:

```yaml
spec:
  schedule:
    cron: "0 * * * *"
    concurrencyPolicy: Forbid  # Default
```

| Policy | Behavior |
|--------|----------|
| `Forbid` | Skip the new run if previous is still active (default) |
| `Allow` | Run concurrently with previous run |
| `Replace` | Cancel the previous run and start new one |

### When to Use Each Policy

**Forbid** (recommended for most cases):
- Prevents resource contention
- Ensures results don't overlap
- Safe default for evaluations

**Allow**:
- Use when runs are independent
- When you need guaranteed execution of every scheduled run
- Ensure sufficient cluster resources

**Replace**:
- When only the latest results matter
- For long-running jobs that may need to be superseded
- Use with caution as it cancels in-progress work

## Complete Example: Nightly Regression Suite

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: nightly-regression
  namespace: arena
  labels:
    team: platform
    environment: staging
spec:
  configRef:
    name: regression-suite
  type: evaluation

  evaluation:
    outputFormats:
      - json
      - junit

  workers:
    replicas: 5

  output:
    type: s3
    s3:
      bucket: arena-results
      prefix: "regression/nightly/"
      region: us-west-2
      secretRef:
        name: s3-credentials

  schedule:
    cron: "0 2 * * *"      # 2:00 AM daily
    timezone: "UTC"
    concurrencyPolicy: Forbid

  ttlSecondsAfterFinished: 604800  # Keep results for 7 days
```

## Monitoring Scheduled Jobs

### View Schedule Status

```bash
kubectl get arenajob nightly-regression -o yaml
```

The status shows scheduling information:

```yaml
status:
  phase: Succeeded
  lastScheduleTime: "2025-01-18T02:00:00Z"
  nextScheduleTime: "2025-01-19T02:00:00Z"
```

### List Recent Runs

Scheduled jobs create child jobs for each execution. View recent runs:

```bash
kubectl get arenajobs -l arena.omnia.altairalabs.ai/parent=nightly-regression
```

### Check for Missed Runs

If a scheduled run was missed (e.g., controller was down), it will be noted in conditions:

```bash
kubectl describe arenajob nightly-regression | grep -A 5 Conditions
```

## Suspending Scheduled Jobs

Temporarily pause a scheduled job without deleting it:

```bash
kubectl patch arenajob nightly-regression --type=merge -p '{"spec":{"suspend":true}}'
```

Resume the schedule:

```bash
kubectl patch arenajob nightly-regression --type=merge -p '{"spec":{"suspend":false}}'
```

Or in the manifest:

```yaml
spec:
  suspend: true  # Pauses scheduling
  schedule:
    cron: "0 2 * * *"
```

## Multiple Schedules

To run the same evaluation on different schedules (e.g., quick checks hourly, full suite nightly), create separate ArenaJobs:

```yaml
# Quick hourly check
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: quick-check-hourly
spec:
  configRef:
    name: quick-check-config  # Subset of scenarios
  schedule:
    cron: "0 * * * *"
  workers:
    replicas: 1
---
# Full nightly suite
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: full-suite-nightly
spec:
  configRef:
    name: full-suite-config  # All scenarios
  schedule:
    cron: "0 2 * * *"
  workers:
    replicas: 10
```

## Integration with CI/CD

### Triggering Manual Runs

Force an immediate run of a scheduled job:

```bash
# Create a one-time job from the same config
kubectl create -f - <<EOF
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  generateName: manual-regression-
  namespace: arena
spec:
  configRef:
    name: regression-suite
  type: evaluation
  workers:
    replicas: 5
EOF
```

### Alerting on Failures

Configure alerts for scheduled job failures using Prometheus alerts:

```yaml
# prometheus-rules.yaml
groups:
  - name: arena-alerts
    rules:
      - alert: ArenaScheduledJobFailed
        expr: |
          arena_job_status{scheduled="true", phase="Failed"} == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Scheduled Arena job {{ $labels.job_name }} failed"
```

## Cleanup and Retention

### Automatic Cleanup with TTL

Set `ttlSecondsAfterFinished` to automatically delete completed jobs:

```yaml
spec:
  ttlSecondsAfterFinished: 604800  # 7 days
  schedule:
    cron: "0 2 * * *"
```

### Manual Cleanup

Delete old job runs:

```bash
# Delete jobs older than 7 days
kubectl delete arenajobs -l arena.omnia.altairalabs.ai/parent=nightly-regression \
  --field-selector 'status.completionTime<2025-01-11T00:00:00Z'
```

## Related Resources

- **[ArenaJob Reference](/reference/arenajob/#schedule)**: Complete schedule configuration options
- **[Monitor Arena Jobs](/how-to/monitor-arena-jobs/)**: Track scheduled job execution
- **[Configure S3 Storage](/how-to/configure-arena-s3-storage/)**: Store results from scheduled runs
