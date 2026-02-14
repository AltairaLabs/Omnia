/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import "context"

// AuditLogger is an optional interface for session audit logging.
// Implemented in ee/pkg/audit for enterprise deployments.
type AuditLogger interface {
	// LogEvent records an audit entry asynchronously. Implementations must
	// be non-blocking — entries may be dropped if the internal buffer is full.
	LogEvent(ctx context.Context, entry *AuditEntry)
	// Close flushes pending entries and stops background workers.
	Close() error
}

// AuditEntry represents a single audit log entry for session operations.
type AuditEntry struct {
	EventType   string
	SessionID   string
	Workspace   string
	AgentName   string
	Namespace   string
	Query       string
	ResultCount int
	IPAddress   string
	UserAgent   string
	Metadata    map[string]string
}

// requestContextKey is the context key for RequestContext.
type requestContextKey struct{}

// RequestContext holds request metadata extracted from HTTP headers.
type RequestContext struct {
	IPAddress string
	UserAgent string
}

// withRequestContext returns a new context with the given RequestContext.
func withRequestContext(ctx context.Context, rc RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey{}, rc)
}

// requestContextFromCtx extracts RequestContext from the context.
// Returns a zero value and false if not present.
func requestContextFromCtx(ctx context.Context) (RequestContext, bool) {
	rc, ok := ctx.Value(requestContextKey{}).(RequestContext)
	return rc, ok
}
