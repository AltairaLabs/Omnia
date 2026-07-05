---
title: "Licensing & features"
description: "Understand Omnia's Open Core and Enterprise editions"
sidebar:
  order: 10
---

Omnia follows an **Open Core** model: the core platform is free and open source (Apache 2.0), with advanced features available in the Enterprise edition.

## Editions overview

| Edition | Price | Use Case |
|---------|-------|----------|
| **Open Core** | Free | Development, learning, small deployments |
| **Enterprise** | Paid | Production, scale, advanced features |
| **Trial** | Free (30 days) | Evaluate Enterprise features |

## Feature comparison

### Agent runtime

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Deploy AI agents | Yes | Yes |
| WebSocket streaming | Yes | Yes |
| Multi-modal support | Yes | Yes |
| Provider CRDs (OpenAI, Anthropic, etc.) | Yes | Yes |
| PromptPack CRDs | Yes | Yes |
| Dashboard UI | Yes | Yes |
| Observability (metrics, traces) | Yes | Yes |

### Agent memory

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Per-user & per-agent memory (save / retrieve / recall) | Yes | Yes |
| Semantic + hybrid (lexical + vector) recall | Yes | Yes |
| Agent-scoped memory (operator-curated, per-agent) | Yes | Yes |
| Institutional (workspace-knowledge) memory tier | No | Yes |
| Multi-tier recall across user / agent / institutional tiers | No | Yes |
| Policy-driven tier ranking (`MemoryPolicy.tierPrecedence`) | No | Yes |
| Per-tier recency half-life (`MemoryPolicy.recall.halfLife`) | No | Yes |
| LLM-driven memory consolidation | No | Yes |
| Memory Galaxy visualization | No | Yes |

When running Open Core, agent memory recall is restricted to the user and agent
tiers with uniform ranking; everything marked Enterprise above falls back to
that Open Core behavior.

### Arena Fleet (testing & evaluation)

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| ArenaSource from ConfigMap | Yes | Yes |
| ArenaSource from Git | No | Yes |
| ArenaSource from OCI Registry | No | Yes |
| ArenaSource from S3/GCS | No | Yes |
| ArenaJob evaluation | Yes (limited) | Yes |
| ArenaJob load testing | No | Yes |
| ArenaJob data generation | No | Yes |
| Concurrent scenarios | 10 max | Unlimited |
| Worker replicas | 1 | Unlimited |
| Scheduled jobs (cron) | No | Yes |
| Event-based triggers | No | Yes |
| Persistent artifact storage | No | Yes |

### Operations

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Helm chart deployment | Yes | Yes |
| Multi-namespace support | Yes | Yes |
| RBAC integration | Yes | Yes |
| Network policies | Yes | Yes |
| Cost tracking & budgets | No | Yes |
| Multi-cluster aggregation | No | Yes |
| Priority support | No | Yes |

### Branding

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Omnia default theme (light / dark) | Yes | Yes |
| White-label branding (colors, logos, fonts, copy) | No | Yes |

White-label branding is gated by the `whiteLabel` license entitlement and
enforced server-side — without it, the dashboard falls back to the Omnia
default regardless of any branding configuration. See
[White-label the Dashboard](/how-to/operations/white-label-the-dashboard/).

## Enabling Enterprise features

To enable Enterprise features in your Helm deployment:

```yaml
enterprise:
  enabled: true

license:
  key: "your-license-key"
  # Or use existing secret:
  # existingSecret: "license-secret"
```

When `enterprise.enabled=true`:
- Enterprise CRDs (ArenaSource, ArenaJob) are installed
- Arena controller and worker deployments are created
- Shared filesystem features for workspaces are available

:::tip[Local Development]
For local development with Tilt, use `ENABLE_ENTERPRISE=true tilt up` to enable enterprise features.
:::

## Open core edition

The Open Core edition is perfect for:

- **Learning and experimentation** with AI agent orchestration
- **Development environments** where full features aren't needed
- **Small deployments** with limited testing requirements
- **Proof of concepts** before committing to Enterprise

### Included features

- Full agent runtime with all providers
- Dashboard for management and monitoring
- Basic Arena testing with ConfigMap sources
- Up to 10 concurrent test scenarios
- Single-worker execution
- 7-day result retention

### Limitations

- No external sources (Git, OCI, S3)
- No load testing or data generation
- No scheduled or triggered jobs
- No distributed workers
- No persistent artifact storage

## Enterprise edition

The Enterprise edition unlocks advanced capabilities for production use:

### Advanced sources

Pull test scenarios and configurations from your existing infrastructure:

- **Git repositories** - Integrate with GitHub, GitLab, Bitbucket
- **OCI registries** - Pull from container registries
- **S3/GCS buckets** - Use cloud storage for large test suites

### Load testing

Run realistic load tests against your AI agents:

- Configurable concurrency and request rates
- Distributed workers for horizontal scaling
- Detailed latency and throughput metrics

### Data generation

Generate synthetic data for training and testing:

- Schema-based generation
- LLM-powered realistic data
- Batch processing capabilities

### Scheduled jobs

Automate your testing workflows:

- Cron-based scheduling
- Event-based triggers (webhooks, pub/sub)
- Integration with CI/CD pipelines

### Persistent storage

Keep your test artifacts long-term:

- S3/GCS artifact storage
- Configurable retention policies
- Cross-job result comparison

## Pricing

| Plan | Activations | Support | Price |
|------|-------------|---------|-------|
| **Team** | 3 clusters | Email | Contact sales |
| **Business** | 10 clusters | Priority | Contact sales |
| **Enterprise** | Unlimited | Dedicated | Contact sales |

All plans include unlimited users and all Enterprise features.

Contact [sales@altairalabs.ai](mailto:sales@altairalabs.ai) for pricing details.

## Trial license

Try Enterprise features free for 30 days:

1. Request a trial at [omnia.altairalabs.ai/trial](https://omnia.altairalabs.ai/trial)
2. Receive your license key via email
3. Install with the license (see [Install with a License](/how-to/operations/install-license/))

Trial licenses include:
- All Enterprise features
- Up to 5 cluster activations
- 100 concurrent scenarios
- 5 worker replicas

## License validation

Omnia validates licenses using cryptographically signed JWT tokens:

- **Offline validation** - No internet required for validation
- **Cached validation** - License checked every 5 minutes
- **Graceful degradation** - Features degrade to Open Core if license expires

### Activation tracking

Enterprise licenses include activation tracking to prevent unauthorized sharing:

- Each license specifies maximum activations
- Clusters register with the license server on installation
- Activations can be managed via the dashboard
- Deactivate old clusters to free up slots

### Air-gapped environments

For environments without internet access:

1. Contact support with your cluster fingerprint
2. Receive a pre-activated license
3. Install normally - no activation call needed

Get your cluster fingerprint:

```bash
kubectl get namespace kube-system -o jsonpath='{.metadata.uid}'
```

## Frequently asked questions

### Can I use open core in production?

Open Core is fully functional for agent deployment. The limitations affect Arena Fleet testing and advanced agent memory (see the feature comparison above) — the core agent runtime and per-user/per-agent memory are unrestricted.

### What happens when my license expires?

Features gracefully degrade to Open Core functionality. Your agents continue running, but Enterprise features become unavailable — for example, memory recall falls back to the Open Core tiers and stored institutional memories are simply not surfaced.

### Can I downgrade from Enterprise to open core?

Yes. Remove your license Secret and the operator will run in Open Core mode. Existing resources using Enterprise features will show validation warnings.

### Is the source code available?

Yes. Omnia is fully open source under Apache 2.0. The license validation code is auditable in the repository.

### How do I get support?

- **Open Core**: GitHub issues, community Discord
- **Enterprise**: Priority email support, dedicated channels for Business/Enterprise plans

## Next steps

- [Install with a License](/how-to/operations/install-license/) - Get started with Enterprise
- [Arena Fleet Overview](/explanation/evaluation/arena-fleet/) - Learn about testing capabilities
- [Request a Trial](https://omnia.altairalabs.ai/trial) - Try Enterprise free
