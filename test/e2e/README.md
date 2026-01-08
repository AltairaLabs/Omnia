# E2E Test Organization

All e2e tests in `test/e2e/e2e_test.go` are Ginkgo BDD tests.

## Important Note about VS Code Test Explorer

**Ginkgo tests do NOT appear in VS Code's built-in Go test explorer.** Ginkgo uses a BDD framework that isn't compatible with the standard Go testing UI. Instead, use the VS Code Tasks (see below) or command-line to run individual tests.

## Running Tests

### Via VS Code Tasks (Recommended)

Press `Cmd+Shift+P` (macOS) or `Ctrl+Shift+P` (Windows/Linux), then:
1. Type "Tasks: Run Task"
2. Select the specific test you want (e.g., "e2e: Agent - Conversation Test")

Available tasks for all 15 individual tests plus:
- **e2e: All Tests** - Run the complete test suite via Makefile
- **e2e: List All Specs** - Show all available test specs without running them

### Via Command Line with Ginkgo CLI

Run specific tests using `--focus` to filter by test name:

```bash
# Just the conversation test
KIND=kind KIND_CLUSTER=omnia-test-e2e ginkgo --tags=e2e --focus='should complete a basic conversation' ./test/e2e/

# Skip cleanup to manually debug after test
E2E_SKIP_CLEANUP=true KIND=kind KIND_CLUSTER=omnia-test-e2e ginkgo --tags=e2e --focus='should complete a basic conversation' ./test/e2e/

# WebSocket test
KIND=kind KIND_CLUSTER=omnia-test-e2e ginkgo --tags=e2e --focus='should handle WebSocket' ./test/e2e/

# All tests matching a pattern
KIND=kind KIND_CLUSTER=omnia-test-e2e ginkgo --tags=e2e --focus='Agent' ./test/e2e/

# List all tests without running them
KIND=kind KIND_CLUSTER=omnia-test-e2e ginkgo --tags=e2e --dry-run -v ./test/e2e/
```

### Environment Variables

- **E2E_SKIP_CLEANUP=true** - Skip cleanup after tests to leave resources in cluster for manual debugging
- **CERT_MANAGER_INSTALL_SKIP=true** - Skip CertManager installation if already present

### Via Makefile

The original command still works for running all tests:
```bash
make test-e2e
```

## Test Organization

Tests are organized in Ginkgo Contexts:

### Manager Tests
- **should run successfully** - Controller startup test
- **should ensure the metrics endpoint is serving metrics** - Metrics endpoint test

### Omnia CRDs Tests
- **should create and validate a PromptPack** - PromptPack CRD test
- **should create and validate a ToolRegistry** - ToolRegistry CRD test
- **should create an AgentRuntime and deploy the agent** - Initial deployment test
- **should update AgentRuntime when PromptPack changes** - Reconciliation test
- **should have both facade and runtime containers running** - Container readiness
- **should handle WebSocket connections to the facade** - WebSocket test
- **should complete a basic conversation with mock provider** - Conversation flow
- **should persist session state in Redis** - Session persistence
- **should execute tools via HTTP adapter** - HTTP tool adapter
- **should use CRD image overrides instead of operator defaults** - Image override
- **should use operator defaults when CRD does not specify images** - Default images
- **should allow partial image overrides (facade only)** - Partial overrides
- **should handle tool calls via demo handler** - Demo handler

## Tips

- Tests are `Ordered` and may have dependencies on earlier tests
- The Kind cluster `omnia-test-e2e` is reused across test runs
- Use `ginkgo --tags=e2e --dry-run -v ./test/e2e/` to see all available tests
- For faster iteration, ensure the cluster and CRDs are already deployed before running individual tests
- Install Ginkgo CLI with: `go install github.com/onsi/ginkgo/v2/ginkgo@latest`
