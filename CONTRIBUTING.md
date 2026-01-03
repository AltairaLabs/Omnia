# Contributing to Omnia

Thank you for your interest in contributing to Omnia! This document provides comprehensive guidelines and instructions for contributing to our open source project.

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to [conduct@altairalabs.ai](mailto:conduct@altairalabs.ai).

## Developer Certificate of Origin (DCO)

This project uses the Developer Certificate of Origin (DCO) to ensure that contributors have the right to submit their contributions. By making a contribution to this project, you certify that:

1. The contribution was created in whole or in part by you and you have the right to submit it under the open source license indicated in the file; or
2. The contribution is based upon previous work that, to the best of your knowledge, is covered under an appropriate open source license and you have the right under that license to submit that work with modifications, whether created in whole or in part by you, under the same open source license (unless you are permitted to submit under a different license), as indicated in the file; or
3. The contribution was provided directly to you by some other person who certified (1), (2) or (3) and you have not modified it.

### Signing Your Commits

To sign off on your commits, add the `-s` flag to your git commit command:

```bash
git commit -s -m "Your commit message"
```

This adds a "Signed-off-by" line to your commit message:

```
Signed-off-by: Your Name <your.email@example.com>
```

## How to Contribute

### Reporting Bugs

- Check existing issues first
- Provide clear reproduction steps
- Include version information and Kubernetes environment details
- Share relevant CRD configurations and operator logs

### Suggesting Features

- Open an issue describing the feature
- Explain the use case and benefits
- Discuss implementation approach

### Submitting Changes

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/your-feature-name`
3. **Make your changes**
4. **Write/update tests**
5. **Run tests**: `make test`
6. **Run linter**: `make lint`
7. **Commit your changes**: Use clear, descriptive commit messages
8. **Push to your fork**: `git push origin feature/your-feature-name`
9. **Open a Pull Request**

## Development Setup

### Prerequisites

- Go 1.21 or later
- Make (for build automation)
- Docker (for building container images)
- kubectl (for Kubernetes interaction)
- kind or minikube (for local testing)
- kubebuilder (for CRD scaffolding)

### Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/Omnia.git
cd Omnia

# Install dependencies
make install

# Run tests
make test

# Build all components
make build
```

### Project Structure

```
Omnia/
├── api/v1alpha1/       # CRD type definitions
├── cmd/
│   ├── operator/       # Operator entrypoint
│   └── agent/          # Agent container entrypoint
├── internal/
│   ├── controller/     # Kubernetes controllers
│   ├── facade/         # WebSocket facade
│   └── session/        # Session store implementations
├── config/
│   ├── crd/            # Generated CRD manifests
│   ├── rbac/           # RBAC configuration
│   └── samples/        # Example CRs
├── charts/omnia/       # Helm chart
└── docs/               # Documentation
```

## Component-Specific Contribution Guidelines

### CRD Types (`api/v1alpha1/`)

**Focus**: Custom Resource Definitions for AgentRuntime, PromptPack, and ToolRegistry

**Key Areas for Contribution:**
- New fields and validation rules
- Status conditions and phase management
- Kubebuilder markers and annotations
- OpenAPI schema improvements

**Testing CRD Changes:**
```bash
# Generate CRD manifests
make manifests

# Install CRDs to cluster
make install

# Run controller tests
make test
```

### Controllers (`internal/controller/`)

**Focus**: Kubernetes reconciliation logic for managing agent deployments

**Key Areas for Contribution:**
- Reconciliation logic improvements
- Status update handling
- Event recording and observability
- Error handling and retry logic

**Testing Controller Changes:**
```bash
# Run unit tests with envtest
make test

# Run specific controller tests
go test ./internal/controller/... -v
```

### WebSocket Facade (`internal/facade/`)

**Focus**: Real-time communication layer between clients and agents

**Key Areas for Contribution:**
- Protocol improvements
- Connection handling and lifecycle
- Streaming optimizations
- Error handling and reconnection

**Testing Facade Changes:**
```bash
# Run facade tests
go test ./internal/facade/... -v
```

### E2E Tests

End-to-end tests validate the full operator workflow in a real Kubernetes cluster.

**Running E2E Tests:**
```bash
# Run the full E2E test suite (creates a Kind cluster)
make test-e2e
```

**Debugging E2E Tests:**

When E2E tests fail, use the debug helper script to step through tests manually:

```bash
# One-time setup: create cluster, build images, deploy operator
./hack/e2e-debug.sh setup

# Deploy test agents
./hack/e2e-debug.sh agent       # Basic runtime mode agent
./hack/e2e-debug.sh demo-agent  # Demo handler for tool call testing

# Run specific tests
./hack/e2e-debug.sh test-ws     # Test WebSocket connection
./hack/e2e-debug.sh test-tool   # Test tool call flow

# Debug failures
./hack/e2e-debug.sh logs        # View operator and agent logs
./hack/e2e-debug.sh shell       # Interactive shell in cluster

# After code changes
./hack/e2e-debug.sh rebuild     # Rebuild images and reload
./hack/e2e-debug.sh cleanup     # Clear resources for fresh test

# Cleanup
./hack/e2e-debug.sh teardown    # Delete everything
```

This workflow allows you to:
- Inspect deployed resources between test steps
- View logs in real-time while tests run
- Make code changes and quickly reload without full cluster recreation
- Run an interactive shell for in-cluster debugging

### Session Store (`internal/session/`)

**Focus**: Conversation state persistence

**Key Areas for Contribution:**
- New storage backend implementations
- Performance optimizations
- TTL and cleanup handling
- Clustering support

**Testing Session Store Changes:**
```bash
# Run session store tests
go test ./internal/session/... -v
```

## Coding Guidelines

### Go Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting (included in `make fmt`)
- Write clear, descriptive variable/function names
- Add package-level documentation comments
- Keep functions focused and testable

### Kubernetes Best Practices

- Follow controller-runtime patterns
- Use proper RBAC with least privilege
- Handle finalizers correctly
- Implement proper status conditions

### Testing

- Write unit tests for new functionality
- Maintain test coverage above 50%
- Use table-driven tests where appropriate
- Use envtest for controller testing
- Mock external dependencies

### Documentation

- Update README.md if adding features
- Add inline comments for complex logic
- Update relevant example configurations
- Add package documentation for new packages

## Pull Request Process

1. **Ensure CI passes** - All tests and linter checks must pass
2. **Update documentation** - README, examples, inline docs
3. **Add changelog entry** - Describe your changes
4. **Request review** - Tag maintainers (see `.github/CODEOWNERS`)
5. **Address feedback** - Respond to review comments
6. **Resolve all conversations** - All review comments must be marked as resolved
7. **Sign commits** - Use `git commit -s` for DCO compliance
8. **Keep branch updated** - Rebase or merge with latest `main`
9. **Squash merge** - Maintains clean commit history (preferred)

## Release Process

Maintainers handle releases:

1. Update version numbers
2. Update CHANGELOG.md
3. Create git tag
4. Build and push container images
5. Publish Helm chart
6. Publish to GitHub releases

## Questions?

- Open a GitHub issue for questions
- Check existing documentation
- Review closed issues and PRs

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
