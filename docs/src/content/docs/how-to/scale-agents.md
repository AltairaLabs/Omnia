---
title: "Scale Agent Deployments"
description: "Scale your agent deployments for production workloads"
sidebar:
  order: 3
---


This guide covers scaling strategies for Omnia agent deployments.

## Manual Scaling

### Set Replicas

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

## Automatic Scaling with HPA

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

### Check HPA Status

```bash
kubectl get hpa
kubectl describe hpa my-agent
```

## Advanced Scaling with KEDA

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

See [Autoscaling Explained](/explanation/autoscaling) for detailed KEDA configuration.

## Resource Configuration

### Set Resource Limits

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

### Resource Guidelines

| Workload | CPU Request | Memory Request |
|----------|-------------|----------------|
| Light | 250m | 128Mi |
| Medium | 500m | 256Mi |
| Heavy | 1000m | 512Mi |

## Session Affinity

When using multiple replicas, ensure session affinity:

### With Redis Sessions (Recommended)

Redis-backed sessions work seamlessly with any replica:

```yaml
spec:
  session:
    type: redis
    storeRef:
      name: redis-credentials
```

### With Memory Sessions

If using memory sessions (not recommended for production), configure service affinity:

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

## Monitoring Scale

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

## Next Steps

- [Autoscaling Explained](/explanation/autoscaling) - Deep dive into HPA vs KEDA
- [Set Up Observability](/how-to/setup-observability) - Monitor scaling metrics
- [AgentRuntime Reference](/reference/agentruntime) - Full autoscaling configuration
