---
title: "Configure OpenRouter Provider"
description: "Use OpenRouter as a unified gateway to Anthropic, OpenAI, Google, Meta and other models via Omnia Provider custom headers"
sidebar:
  order: 19
---

[OpenRouter](https://openrouter.ai) is a unified gateway that exposes 100+ LLMs behind a single OpenAI-compatible API — Claude, GPT, Gemini, Llama, Mistral, and others — with one API key and one endpoint.

Omnia's `Provider` custom resource treats OpenRouter like any other OpenAI-compatible endpoint. Two Omnia features combine to make this work:

1. **`spec.baseURL`** — point the OpenAI wire format at OpenRouter.
2. **`spec.headers`** — set the `HTTP-Referer` and `X-Title` attribution headers that OpenRouter uses for leaderboards and rate-limit tiers.

## Prerequisites

- An [OpenRouter account](https://openrouter.ai) with an API key (create one at [openrouter.ai/keys](https://openrouter.ai/keys)).
- Omnia operator installed in the cluster.

## 1. Create a Secret for the API key

```bash
kubectl create secret generic openrouter-credentials \
  --namespace agents \
  --from-literal=OPENAI_API_KEY='sk-or-v1-...'
```


## 2. Create the Provider

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openrouter
  namespace: agents
spec:
  type: openai
  model: anthropic/claude-sonnet-4                # OpenRouter model ID
  baseURL: https://openrouter.ai/api/v1

  headers:
    HTTP-Referer: https://your-app.example.com    # Required for leaderboard attribution
    X-Title: omnia                                # Appears in OpenRouter analytics

  credential:
    secretRef:
      name: openrouter-credentials

  capabilities:
    - text
    - streaming
    - tools
    - json
```

### How the headers reach the wire

When the runtime creates the underlying PromptKit provider, `spec.headers` is passed through `providers.ProviderSpec.Headers`. PromptKit's `openai` provider (which `type: openai` resolves to) applies custom headers to every `/chat/completions` request via its `ApplyCustomHeaders` helper — including streaming and tool-calling requests. Collisions with built-in headers (e.g. `Authorization`) are rejected at request time so you can't accidentally break auth.

## 3. Verify the Provider is Ready

```bash
kubectl get provider openrouter -n agents -o wide
kubectl get provider openrouter -n agents -o jsonpath='{.status.conditions}' | jq .
```

Both `SecretFound` and `CredentialConfigured` conditions should be `True`. OpenRouter does not respond to unauthenticated `GET /` requests, so the `EndpointReachable` condition may be omitted or show a 401 — either is considered reachable (a 401 proves the endpoint is up).

## 4. Using with AgentRuntime

Reference the Provider from an `AgentRuntime` like any other:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: my-agent
  namespace: agents
spec:
  promptPackRef:
    name: my-pack
  providers:
    - name: default
      providerRef:
        name: openrouter
  facade:
    type: websocket
```

## Choosing a model

OpenRouter's model IDs follow the `<vendor>/<model>` pattern. Examples:

| Omnia `spec.model`                   | Hosted model                       |
|--------------------------------------|------------------------------------|
| `anthropic/claude-sonnet-4`          | Anthropic Claude Sonnet 4          |
| `openai/gpt-4o`                      | OpenAI GPT-4o                      |
| `google/gemini-2.0-flash-exp`        | Google Gemini 2.0 Flash            |
| `meta-llama/llama-3.1-70b-instruct`  | Meta Llama 3.1 70B                 |

Full list: [openrouter.ai/models](https://openrouter.ai/models).

## Custom pricing (optional)

OpenRouter's pricing can differ from the first-party API. Supply `spec.pricing` if you want Omnia's cost tracking to reflect OpenRouter's actual rates:

```yaml
spec:
  pricing:
    inputCostPer1K: "0.003"
    outputCostPer1K: "0.015"
```

If omitted, PromptKit falls back to built-in pricing for the underlying model family (e.g. Anthropic's public list prices for a `claude-sonnet-4` request). For production cost tracking against OpenRouter invoices, set the pricing explicitly.

## Troubleshooting

**401 Unauthorized on every request.** Check that the secret value uses an `sk-or-v1-...` OpenRouter key — a direct-OpenAI `sk-...` key will not work against `openrouter.ai`. Verify with `kubectl get secret openrouter-credentials -n agents -o jsonpath='{.data.OPENAI_API_KEY}' | base64 -d | head -c 12`.

**Rate limits / attribution missing.** OpenRouter's free tier and leaderboards key off the `HTTP-Referer` and `X-Title` headers. If you omit them, requests still succeed but you'll hit the unattributed rate limit and your app won't appear on the leaderboard.

**`headers map is immutable` error at request time.** This is PromptKit rejecting a custom header that collides with one the provider sets itself (most commonly `Authorization`). Remove that key from `spec.headers`.

## See also

- [Provider CRD reference — `spec.headers`](/reference/provider/#headers) — full field semantics and collision rules.
- [OpenRouter docs — Quickstart](https://openrouter.ai/docs/quickstart) — OpenRouter's own getting-started.
