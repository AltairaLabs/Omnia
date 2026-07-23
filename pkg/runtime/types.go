package runtime

// Identity is the caller identity parsed from x-omnia-* gRPC metadata attached
// by the facade. The raw bearer token is deliberately NOT propagated by the
// facade and is therefore absent here; downstream identity travels via UserID,
// UserEmail, and Claims (x-omnia-claim-*).
type Identity struct {
	AgentName     string
	Namespace     string
	SessionID     string
	RequestID     string
	UserID        string
	UserEmail     string
	Provider      string
	Model         string
	Origin        string
	Workspace     string
	Claims        map[string]string
	ConsentGrants []string
	ConsentLayer  string
}

// ContentPart is one part of a multimodal turn input (or Done output).
type ContentPart struct {
	Type       string // "text" | "image" | "audio" | "video"
	Text       string // set when Type == "text"
	Data       string // base64-encoded media (mutually exclusive with URL)
	URL        string // http/https media passthrough
	MimeType   string // e.g. "image/png"
	StorageRef string // opaque backend ref, e.g. "omnia://sessions/{id}/media/{id}"
}

// Turn is a single inbound conversational turn.
type Turn struct {
	SessionID     string
	Content       string            // legacy text; Parts takes precedence when set
	Parts         []ContentPart     // multimodal input
	Metadata      map[string]string // app-level ClientMessage.metadata (e.g. mock scenarios)
	ConsentGrants []string          // per-message consent overrides
	Identity      Identity
}

// ClientToolCall requests the client (browser) execute a tool.
type ClientToolCall struct {
	ID             string
	Name           string
	ArgumentsJSON  string
	ConsentMessage string
	Categories     []string
}

// ClientToolResult is the client's response to a ClientToolCall.
type ClientToolResult struct {
	CallID          string
	ResultJSON      string
	IsRejected      bool
	RejectionReason string
}

// MediaChunk is a progressive media output chunk (raw bytes, not base64).
type MediaChunk struct {
	MediaID  string
	Sequence int32
	IsLast   bool
	MimeType string
	Data     []byte
}

// Usage is token/cost accounting for a completed turn or invocation.
type Usage struct {
	InputTokens  int32
	OutputTokens int32
	CostUSD      float32
}

// Done is the terminal frame of a turn.
type Done struct {
	Final string        // final text; Parts takes precedence when set
	Usage *Usage        // optional accounting
	Parts []ContentPart // optional multimodal output
}

// InvocationRequest is a one-shot function-mode call.
type InvocationRequest struct {
	InputJSON    string
	InvocationID string
	Metadata     map[string]string
	Identity     Identity
}

// InvocationResponse is the result of a one-shot Invoke. The SDK echoes the
// request's InvocationID onto the wire; the author need not set it.
type InvocationResponse struct {
	OutputJSON string
	Usage      *Usage
	DurationMS int32
}

// ResumeState is the SDK-facing resume classification for HasConversation.
type ResumeState int

const (
	// ResumeUnavailable means the store is unreachable/unknown — NOT an expiry.
	// This is the default when a Handler does not implement ConversationProber.
	ResumeUnavailable ResumeState = iota
	// ResumeResumable means the session's context can be resumed.
	ResumeResumable
	// ResumeNotFound means there is definitively no context (expired or never
	// persisted).
	ResumeNotFound
)
