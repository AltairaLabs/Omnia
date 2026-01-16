---
title: "Arena Fleet Architecture"
description: "Understanding Arena Fleet for prompt testing and evaluation"
sidebar:
  order: 6
  badge:
    text: Arena
    variant: note
---

Arena Fleet is Omnia's distributed testing framework for evaluating PromptKit bundles at scale. It enables systematic testing of prompts against datasets, tracking results, and comparing performance across versions.

## Overview

Arena Fleet provides:

- **Distributed execution**: Run tests across multiple workers
- **GitOps integration**: Source bundles from Git, OCI, or ConfigMaps
- **Revision tracking**: Track which bundle versions were tested
- **Result aggregation**: Collect and analyze test results
- **Scalability**: Handle large test datasets efficiently

## Core Concepts

### PromptKit Bundles

Arena Fleet tests [PromptKit](https://promptpack.org) bundles - structured collections of prompts with versioning, templating, and parameter definitions. Bundles are fetched from external sources and tested against datasets.

### Sources

An **ArenaSource** defines where to fetch PromptKit bundles from:

```
┌─────────────────────────────────────────────────────────────┐
│                      ArenaSource                            │
├─────────────────────────────────────────────────────────────┤
│  • Git repository (branch, tag, or commit)                  │
│  • OCI registry (container image format)                    │
│  • Kubernetes ConfigMap (for simple cases)                  │
├─────────────────────────────────────────────────────────────┤
│  Polls source at interval → Updates artifact revision       │
│  Provides download URL for workers                          │
└─────────────────────────────────────────────────────────────┘
```

The controller automatically:
1. Polls the source at the configured interval
2. Detects changes (new commits, tags, or versions)
3. Updates the artifact URL and revision
4. Triggers downstream jobs when sources change

### Configurations

An **ArenaConfig** defines how to run tests:

- Which sources to use (prompt bundles, test data)
- Which provider to use for LLM calls
- Test parameters (concurrency, timeout, etc.)
- Evaluation criteria

### Jobs

An **ArenaJob** executes a test run:

- References an ArenaConfig
- Partitions work across workers
- Tracks progress and collects results
- Stores aggregated results

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Arena Controllers                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ ArenaSource  │───▶│ ArenaConfig  │───▶│  ArenaJob    │  │
│  │ Controller   │    │ Controller   │    │ Controller   │  │
│  └──────┬───────┘    └──────────────┘    └──────┬───────┘  │
│         │                                        │          │
│         ▼                                        ▼          │
│  ┌──────────────┐                        ┌──────────────┐  │
│  │   Fetcher    │                        │ Work Queue   │  │
│  │  (Git/OCI)   │                        │   (Redis)    │  │
│  └──────┬───────┘                        └──────┬───────┘  │
│         │                                        │          │
│         ▼                                        ▼          │
│  ┌──────────────┐                        ┌──────────────┐  │
│  │  Artifacts   │                        │   Workers    │  │
│  │   Storage    │                        │   (Pods)     │  │
│  └──────────────┘                        └──────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## Workflow

### 1. Define Sources

Create ArenaSource resources pointing to your PromptKit bundles:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: my-prompts
spec:
  type: git
  interval: 5m
  git:
    url: https://github.com/acme/prompts
    ref:
      branch: main
```

### 2. Configure Tests

Create an ArenaConfig that references sources and defines test parameters:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: evaluation-config
spec:
  sourceRef:
    name: my-prompts
  providerRef:
    name: claude-provider
  workers: 4
  timeout: 5m
```

### 3. Run Jobs

Create an ArenaJob to execute tests:

```yaml
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: evaluation-run-001
spec:
  configRef:
    name: evaluation-config
```

### 4. Monitor Results

Check job status and retrieve results:

```bash
kubectl get arenajob evaluation-run-001 -o yaml
```

## Revision Tracking

Arena Fleet tracks source revisions for reproducibility:

| Source Type | Revision Format | Example |
|-------------|-----------------|---------|
| Git | `branch@sha1:commit` | `main@sha1:abc123` |
| OCI | `tag@sha256:digest` | `v1.0@sha256:def456` |
| ConfigMap | `resourceVersion` | `12345` |

This enables:
- **Reproducible tests**: Re-run with exact same bundle version
- **Change detection**: Only re-test when sources change
- **Audit trail**: Track which versions were tested

## GitOps Integration

Arena Fleet integrates naturally with GitOps workflows:

1. **Developers** push prompt changes to Git
2. **ArenaSource** detects changes and updates artifacts
3. **ArenaJob** runs tests against new version
4. **Results** inform whether to promote changes

```
Developer → Git Push → ArenaSource → ArenaJob → Results
                           ↓
                    Artifact Update
```

## Next Steps

- **[ArenaSource CRD Reference](/reference/arenasource)**: Complete spec details
- **[ArenaConfig CRD Reference](/reference/arenaconfig)**: Test configuration options
- **[ArenaJob CRD Reference](/reference/arenajob)**: Job execution details
