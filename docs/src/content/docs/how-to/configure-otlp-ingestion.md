---
title: "Ingest Traces from External Agents"
description: "Send OpenTelemetry GenAI traces from LangChain, OpenAI, AWS Bedrock, and other frameworks to Omnia"
---

Omnia's session-api includes an OTLP ingestion endpoint that accepts OpenTelemetry traces and converts them into session data. This lets any OTel-instrumented agent framework record conversations in Omnia without using the Omnia-specific WebSocket facade.

## Overview

```
External agents (LangChain, CrewAI, OpenAI SDK, AWS Bedrock, etc.)
    |
    |  OTLP/HTTP (JSON or Protobuf)  :4318/v1/traces
    |  OTLP/gRPC (Protobuf)          :4317
    |
    v
Session API  -->  Transformer  -->  Session Store (Postgres)
```

The endpoint supports:

- **OTLP/HTTP** with `application/json` or `application/x-protobuf` (recommended for external agents)
- **OTLP/gRPC** (recommended for in-cluster sidecars)
- All three generations of OTel GenAI semantic conventions (current, deprecated, and legacy OpenLLMetry)

## Enable OTLP Ingestion

OTLP is disabled by default. Enable it in your Helm values:

```yaml
sessionApi:
  otlp:
    enabled: true
    grpcPort: 4317  # standard OTLP gRPC port
    httpPort: 4318  # standard OTLP HTTP port
```

Deploy the update:

```bash
helm upgrade omnia oci://ghcr.io/altairalabs/omnia \
  --namespace omnia-system \
  -f values.yaml
```

The session-api pods will now expose two additional ports for OTLP ingestion.

## Send Traces from Python (OpenAI SDK)

The simplest way to test is with the OpenTelemetry Python SDK:

```bash
pip install opentelemetry-api opentelemetry-sdk \
  opentelemetry-exporter-otlp-proto-http
```

```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

resource = Resource.create({
    "service.name": "my-agent",
    "service.namespace": "default",
})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(
    endpoint="http://omnia-session-api.omnia-system:4318/v1/traces"
)
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

tracer = trace.get_tracer("my-agent")
with tracer.start_as_current_span("chat gpt-4") as span:
    span.set_attribute("gen_ai.conversation.id", "conv-123")
    span.set_attribute("gen_ai.request.model", "gpt-4")
    span.set_attribute("gen_ai.provider.name", "openai")
    span.set_attribute("gen_ai.usage.input_tokens", 150)
    span.set_attribute("gen_ai.usage.output_tokens", 75)

provider.shutdown()
```

## Send Traces from LangChain (via OpenLLMetry)

[OpenLLMetry](https://github.com/traceloop/openllmetry) is the most popular OpenTelemetry instrumentation for LLM frameworks. It auto-instruments LangChain, OpenAI, Anthropic, and more.

```bash
pip install traceloop-sdk
```

```python
from traceloop.sdk import Traceloop

Traceloop.init(
    app_name="my-langchain-agent",
    api_endpoint="http://omnia-session-api.omnia-system:4318",
    disable_batch=False,
)
```

OpenLLMetry uses the legacy indexed attribute format (`gen_ai.prompt.0.role`, `gen_ai.prompt.0.content`, etc.). Omnia supports this format natively.

> **Tip**: Set `TRACELOOP_TRACE_CONTENT=true` to include message content in traces. Without it, only token counts and metadata are captured.

## Send Traces from AWS Bedrock (via ADOT)

AWS Distro for OpenTelemetry (ADOT) works with the standard OTLP exporter. Configure the OTLP endpoint in your ADOT collector config:

```yaml
exporters:
  otlphttp:
    endpoint: "http://omnia-session-api.omnia-system:4318"

service:
  pipelines:
    traces:
      exporters: [otlphttp]
```

For Python agents using Bedrock with the OpenTelemetry SDK directly:

```python
import os
os.environ["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://omnia-session-api.omnia-system:4318"
os.environ["OTEL_SERVICE_NAME"] = "bedrock-agent"
```

AWS Bedrock emits `gen_ai.system = "aws.bedrock"` following the OTel GenAI semantic conventions.

## Session Identification

Omnia resolves a session ID from spans using this priority chain:

1. `gen_ai.conversation.id` (OTel GenAI convention)
2. `session.id` (span attribute)
3. `langfuse.session.id` (Langfuse convention)
4. `session.id` on the resource (resource attribute)
5. Trace ID hex string (always available as a fallback)

If your framework does not set a conversation ID, all spans within the same trace will be grouped into the same session automatically via the trace ID.

To explicitly group spans across multiple traces into one session, set `gen_ai.conversation.id`:

```python
span.set_attribute("gen_ai.conversation.id", "my-session-123")
```

## Supported Attribute Conventions

The transformer handles three generations of OTel GenAI attributes:

### Current OTel GenAI (v1.37+)

| Attribute | Purpose |
|-----------|---------|
| `gen_ai.provider.name` | Provider (openai, anthropic, aws.bedrock) |
| `gen_ai.request.model` | Requested model |
| `gen_ai.response.model` | Actual model used (preferred) |
| `gen_ai.input.messages` | Structured input messages (JSON array) |
| `gen_ai.output.messages` | Structured output messages (JSON array) |
| `gen_ai.usage.input_tokens` | Input token count |
| `gen_ai.usage.output_tokens` | Output token count |
| `gen_ai.conversation.id` | Session identifier |

### Legacy OpenLLMetry (most deployed)

| Attribute | Purpose |
|-----------|---------|
| `gen_ai.system` | Provider name (deprecated, maps to `gen_ai.provider.name`) |
| `gen_ai.prompt.{i}.role` | Message role at index i |
| `gen_ai.prompt.{i}.content` | Message content at index i |
| `gen_ai.completion.{i}.role` | Completion role at index i |
| `gen_ai.completion.{i}.content` | Completion content at index i |
| `gen_ai.usage.prompt_tokens` | Input tokens (deprecated name) |
| `gen_ai.usage.completion_tokens` | Output tokens (deprecated name) |

### Span Events

Some tools put message content in a span event instead of attributes. Omnia checks for the `gen_ai.client.inference.operation.details` event and extracts `gen_ai.input.messages` / `gen_ai.output.messages` from it.

## Data Mapping

| OTel Concept | Omnia Session Field |
|-------------|-------------------|
| Conversation ID / Trace ID | `session.id` |
| `service.name` resource attribute | `session.agentName` |
| `service.namespace` resource attribute | `session.namespace` |
| `gen_ai.provider.name` / `gen_ai.system` | `session.state["gen_ai.provider"]` |
| `gen_ai.response.model` / `gen_ai.request.model` | `session.state["gen_ai.model"]` and `message.metadata["gen_ai.model"]` |
| Input/output messages | `session.messages` (with role and content) |
| Token usage | `session.totalInputTokens` / `session.totalOutputTokens` |

## Exposing the Endpoint Externally

For agents running outside the cluster (e.g., in AWS Lambda, ECS, or a separate VPC), expose the OTLP HTTP endpoint via an ingress or load balancer:

```yaml
# Example: expose via a Kubernetes Ingress
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: omnia-otlp
  namespace: omnia-system
spec:
  rules:
    - host: otlp.example.com
      http:
        paths:
          - path: /v1/traces
            pathType: Exact
            backend:
              service:
                name: omnia-session-api
                port:
                  number: 4318
```

> **Important**: The OTLP endpoint does not currently require authentication. When exposing externally, protect it with a network policy, API gateway, or ingress authentication.

## Verifying Ingestion

After sending traces, verify sessions were created:

```bash
# List sessions via the session API
curl -s http://omnia-session-api.omnia-system:8080/api/v1/sessions | jq .

# Get a specific session
curl -s http://omnia-session-api.omnia-system:8080/api/v1/sessions/conv-123 | jq .
```

Sessions created via OTLP ingestion appear in the Omnia dashboard alongside sessions from native Omnia agents.

## Troubleshooting

### No sessions appearing

1. Check that `sessionApi.otlp.enabled` is `true` in your Helm values
2. Verify the session-api pods are running with OTLP ports:
   ```bash
   kubectl get pods -n omnia-system -l app.kubernetes.io/component=session-api
   kubectl logs -n omnia-system -l app.kubernetes.io/component=session-api
   ```
3. Confirm the OTLP port is reachable:
   ```bash
   kubectl port-forward -n omnia-system svc/omnia-session-api 4318:4318
   curl -X POST http://localhost:4318/v1/traces \
     -H "Content-Type: application/json" \
     -d '{}'
   ```

### Messages missing content

Most frameworks disable content capture by default to avoid sending PII. Enable it:

- **OpenLLMetry**: `TRACELOOP_TRACE_CONTENT=true`
- **OTel Python OpenAI**: `OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=true`

### Sessions not grouping correctly

If each span creates a separate session, ensure your agent sets `gen_ai.conversation.id` consistently across related spans. Without it, the trace ID is used, and each new trace creates a new session.
