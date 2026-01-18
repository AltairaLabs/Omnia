---
title: "Arena Fleet: Run Your First Evaluation"
description: "Get started with Arena Fleet by running your first prompt evaluation"
sidebar:
  order: 4
  badge:
    text: Arena
    variant: note
---

This tutorial walks you through running your first prompt evaluation using Arena Fleet. By the end, you'll have a complete evaluation pipeline running in your cluster.

## Prerequisites

Before you begin, ensure you have:

- A Kubernetes cluster with Omnia installed
- `kubectl` configured to access your cluster
- An LLM provider configured (or use the demo Ollama setup)

:::tip[No API Key? Use Ollama]
If you don't have an API key, you can use the `omnia-demos` chart which includes a local Ollama instance:

```bash
helm install omnia-demos oci://ghcr.io/altairalabs/omnia-demos \
  -n omnia-demo --create-namespace \
  --set arenaDemo.enabled=true
```

This deploys a complete Arena demo with sample evaluations using Ollama. Skip to [Step 5](#step-5-monitor-the-job) to see results.
:::

## Overview

Arena Fleet evaluates prompts through three CRDs:

```
ArenaSource → ArenaConfig → ArenaJob → Results
    │              │            │
    │              │            └── Executes the evaluation
    │              └── Defines what to test and how
    └── Fetches your PromptKit bundle
```

## Step 1: Create an ArenaSource

An ArenaSource defines where to fetch your PromptKit bundle from. For this tutorial, we'll use a ConfigMap source.

First, create a ConfigMap with a simple PromptKit bundle containing a test scenario:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: greeting-prompts
  namespace: default
data:
  pack.json: |
    {
      "$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
      "id": "greeting-prompts",
      "name": "Greeting Prompts",
      "version": "1.0.0",
      "template_engine": {
        "version": "v1",
        "syntax": "{{variable}}"
      },
      "prompts": {
        "greeting": {
          "id": "greeting",
          "name": "Greeting Prompt",
          "version": "1.0.0",
          "system_template": "You are a friendly assistant. Respond warmly to greetings.",
          "user_template": "Say hello to {{name}}.",
          "parameters": {
            "temperature": 0.7
          }
        }
      },
      "scenarios": {
        "greeting-test": {
          "id": "greeting-test",
          "name": "Greeting Test",
          "prompt_ref": "greeting",
          "variables": {
            "name": "World"
          },
          "assertions": [
            {
              "type": "contains",
              "value": "hello",
              "case_insensitive": true
            }
          ]
        }
      }
    }
```

Now create the ArenaSource:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: greeting-source
  namespace: default
spec:
  type: configmap
  configMap:
    name: greeting-prompts
    key: pack.json
  interval: 5m
```

Apply both resources:

```bash
kubectl apply -f configmap.yaml
kubectl apply -f arenasource.yaml
```

Verify the source is ready:

```bash
kubectl get arenasource greeting-source
```

You should see:

```
NAME              TYPE        PHASE   REVISION   AGE
greeting-source   configmap   Ready   12345      10s
```

## Step 2: Configure a Provider

If you don't already have a Provider configured, create one:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials
  namespace: default
type: Opaque
stringData:
  ANTHROPIC_API_KEY: "sk-ant-..."  # Or OPENAI_API_KEY
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: claude-provider
  namespace: default
spec:
  type: claude
  model: claude-sonnet-4-20250514
  secretRef:
    name: llm-credentials
```

```bash
kubectl apply -f provider.yaml
```

Verify the provider is ready:

```bash
kubectl get provider claude-provider
```

## Step 3: Create an ArenaConfig

The ArenaConfig combines your source with providers and evaluation settings:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: greeting-eval
  namespace: default
spec:
  sourceRef:
    name: greeting-source
  providers:
    - name: claude-provider
  evaluation:
    timeout: "2m"
    maxRetries: 2
    concurrency: 1
    metrics:
      - latency
      - tokens
```

```bash
kubectl apply -f arenaconfig.yaml
```

Verify the config is ready:

```bash
kubectl get arenaconfig greeting-eval
```

## Step 4: Run an ArenaJob

Create an ArenaJob to execute the evaluation:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: greeting-eval-001
  namespace: default
spec:
  configRef:
    name: greeting-eval
  type: evaluation
  evaluation:
    outputFormats:
      - json
  workers:
    replicas: 1
  ttlSecondsAfterFinished: 3600
```

```bash
kubectl apply -f arenajob.yaml
```

## Step 5: Monitor the Job

Watch the job progress:

```bash
kubectl get arenajob greeting-eval-001 -w
```

You'll see the job progress through phases:

```
NAME                PHASE      PROGRESS   AGE
greeting-eval-001   Pending    0/1        5s
greeting-eval-001   Running    0/1        10s
greeting-eval-001   Running    1/1        25s
greeting-eval-001   Succeeded  1/1        30s
```

Get detailed status:

```bash
kubectl get arenajob greeting-eval-001 -o yaml
```

The status section shows:

```yaml
status:
  phase: Succeeded
  progress:
    total: 1
    completed: 1
    failed: 0
  result:
    summary:
      passed: 1
      failed: 0
      duration: "5.2s"
```

## Step 6: View Results

For jobs with S3 or PVC output configured, results are stored at the configured location. For this simple example, view results in the job status:

```bash
kubectl describe arenajob greeting-eval-001
```

To see worker logs:

```bash
kubectl logs -l arena.omnia.altairalabs.ai/job=greeting-eval-001
```

## Understanding the Results

Arena Fleet evaluations produce results showing:

- **Pass/Fail**: Whether assertions passed
- **Latency**: Response time from the LLM
- **Tokens**: Input/output token counts
- **Cost**: Estimated cost (if pricing configured)

Example result summary:

```json
{
  "job": "greeting-eval-001",
  "scenarios": [
    {
      "id": "greeting-test",
      "provider": "claude-provider",
      "passed": true,
      "latency_ms": 1234,
      "tokens": {
        "input": 45,
        "output": 28
      },
      "assertions": [
        {
          "type": "contains",
          "expected": "hello",
          "actual": "Hello, World! How can I help you today?",
          "passed": true
        }
      ]
    }
  ]
}
```

## Next Steps

Now that you've run your first evaluation:

- **[Configure S3 Storage](/how-to/configure-arena-s3-storage/)**: Store results in S3 for persistence
- **[Set Up Scheduled Jobs](/how-to/setup-arena-scheduled-jobs/)**: Run evaluations on a schedule
- **[Monitor Job Progress](/how-to/monitor-arena-jobs/)**: Track evaluations in real-time
- **[Use Git Sources](/reference/arenasource/#git)**: Fetch bundles from Git repositories
- **[Compare Providers](/reference/arenaconfig/#providers)**: Test against multiple LLMs

## Cleanup

Remove the resources created in this tutorial:

```bash
kubectl delete arenajob greeting-eval-001
kubectl delete arenaconfig greeting-eval
kubectl delete arenasource greeting-source
kubectl delete configmap greeting-prompts
kubectl delete provider claude-provider
kubectl delete secret llm-credentials
```
