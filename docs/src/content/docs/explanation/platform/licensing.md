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

### Agent Memory

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Per-user & per-agent memory (save / retrieve / recall) | ✅ | ✅ |
| Semantic + hybrid (lexical + vector) recall | ✅ | ✅ |
| Agent-scoped memory (operator-curated, per-agent) | ✅ | ✅ |
| Institutional (workspace-knowledge) memory tier | ❌ | ✅ |
| Multi-tier recall across user / agent / institutional tiers | ❌ | ✅ |
| Policy-driven tier ranking (`MemoryPolicy.tierPrecedence`) | ❌ | ✅ |
| Per-tier recency half-life (`MemoryPolicy.recall.halfLife`) | ❌ | ✅ |
| LLM-driven memory consolidation | ❌ | ✅ |
| Memory Galaxy visualization | ❌ | ✅ |

The institutional tier, multi-tier recall, policy-driven ranking, half-life, LLM
consolidation and Memory Galaxy ship only in the **Enterprise edition**
(`enterprise.enabled=true`). An Open Core build does not include these code
paths, so recall uses the user and agent tiers with uniform ranking.

Within an Enterprise deployment these memory features are gated by the
`enterprise.enabled` edition flag, **not** by license validity: an absent or
expired license logs a startup reminder but does not disable them. See
[License enforcement](#license-enforcement) below.

### Arena Fleet (Testing & Evaluation)

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| ArenaSource from ConfigMap | ✅ | ✅ |
| ArenaSource from Git | ❌ | ✅ |
| ArenaSource from OCI Registry | ❌ | ✅ |
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

### Branding

| Feature | Open Core | Enterprise |
|---------|:---------:|:----------:|
| Omnia default theme (light / dark) | ✅ | ✅ |
| White-label branding (colors, logos, fonts, copy) | ❌ | ✅ |

White-label branding is gated by the `whiteLabel` license entitlement and
enforced server-side — without it, the dashboard falls back to the Omnia
default regardless of any branding configuration. See
[White-label the Dashboard](/how-to/operations/white-label-the-dashboard/).

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
- The enterprise memory, privacy and policy services are deployed

`enterprise.enabled` is the **deploy-time gate** — it controls which components
are installed. The `license.key` is a separate concern: it determines which of
those deployed features are *entitled*. Today the license genuinely enforces two
things — dashboard white-labelling and Arena Fleet source/job/limit checks — while
the enterprise memory, privacy and policy services run on the edition flag alone
and only log a reminder when unlicensed. See [License enforcement](#license-enforcement).

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

- No external sources (Git, OCI)
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
3. Install with the license (see [Install with a License](/how-to/operations/install-license/))

Trial licenses include:
- All Enterprise features
- Up to 5 cluster activations
- 100 concurrent scenarios
- 5 worker replicas

## License enforcement

Omnia licenses are cryptographically signed **RS256 JWT** tokens. Validation is
entirely **offline** — the operator verifies the token against an embedded public
key (rotatable via a ConfigMap) and re-reads the license Secret every 5 minutes.
No internet connection is required to validate a license or to run Enterprise
features.

Enforcement is deliberately light. Omnia is **honour-system** licensing: most
Enterprise features are gated only by the `enterprise.enabled` deploy flag, and
each Enterprise component (the operator plus the memory, privacy and policy-proxy
pods) logs a one-time **startup reminder** when it is running Enterprise features
without a valid license. That reminder never blocks — the features keep working.

What the license actually enforces today:

- **Dashboard white-labelling** — the `whiteLabel` entitlement is checked
  server-side. Without a valid enterprise-tier license the dashboard renders the
  Omnia default theme regardless of any branding configuration.
- **Arena Fleet limits** — admission webhooks reject ArenaSource / ArenaJob
  resources that exceed the license: non-Git/OCI source types, load-testing and
  data-generation jobs, cron scheduling, extra worker replicas, and scenario
  counts above the licensed maximum. Existing resources keep running; only new or
  updated resources are validated.

What is **not** yet enforced:

- The `memoryEnterprise`, `privacyEnterprise` and `policyProxy` entitlement
  booleans exist in the license model (added in #1682) and are surfaced on
  `GET /api/v1/license`, but no backend reads them yet. The memory, privacy and
  policy services run whenever `enterprise.enabled=true`, licensed or not. Runtime
  gates for these are planned in follow-up work.

### Activation tracking (optional telemetry)

For enterprise-tier licenses the operator can register the cluster with the
Altaira Labs license server (`https://license.altairalabs.ai`): it activates once
on install and sends a heartbeat every 24 hours. This exists to **count cluster
activations** for the sales relationship — it records Kubernetes events when a
license is over its activation limit or a heartbeat lapses, but it does **not**
disable any feature if the phone-home fails. Open-core licenses skip activation
entirely.

Because activation is not part of feature gating, environments without outbound
internet access are unaffected: activation attempts simply log a warning event
and the features keep running.

Get your cluster fingerprint (the activation server's cluster identifier):

```bash
kubectl get namespace kube-system -o jsonpath='{.metadata.uid}'
```

## Frequently Asked Questions

### Can I use Open Core in production?

Yes! Open Core is fully functional for agent deployment. The limitations affect Arena Fleet testing and advanced agent memory (see the feature comparison above) — the core agent runtime and per-user/per-agent memory are unrestricted.

### What happens when my license expires?

Nothing stops. Your agents keep running and — because most Enterprise features are
gated by the `enterprise.enabled` deploy flag rather than by license validity —
the enterprise memory, privacy and policy services keep working too. Each
component logs a startup reminder that it is unlicensed. The two exceptions are
the genuinely enforced entitlements: dashboard white-labelling reverts to the
Omnia default theme, and Arena Fleet admission webhooks begin rejecting *new or
updated* enterprise-tier ArenaSource / ArenaJob resources (already-running Arena
resources are not torn down). See [License enforcement](#license-enforcement).

### Can I downgrade from Enterprise to Open Core?

Remove your license Secret and the operator falls back to the open-core license
(no white-labelling, Arena limited to ConfigMap sources and the open-core
scenario/worker limits). This is not the same as switching to an Open Core
*build*: `enterprise.enabled=true` keeps the enterprise components deployed, and
they continue to run with a startup reminder. To fully remove Enterprise features,
set `enterprise.enabled=false`.

### Is the source code available?

Yes. Omnia is fully open source under Apache 2.0. The license validation code is auditable in the repository.

### How do I get support?

- **Open Core**: GitHub issues, community Discord
- **Enterprise**: Priority email support, dedicated channels for Business/Enterprise plans

## Next Steps

- [Install with a License](/how-to/operations/install-license/) - Get started with Enterprise
- [Arena Fleet Overview](/explanation/evaluation/arena-fleet/) - Learn about testing capabilities
- [Request a Trial](https://omnia.altairalabs.ai/trial) - Try Enterprise free
