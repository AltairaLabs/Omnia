package runtime

import "context"

// Handler is the surface an author implements to build a runtime. The SDK owns
// the omnia.runtime.v1 wire protocol; the author owns turn logic.
type Handler interface {
	// Capabilities returns the authoritative, honest capability set (use the
	// contract.Capability* constants). Advertised in both Health and the
	// per-stream RuntimeHello; the two MUST match, so the SDK sources both here.
	Capabilities() []string

	// Converse handles one inbound turn, emitting output via emit. It MUST end
	// the turn with emit.Done(...); if it returns nil without calling Done, the
	// SDK sends an empty Done on its behalf.
	Converse(ctx context.Context, turn Turn, emit Emitter) error
}

// Invoker is an optional Handler extension enabling the one-shot Invoke RPC
// (function mode). A Handler that implements Invoker MUST advertise
// contract.CapabilityInvoke; one that does not MUST NOT advertise it.
type Invoker interface {
	Invoke(ctx context.Context, req InvocationRequest) (InvocationResponse, error)
}

// ConversationProber is an optional Handler extension answering resume probes
// (HasConversation). A Handler that does not implement it reports every session
// as ResumeUnavailable.
type ConversationProber interface {
	HasConversation(ctx context.Context, sessionID string) ResumeState
}

// Emitter is the clean output surface the SDK marshals to ServerMessage frames.
type Emitter interface {
	// Chunk streams a partial-text fragment. Empty strings are dropped.
	Chunk(text string) error
	// ToolCall requests a client-side (browser) tool execution and blocks for
	// the client's result; the SDK owns the wire round-trip.
	ToolCall(call ClientToolCall) (ClientToolResult, error)
	// Media streams one progressive media chunk.
	Media(chunk MediaChunk) error
	// Done ends the turn.
	Done(d Done) error
}
