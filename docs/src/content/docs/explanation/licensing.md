---
title: "Licensing & Features"
description: "Understand Omnia's Open Core and Enterprise editions"
sidebar:
  order: 10
---

Omnia follows an **Open Core** model: the core platform is free and open source (Apache 2.0), with advanced features available in the Enterprise edition.

## Editions Overview

| Edition | Price | Use Case |
|---------|-------|----------|
| **Open Core** | Free | Development, learning, small deployments |
| **Enterprise** | Paid | Production, scale, advanced features |
| **Trial** | Free (30 days) | Evaluate Enterprise features |

## Feature Comparison

### Agent Runtime

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Deploy AI agents | ✅ | ✅ |
| WebSocket streaming | ✅ | ✅ |
| Multi-modal support | ✅ | ✅ |
| Provider CRDs (OpenAI, Anthropic, etc.) | ✅ | ✅ |
| PromptPack CRDs | ✅ | ✅ |
| Dashboard UI | ✅ | ✅ |
| Observability (metrics, traces) | ✅ | ✅ |

### Arena Fleet (Testing & Evaluation)

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| ArenaSource from ConfigMap | ✅ | ✅ |
| ArenaSource from Git | ❌ | ✅ |
| ArenaSource from OCI Registry | ❌ | ✅ |
| ArenaSource from S3/GCS | ❌ | ✅ |
| ArenaJob evaluation | ✅ (limited) | ✅ |
| ArenaJob load testing | ❌ | ✅ |
| ArenaJob data generation | ❌ | ✅ |
| Concurrent scenarios | 10 max | Unlimited |
| Worker replicas | 1 | Unlimited |
| Scheduled jobs (cron) | ❌ | ✅ |
| Event-based triggers | ❌ | ✅ |
| Persistent artifact storage | ❌ | ✅ |

### Operations

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Helm chart deployment | ✅ | ✅ |
| Multi-namespace support | ✅ | ✅ |
| RBAC integration | ✅ | ✅ |
| Network policies | ✅ | ✅ |
| Cost tracking & budgets | ❌ | ✅ |
| Multi-cluster aggregation | ❌ | ✅ |
| Priority support | ❌ | ✅ |

## Enabling Enterprise Features

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

## Open Core Edition

The Open Core edition is perfect for:

- **Learning and experimentation** with AI agent orchestration
- **Development environments** where full features aren't needed
- **Small deployments** with limited testing requirements
- **Proof of concepts** before committing to Enterprise

### Included Features

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

## Enterprise Edition

The Enterprise edition unlocks advanced capabilities for production use:

### Advanced Sources

Pull test scenarios and configurations from your existing infrastructure:

- **Git repositories** - Integrate with GitHub, GitLab, Bitbucket
- **OCI registries** - Pull from container registries
- **S3/GCS buckets** - Use cloud storage for large test suites

### Load Testing

Run realistic load tests against your AI agents:

- Configurable concurrency and request rates
- Distributed workers for horizontal scaling
- Detailed latency and throughput metrics

### Data Generation

Generate synthetic data for training and testing:

- Schema-based generation
- LLM-powered realistic data
- Batch processing capabilities

### Scheduled Jobs

Automate your testing workflows:

- Cron-based scheduling
- Event-based triggers (webhooks, pub/sub)
- Integration with CI/CD pipelines

### Persistent Storage

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

## Trial License

Try Enterprise features free for 30 days:

1. Request a trial at [omnia.altairalabs.ai/trial](https://omnia.altairalabs.ai/trial)
2. Receive your license key via email
3. Install with the license (see [Install with a License](/how-to/install-license/))

Trial licenses include:
- All Enterprise features
- Up to 5 cluster activations
- 100 concurrent scenarios
- 5 worker replicas

## License Validation

Omnia validates licenses using cryptographically signed JWT tokens:

- **Offline validation** - No internet required for validation
- **Cached validation** - License checked every 5 minutes
- **Graceful degradation** - Features degrade to Open Core if license expires

### Activation Tracking

Enterprise licenses include activation tracking to prevent unauthorized sharing:

- Each license specifies maximum activations
- Clusters register with the license server on installation
- Activations can be managed via the dashboard
- Deactivate old clusters to free up slots

### Air-Gapped Environments

For environments without internet access:

1. Contact support with your cluster fingerprint
2. Receive a pre-activated license
3. Install normally - no activation call needed

Get your cluster fingerprint:

```bash
kubectl get namespace kube-system -o jsonpath='{.metadata.uid}'
```

## Frequently Asked Questions

### Can I use Open Core in production?

Yes! Open Core is fully functional for agent deployment. The limitations only affect Arena Fleet testing features.

### What happens when my license expires?

Features gracefully degrade to Open Core functionality. Your agents continue running, but Enterprise Arena features become unavailable.

### Can I downgrade from Enterprise to Open Core?

Yes. Remove your license Secret and the operator will run in Open Core mode. Existing resources using Enterprise features will show validation warnings.

### Is the source code available?

Yes. Omnia is fully open source under Apache 2.0. The license validation code is auditable in the repository.

### How do I get support?

- **Open Core**: GitHub issues, community Discord
- **Enterprise**: Priority email support, dedicated channels for Business/Enterprise plans

## Next Steps

- [Install with a License](/how-to/install-license/) - Get started with Enterprise
- [Arena Fleet Overview](/explanation/arena-fleet/) - Learn about testing capabilities
- [Request a Trial](https://omnia.altairalabs.ai/trial) - Try Enterprise free
