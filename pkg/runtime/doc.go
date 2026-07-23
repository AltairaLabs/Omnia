// Package runtime is the framework-agnostic Go SDK for building an Omnia custom
// runtime. Implement the small Handler interface and Serve gives you a fully
// conformant omnia.runtime.v1 gRPC server — you never hand-write the wire
// protocol (hello-first, client-tool round-trips, ServerMessage marshalling,
// health, capability advertisement).
//
// The SDK has zero framework dependencies: wrap LangChain-go, your own client,
// or a subprocess in tens of lines. It passes the Wave-4 conformance suite
// (pkg/runtime/conformance) by construction.
//
// Import path: github.com/altairalabs/omnia/pkg/runtime
//
// PromptKit-based runtimes should instead use
// github.com/altairalabs/omnia/pkg/runtime/promptkit, which exposes Omnia's
// first-party PromptKit runtime directly.
package runtime
