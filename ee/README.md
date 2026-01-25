# Omnia Enterprise Features

This directory contains enterprise features for Omnia, licensed under the
Functional Source License (FSL-1.1-Apache-2.0). See [LICENSE](./LICENSE) for details.

## Features

### Arena

Arena provides enterprise-grade evaluation, load testing, and data generation
capabilities for AI agents:

- **Evaluation Jobs** - Systematically evaluate agent performance against scenarios
- **Load Testing** - Stress test agents under realistic traffic patterns
- **Data Generation** - Generate synthetic training and test data

### Licensing

Enterprise features require a valid license key. Contact sales@altairalabs.com
for licensing information.

## Structure

```
ee/
├── api/v1alpha1/       # Arena CRD types (ArenaJob, ArenaSource)
├── cmd/
│   ├── omnia-arena-controller/  # Arena controller binary
│   └── arena-worker/            # Arena worker binary
├── internal/
│   ├── controller/     # Arena controllers
│   └── webhook/        # License validation webhooks
└── pkg/
    ├── license/        # License validation
    └── arena/          # Arena infrastructure
        ├── aggregator/ # Result aggregation
        ├── fetcher/    # Source fetching (git, oci, s3)
        ├── queue/      # Work queue (Redis)
        ├── storage/    # Result storage
        ├── overrides/  # Override management
        ├── partitioner/# Scenario partitioning
        └── providers/  # Provider discovery
```

## Building

```bash
# Build arena controller
go build -o bin/omnia-arena-controller ./ee/cmd/omnia-arena-controller

# Build arena worker
go build -o bin/arena-worker ./ee/cmd/arena-worker
```

## Deployment

Enterprise features are enabled via Helm values:

```yaml
enterprise:
  enabled: true
  license:
    key: "your-license-key"
  arena:
    enabled: true
```
