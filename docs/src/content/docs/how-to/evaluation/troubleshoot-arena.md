---
title: "Troubleshoot Arena Fleet"
description: "Diagnose and resolve common Arena Fleet issues"
sidebar:
  order: 13
---

This guide helps you diagnose and resolve common issues with Arena Fleet evaluations.

## ArenaSource Issues

### Source Stuck in Pending

**Symptoms**: ArenaSource stays in `Pending` phase.

```bash
kubectl get arenasource my-source
# NAME        TYPE   PHASE     AGE
# my-source   git    Pending   5m
```

**Diagnosis**:

```bash
kubectl describe arenasource my-source
```

**Common Causes**:

1. **Invalid Git URL**:
   ```
   Message: failed to clone: repository not found
   ```
   - Verify the URL is correct and accessible
   - Check if the repository is private and needs credentials

2. **Missing credentials**:
   ```
   Message: authentication required
   ```
   - Create and reference a credentials secret
   - Verify the secret exists in the same namespace

3. **Network issues**:
   ```
   Message: dial tcp: lookup github.com: no such host
   ```
   - Check cluster DNS resolution
   - Verify network policies allow egress

**Resolution**:

```yaml
# For private repositories, add secretRef
spec:
  git:
    url: https://github.com/org/private-repo
    secretRef:
      name: git-credentials
```

### Source Fetch Errors

**Symptoms**: Source shows `Error` phase.

```bash
kubectl get arenasource my-source -o jsonpath='{.status.conditions}'
```

**Common Causes**:

1. **Invalid path**: The specified path doesn't exist in the repository
2. **Invalid ref**: Branch, tag, or commit doesn't exist
3. **Timeout**: Source fetch took too long

**Resolution**:

```yaml
# Verify the path and ref exist
spec:
  git:
    url: https://github.com/org/repo
    ref:
      branch: main  # Verify this branch exists
    path: prompts/  # Verify this path exists
  timeout: 120s     # Increase if needed
```

### ConfigMap Source Not Updating

**Symptoms**: ConfigMap changes don't trigger source updates.

**Cause**: ArenaSource watches ConfigMap `resourceVersion`, which changes on any modification.

**Resolution**: Ensure the ConfigMap is being modified:

```bash
# Check ConfigMap resourceVersion
kubectl get configmap my-prompts -o jsonpath='{.metadata.resourceVersion}'

# Force update by touching the ConfigMap
kubectl patch configmap my-prompts -p '{"metadata":{"annotations":{"updated":"'$(date +%s)'"}}}'
```

## Arena Config File Issues

There is **no `ArenaConfig` custom resource**. The Arena configuration is the
`config.arena.yaml` file **inside the ArenaSource bundle**, selected by
`ArenaJob.spec.arenaFile` (default `config.arena.yaml`). Configuration problems
therefore surface on the **ArenaSource** status (bundle fetch/validation) or in the
**worker pod logs** (config parse/validation at job start) — not on a separate resource.

### Config File Not Found or Invalid

**Symptoms**: The ArenaJob fails early; worker logs show `arena config file not found`
or a YAML parse error.

**Common Causes**:

1. **No config file in the bundle**: the bundle must contain a `config.arena.yaml`
   (or the file named by `spec.arenaFile`). A `pack.json`-only bundle is not enough.
   - Verify the file is present in the source bundle.

2. **Provider not found**:
   ```
   Message: Provider "claude-provider" not found
   ```
   - Verify the Provider CRD exists in the referenced namespace and is mapped into a
     provider group in the job's `spec.providers`.

3. **YAML syntax error** in `config.arena.yaml`:
   - Validate the file locally before packaging the bundle.

**Resolution**:

```bash
# Check the source is synced and valid
kubectl get arenasource my-source
kubectl describe arenasource my-source

# Check referenced providers exist
kubectl get provider claude-provider

# Inspect the worker logs for the parse/validation error
kubectl logs -l arena.omnia.altairalabs.ai/job=my-job | grep -i "config\|arena"
```

### Zero Scenarios Resolved

**Symptoms**: The job runs but executes `scenarioCount: 0`.

**Causes**:
- The ArenaJob's `spec.scenarios` include/exclude filters don't match any scenarios
- The bundle doesn't contain the referenced scenarios

**Resolution**:

```bash
# Inspect the ArenaJob's scenario filters
kubectl get arenajob my-job -o jsonpath='{.spec.scenarios}'

# Widen the filter to match everything
spec:
  scenarios:
    include:
      - "*"  # Include all scenarios
```

## ArenaJob Issues

### Job Stuck in Pending

**Symptoms**: Job stays in `Pending` phase.

**Diagnosis**:

```bash
kubectl describe arenajob my-job
kubectl get events --field-selector involvedObject.name=my-job
```

**Common Causes**:

1. **Source not ready**:
   ```
   Message: ArenaSource "my-source" is not ready
   ```
   - Fix the ArenaSource first (the bundle must sync before the job can start)

2. **Insufficient resources**:
   ```
   Message: 0/3 nodes are available: insufficient cpu
   ```
   - Reduce worker replicas or add cluster capacity

3. **Image pull errors**:
   ```
   Message: Failed to pull image "ghcr.io/altairalabs/arena-worker"
   ```
   - Check image pull secrets
   - Verify image exists

**Resolution**:

```yaml
# Reduce resource requirements
spec:
  workers:
    replicas: 1  # Start with fewer workers
```

### Workers Crash or Restart

**Symptoms**: Worker pods show `CrashLoopBackOff` or frequent restarts.

**Diagnosis**:

```bash
kubectl logs -l arena.omnia.altairalabs.ai/job=my-job --previous
kubectl describe pod <worker-pod-name>
```

**Common Causes**:

1. **Out of memory**:
   ```
   OOMKilled
   ```
   - Increase worker memory limits

2. **Provider errors**:
   ```
   Error: rate limit exceeded
   ```
   - Reduce concurrency
   - Check provider quota

3. **Invalid bundle**:
   ```
   Error: failed to parse pack.json
   ```
   - Validate your PromptKit bundle

**Resolution**:

```yaml
# Increase resources in Helm values
arena:
  worker:
    resources:
      limits:
        memory: 1Gi
      requests:
        memory: 512Mi
```

### High Failure Rate

**Symptoms**: Many scenarios failing during evaluation.

**Diagnosis**:

```bash
# Check job progress
kubectl get arenajob my-job -o jsonpath='{.status.progress}'

# View worker logs for errors
kubectl logs -l arena.omnia.altairalabs.ai/job=my-job | grep -i "error\|failed"
```

**Common Causes**:

1. **Assertion failures** (expected):
   - Review assertion definitions
   - Adjust expected values

2. **Provider rate limits**:
   ```
   Error: 429 Too Many Requests
   ```
   - Reduce concurrency
   - Add delays between requests

3. **Timeouts**:
   ```
   Error: context deadline exceeded
   ```
   - Increase evaluation timeout
   - Check for slow providers

**Resolution**:

```yaml
spec:
  sourceRef:
    name: my-source
  # Reduce concurrency and increase timeout
evaluation:
  timeout: "10m"
  concurrency: 1
  maxRetries: 5
```

### Results Not Stored

**Symptoms**: Job succeeds but no results in S3/PVC.

**Diagnosis**:

```bash
# Check job status for result URL
kubectl get arenajob my-job -o jsonpath='{.status.result}'

# Check worker logs for storage errors
kubectl logs -l arena.omnia.altairalabs.ai/job=my-job | grep -i "s3\|storage\|upload"
```

**Common Causes**:

1. **Missing credentials**: S3 secret not found or invalid
2. **Bucket doesn't exist**: Bucket must be pre-created
3. **Permission denied**: IAM policy doesn't allow writes

**Resolution**:

```bash
# Test S3 access from within cluster
kubectl run s3-test --rm -it --image=amazon/aws-cli -- \
  s3 ls s3://my-bucket/

# Check secret exists
kubectl get secret arena-s3-credentials -o yaml
```

## Queue Issues (Redis)

### Workers Not Processing

**Symptoms**: Job running but progress not advancing.

**Diagnosis**:

```bash
# Check Redis connectivity
kubectl exec -it <operator-pod> -- redis-cli -h omnia-redis-master ping

# Check queue depth
kubectl exec -it <operator-pod> -- redis-cli -h omnia-redis-master llen arena:queue:my-job
```

**Common Causes**:

1. **Redis not reachable**: Check Redis service and pods
2. **Queue configuration mismatch**: Workers and controller using different queues
3. **Redis authentication**: Password mismatch

**Resolution**:

```yaml
# Verify Redis configuration in Helm values
arena:
  queue:
    type: redis
    redis:
      host: "omnia-redis-master"
      port: 6379
```

## Controller Issues

### Controllers Not Reconciling

**Symptoms**: CRDs created but nothing happens.

**Diagnosis**:

```bash
# Check operator logs
kubectl logs -n omnia-system deployment/omnia-controller-manager | grep -i arena

# Check if Arena controllers are enabled
kubectl get deployment omnia-controller-manager -o yaml | grep -i arena
```

**Common Causes**:

1. **Arena not enabled**: Feature disabled in Helm values
2. **RBAC issues**: Controller missing permissions
3. **CRD not installed**: Arena CRDs not present

**Resolution**:

```yaml
# Enable Arena in Helm values
arena:
  enabled: true
```

```bash
# Verify CRDs exist
kubectl get crd arenasources.omnia.altairalabs.ai
kubectl get crd arenajobs.omnia.altairalabs.ai
```

## Debugging Commands Reference

### Quick Health Check

```bash
# Check all Arena resources
kubectl get arenasource,arenajob -A

# Check operator logs for errors
kubectl logs -n omnia-system deployment/omnia-controller-manager --tail=100 | grep -i "error\|arena"
```

### Verbose Debugging

```bash
# Enable debug logging (requires operator restart)
kubectl set env -n omnia-system deployment/omnia-controller-manager LOG_LEVEL=debug

# Stream all Arena-related logs
kubectl logs -n omnia-system deployment/omnia-controller-manager -f | grep -i arena
```

### Resource Cleanup

```bash
# Delete stuck jobs
kubectl delete arenajob --all

# Force delete with finalizer removal (use with caution)
kubectl patch arenajob my-job -p '{"metadata":{"finalizers":null}}' --type=merge
kubectl delete arenajob my-job
```

## Getting Help

If you're still experiencing issues:

1. Check the [Arena Fleet Architecture](/explanation/evaluation/arena-fleet/) for conceptual understanding
2. Review the references: [ArenaSource](/reference/evaluation/arenasource/), [Arena Config File](/reference/evaluation/arenaconfig/), [ArenaJob](/reference/evaluation/arenajob/)
3. Search or open an issue on [GitHub](https://github.com/AltairaLabs/Omnia/issues)
