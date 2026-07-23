// Package runtime is the framework-agnostic Omnia runtime SDK. See doc.go.
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
