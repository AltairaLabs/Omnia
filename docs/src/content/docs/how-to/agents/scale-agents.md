---
title: "Scale agent deployments"
description: "Scale your agent deployments for production workloads"
sidebar:
  order: 3
---


This guide covers scaling strategies for Omnia agent deployments.

## Manual scaling

### Set replicas

Scale by adjusting the `runtime.replicas` field:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  runtime:
    replicas: 3
  # ...
```

Or use kubectl:

```bash
kubectl patch agentruntime my-agent --type=merge \
  -p '{"spec":{"runtime":{"replicas":5}}}'
```

## Automatic scaling with HPA

Enable built-in HPA autoscaling:

```yaml
spec:
  runtime:
    autoscaling:
      enabled: true
      type: hpa
      minReplicas: 2
      maxReplicas: 10
      targetMemoryUtilizationPercentage: 70
      targetCPUUtilizationPercentage: 90
```

The HPA automatically adjusts replicas based on resource utilization.

### Check HPA status

```bash
kubectl get hpa
kubectl describe hpa my-agent
```

## Advanced scaling with KEDA

For custom metrics and scale-to-zero capabilities, use KEDA:

```yaml
spec:
  runtime:
    autoscaling:
      enabled: true
      type: keda
      minReplicas: 1
      maxReplicas: 20
      keda:
        pollingInterval: 30
        cooldownPeriod: 300
        triggers:
          - type: prometheus
            metadata:
              serverAddress: "http://prometheus:9090"
              query: 'sum(omnia_agent_connections_active{agent="my-agent"})'
              threshold: "10"
```

See [Autoscaling Explained](/explanation/agents/autoscaling) for detailed KEDA configuration.

## Resource configuration

### Set resource limits

Configure CPU and memory for predictable performance:

```yaml
spec:
  runtime:
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "1000m"
        memory: "512Mi"
```

### Resource guidelines

| Workload | CPU Request | Memory Request |
|----------|-------------|----------------|
| Light | 250m | 128Mi |
| Medium | 500m | 256Mi |
| Heavy | 1000m | 512Mi |

## Session affinity

When using multiple replicas, ensure session affinity:

### With Redis context store (recommended)

Redis-backed context store works with any replica count:

```yaml
spec:
  context:
    type: redis
    storeRef:
      name: redis-credentials
```

### With memory context store

If using memory context store (not recommended for production), configure service affinity:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-agent
spec:
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 3600
```

## Monitoring scale

Check replica status:

```bash
kubectl get agentruntime my-agent -o wide
```

View status conditions:

```bash
kubectl describe agentruntime my-agent
```

View autoscaling metrics:

```bash
kubectl get hpa my-agent

kubectl get scaledobject my-agent
kubectl get hpa keda-hpa-my-agent
```

## Next steps

- [Autoscaling Explained](/explanation/agents/autoscaling) - Deep dive into HPA vs KEDA
- [Set Up Observability](/how-to/observability/setup-observability) - Monitor scaling metrics
- [AgentRuntime Reference](/reference/core/agentruntime) - Full autoscaling configuration
