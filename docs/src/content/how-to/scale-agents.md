---
title: "Scale Agent Deployments"
description: "Scale your agent deployments for production workloads"
order: 3
---

# Scale Agent Deployments

This guide covers scaling strategies for Omnia agent deployments.

## Horizontal Scaling

### Increase Replicas

Scale by adjusting the `replicas` field:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  replicas: 3  # Run 3 agent instances
  # ...
```

Or use kubectl:

```bash
kubectl scale agentruntime my-agent --replicas=5
```

### Horizontal Pod Autoscaler

Create an HPA for automatic scaling:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-agent-hpa
spec:
  scaleTargetRef:
    apiVersion: omnia.altairalabs.ai/v1alpha1
    kind: AgentRuntime
    name: my-agent
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

## Resource Configuration

### Set Resource Limits

Configure CPU and memory for predictable performance:

```yaml
spec:
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

### With Redis Sessions

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

## Load Balancing

The agent Service automatically load balances across replicas. For WebSocket connections, consider:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-agent
  annotations:
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
    nginx.ingress.kubernetes.io/upstream-hash-by: "$remote_addr"
spec:
  rules:
    - host: agent.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-agent
                port:
                  number: 8080
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
