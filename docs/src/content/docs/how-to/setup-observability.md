---
title: "Set Up Observability"
description: "Deploy the integrated observability stack for monitoring agents"
sidebar:
  order: 4
---


Omnia includes an optional observability stack with Prometheus, Grafana, Loki, and Tempo for comprehensive monitoring of your agent deployments.

## Prerequisites

- Kubernetes cluster with Helm 3.x
- Omnia Helm chart installed

## Enable the Observability Stack

The observability components are disabled by default. Enable them in your Helm values:

```yaml
prometheus:
  enabled: true

grafana:
  enabled: true

loki:
  enabled: true

tempo:
  enabled: true

alloy:
  enabled: true
```

Install or upgrade with these values:

```bash
helm upgrade --install omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  --create-namespace \
  -f values.yaml
```

## Access Grafana

### Port Forward

For development, port-forward to access Grafana:

```bash
kubectl port-forward svc/omnia-grafana 3000:80 -n omnia-system
```

Open http://localhost:3000 and log in with:
- Username: `admin`
- Password: `admin` (change this in production)

### Via Internal Gateway

If you've enabled the internal gateway (with Istio), Grafana is available at `/grafana`:

```bash
kubectl get gateway omnia-internal -n omnia-system -o jsonpath='{.status.addresses[0].value}'
```

Then access `http://<gateway-ip>:8080/grafana/`

## View Agent Metrics

Omnia agents expose Prometheus metrics automatically. Key metrics include:

| Metric | Type | Description |
|--------|------|-------------|
| `omnia_agent_connections_active` | Gauge | Current WebSocket connections |
| `omnia_agent_connections_total` | Counter | Total connections since startup |
| `omnia_agent_requests_inflight` | Gauge | Pending LLM requests |
| `omnia_agent_request_duration_seconds` | Histogram | Request latency |
| `omnia_agent_messages_received_total` | Counter | Messages received |
| `omnia_agent_messages_sent_total` | Counter | Messages sent |

### Query Metrics in Grafana

1. Open Grafana and go to **Explore**
2. Select the **Prometheus** datasource
3. Try these queries:

```text
omnia_agent_connections_active

rate(omnia_agent_requests_total[5m])

histogram_quantile(0.95, rate(omnia_agent_request_duration_seconds_bucket[5m]))
```

## View Agent Logs

Logs are collected by Alloy and stored in Loki.

### Query Logs in Grafana

1. Open Grafana and go to **Explore**
2. Select the **Loki** datasource
3. Use LogQL queries:

```text
{namespace="omnia-system", container="agent"}

{namespace="omnia-system"} |= "error"

{namespace="omnia-system", app_name="my-agent"}
```

## Agent Tracing with OpenTelemetry

The runtime container supports OpenTelemetry tracing for detailed visibility into conversations, LLM calls, and tool executions.

### Enable Tracing

Tracing is configured via environment variables on the AgentRuntime. The operator will pass these to the runtime container:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
spec:
  # ... other config ...
  runtime:
    env:
      - name: OMNIA_TRACING_ENABLED
        value: "true"
      - name: OMNIA_TRACING_ENDPOINT
        value: "tempo.omnia-system.svc.cluster.local:4317"
      - name: OMNIA_TRACING_SAMPLE_RATE
        value: "1.0"
      - name: OMNIA_TRACING_INSECURE
        value: "true"
```

### Tracing Configuration Options

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `OMNIA_TRACING_ENABLED` | Enable OpenTelemetry tracing | `false` |
| `OMNIA_TRACING_ENDPOINT` | OTLP collector endpoint (gRPC) | - |
| `OMNIA_TRACING_SAMPLE_RATE` | Sampling rate (0.0 to 1.0) | `1.0` |
| `OMNIA_TRACING_INSECURE` | Disable TLS for OTLP connection | `false` |

### Span Types

The runtime creates three types of spans:

**Conversation Spans** (`conversation.turn`)
- Created for each message exchange
- Includes session ID, message length, response length
- Parent span for LLM and tool spans

**LLM Spans** (`llm.call`)
- Created for each LLM API call
- Includes model name, token counts (input/output), cost

**Tool Spans** (`tool.<name>`)
- Created for each tool execution
- Includes tool name, success/error status, result size

### Trace Attributes

Traces include rich metadata for debugging:

| Attribute | Description |
|-----------|-------------|
| `omnia.session_id` | Conversation session identifier |
| `llm.model` | LLM model used |
| `llm.input_tokens` | Input token count |
| `llm.output_tokens` | Output token count |
| `llm.cost_usd` | Estimated cost in USD |
| `tool.name` | Tool that was called |
| `tool.is_error` | Whether tool returned an error |
| `tool.result_size` | Size of tool result |

### View Traces in Tempo

Tempo collects distributed traces from agents.

### Query Traces in Grafana

1. Open Grafana and go to **Explore**
2. Select the **Tempo** datasource
3. Search by:
   - Service name (e.g., `omnia-runtime-my-agent`)
   - Trace ID
   - Duration
   - Tags (e.g., `omnia.session_id`)

### Example Trace Query

Find slow conversations:

```text
{ duration > 5s && resource.service.name =~ "omnia-runtime.*" }
```

Find tool errors:

```text
{ span.tool.is_error = true }
```

## Production Considerations

### Persistent Storage

Enable persistent storage for production:

```yaml
prometheus:
  server:
    persistentVolume:
      enabled: true
      size: 50Gi

loki:
  singleBinary:
    persistence:
      enabled: true
      size: 50Gi

tempo:
  persistence:
    enabled: true
    size: 10Gi
```

### Change Grafana Password

```yaml
grafana:
  adminPassword: your-secure-password
```

Or use a secret:

```yaml
grafana:
  admin:
    existingSecret: grafana-admin-secret
    userKey: admin-user
    passwordKey: admin-password
```

### Resource Limits

Adjust resources based on your cluster size:

```yaml
prometheus:
  server:
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 1000m
        memory: 1Gi

grafana:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
```

## Disable Individual Components

You can enable only the components you need:

```yaml
prometheus:
  enabled: true
grafana:
  enabled: true
loki:
  enabled: false
tempo:
  enabled: false
alloy:
  enabled: false
```

## Use External Observability

If you have existing observability infrastructure, disable the subcharts and configure agents to export to your systems:

```yaml
prometheus:
  enabled: false
grafana:
  enabled: false
loki:
  enabled: false
tempo:
  enabled: false
```

Agent pods include Prometheus scrape annotations by default, so your existing Prometheus can scrape them automatically.
