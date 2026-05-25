// This fixture is a self-contained stdlib-only Go binary used by the
// consolidation E2E test. It declares its own module so the Dockerfile
// can build it inside a scratch image without the omnia workspace.
// Do NOT add omnia module imports here — the fixture must stay stdlib
// only or the kind-loaded image gets huge.
module github.com/altairalabs/omnia/test/e2e/fixtures/consolidation-fn-stub

go 1.23
