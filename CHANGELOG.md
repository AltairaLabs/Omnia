# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Tool registry overrides for ArenaJob - override tools in `arena.config.yaml` with handlers from ToolRegistry CRDs using label selectors
- Provider overrides for ArenaJob using Kubernetes label selectors - dynamically select Provider CRDs at runtime based on labels
- Version switching in Arena dashboard - browse and switch between synced content versions
- Reusable label selector utilities in `pkg/selector` package
- Versioned release automation for Helm charts and documentation (#72)
- Release workflow with automated Docker, Helm, and docs publishing
- GHCR OCI registry support for Helm charts
- Release helper script (`scripts/release.sh`)
- Makefile targets for Helm packaging and validation

## [0.1.0] - 2026-01-03

### Added
- Initial release of Omnia Kubernetes operator
- AgentRuntime CRD for deploying AI agents
- PromptPack CRD for managing agent prompts
- ToolRegistry CRD for tool discovery and configuration
- Provider CRD for LLM provider configuration
- WebSocket and gRPC facade support
- Session management with Redis and in-memory backends
- HTTP, gRPC, and MCP tool adapters
- KEDA-based autoscaling support
- Prometheus metrics integration
- Comprehensive E2E test suite
- Starlight documentation site

### Infrastructure
- GitHub Actions CI/CD pipeline
- SonarCloud code quality integration
- Helm chart with optional observability stack dependencies
