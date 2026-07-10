---
title: "Arena Fleet: Run Your First Evaluation"
description: "Get started with Arena Fleet by running your first prompt evaluation"
sidebar:
  order: 4
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
helm install omnia-demos oci://ghcr.io/altairalabs/charts/omnia-demos \
  --devel \
  -n omnia-demo --create-namespace \
  --set arenaDemo.enabled=true
```

This deploys a complete Arena demo with sample evaluations using Ollama. Skip to [Step 4](#step-4-monitor-the-job) to see results.
:::

## Overview

Arena Fleet evaluates prompts through **two CRDs** and a config file that lives inside your bundle:

```
ArenaSource ───────────────▶ ArenaJob ───▶ Results
 (bundle:                       │
  config.arena.yaml   ◀─────────┘ selects with spec.arenaFile
  + prompts + scenarios)          executes the evaluation
```

- **[ArenaSource](/reference/evaluation/arenasource/)** fetches your bundle. The bundle contains the arena config file (`config.arena.yaml`), plus the prompt and scenario files it references.
- **[ArenaJob](/reference/evaluation/arenajob/)** references the source, selects the config file with `spec.arenaFile`, supplies providers, and runs the evaluation.

There is **no `ArenaConfig` Kubernetes resource** — the configuration is the [`config.arena.yaml` file](/reference/evaluation/arenaconfig/) inside the bundle, not something you `kubectl apply`.

## Step 1: Create an ArenaSource

An ArenaSource defines where to fetch your bundle from. For this tutorial, we'll use a ConfigMap source. Each key in the ConfigMap becomes a file in the bundle — nested paths are encoded with `__` (double underscore) in place of `/`, because Kubernetes ConfigMap keys cannot contain slashes.

Create a ConfigMap containing the whole bundle: the arena config file, one prompt, and one scenario with an assertion:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: greeting-bundle
  namespace: default
data:
  config.arena.yaml: |
    apiVersion: promptkit.altairalabs.ai/v1alpha1
    kind: Arena
    metadata:
      name: greeting-eval
    spec:
      prompt_configs:
        - id: assistant
          file: prompts/assistant.yaml
      # Providers are supplied by the ArenaJob from Provider CRDs.
      providers: []
      scenarios:
        - file: scenarios/greeting.scenario.yaml
      defaults:
        temperature: 0.7
        max_tokens: 200
        output:
          dir: out
          formats:
            - json

  # "prompts__assistant.yaml" decodes to "prompts/assistant.yaml"
  prompts__assistant.yaml: |
    apiVersion: promptkit.altairalabs.ai/v1alpha1
    kind: PromptConfig
    metadata:
      name: assistant
    spec:
      task_type: assistant
      version: v1.0.0
      description: A friendly greeting assistant
      system_template: "You are a friendly assistant. Respond warmly to greetings."

  # "scenarios__greeting.scenario.yaml" decodes to "scenarios/greeting.scenario.yaml"
  scenarios__greeting.scenario.yaml: |
    apiVersion: promptkit.altairalabs.ai/v1alpha1
    kind: Scenario
    metadata:
      name: greeting-test
    spec:
      id: greeting-test
      task_type: assistant
      description: Greeting test
      turns:
        - role: user
          content: "Say hello to the world."
          assertions:
            - type: contains
              params:
                patterns:
                  - "hello"
```

Now create the ArenaSource that points at the ConfigMap:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: greeting-source
  namespace: default
spec:
  type: configmap
  configMap:
    name: greeting-bundle
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
NAME              TYPE        PHASE   AGE
greeting-source   configmap   Ready   10s
```

## Step 2: Configure a Provider

If you don't already have a Provider configured, create one. The ArenaJob will resolve providers from this CRD, so no provider file is needed inside the bundle.

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

## Step 3: Run an ArenaJob

Create an ArenaJob to execute the evaluation. It references the source, selects the config file with `arenaFile`, and maps the Provider CRD into the config's `default` provider group:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: greeting-eval-001
  namespace: default
spec:
  sourceRef:
    name: greeting-source
  arenaFile: config.arena.yaml
  type: evaluation
  providers:
    default:
      - providerRef:
          name: claude-provider
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

## Step 4: Monitor the Job

Watch the job progress:

```bash
kubectl get arenajob greeting-eval-001 -w
```

You'll see the job progress through phases:

```
NAME                SOURCE            TYPE         PHASE      AGE
greeting-eval-001   greeting-source   evaluation   Pending    5s
greeting-eval-001   greeting-source   evaluation   Running    10s
greeting-eval-001   greeting-source   evaluation   Succeeded  30s
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
      passed: "1"
      failed: "0"
```

## Step 5: View Results

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

- **Pass/Fail**: Whether assertions passed (our scenario asserts the response `contains` "hello")
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
          "passed": true
        }
      ]
    }
  ]
}
```

## Next Steps

Now that you've run your first evaluation:

- **[Configure S3 Storage](/how-to/evaluation/configure-arena-s3-storage/)**: Store results in S3 for persistence
- **[Set Up Scheduled Jobs](/how-to/evaluation/setup-arena-scheduled-jobs/)**: Run evaluations on a schedule
- **[Monitor Job Progress](/how-to/evaluation/monitor-arena-jobs/)**: Track evaluations in real-time
- **[Use Git Sources](/reference/evaluation/arenasource/#git)**: Fetch bundles from Git repositories
- **[Compare Providers](/reference/evaluation/arenajob/#providers)**: Test against multiple LLMs
- **[Tune the config file](/reference/evaluation/arenaconfig/)**: Learn the full `config.arena.yaml` schema

## Cleanup

Remove the resources created in this tutorial:

```bash
kubectl delete arenajob greeting-eval-001
kubectl delete arenasource greeting-source
kubectl delete configmap greeting-bundle
kubectl delete provider claude-provider
kubectl delete secret llm-credentials
```
