/*
Copyright 2025-2026.

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

package facade

import (
	"time"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/facade/auth"
)

// ServerOption configures a Server at construction time.
type ServerOption func(*Server)

// WithMetrics sets the metrics collector for the server.
func WithMetrics(m ServerMetrics) ServerOption {
	return func(s *Server) {
		s.metrics = m
	}
}

// WithMediaStorage sets the media storage for the server.
// When set, the server can handle upload_request messages from clients.
func WithMediaStorage(ms media.Storage) ServerOption {
	return func(s *Server) {
		s.mediaStorage = ms
	}
}

// WithTracingProvider sets the tracing provider for the server.
// When set, the server creates spans for sessions and messages.
func WithTracingProvider(p *tracing.Provider) ServerOption {
	return func(s *Server) {
		s.tracingProvider = p
	}
}

// WithRecordingPool sets the recording worker pool for async session recording.
func WithRecordingPool(p *RecordingPool) ServerOption {
	return func(s *Server) {
		s.recordingPool = p
	}
}

// WithAllowedOrigins sets the allowed origins for WebSocket connections.
// Origins should be scheme + host (e.g., "https://example.com").
// When set, only requests from these origins are allowed.
// When empty, the default gorilla/websocket same-origin check is used.
func WithAllowedOrigins(origins []string) ServerOption {
	return func(s *Server) {
		s.allowedOrigins = origins
	}
}

// WithMgmtPlaneValidator configures the server to run a single mgmt-plane
// Validator. Convenience wrapper around WithAuthChain — exists to keep
// the PR 1a/c API stable while the wider chain machinery (PR 2b+) lands.
//
// Identical semantics to WithAuthChain(auth.Chain{v}): the validator
// runs first; ErrNoCredential falls through to the unauthenticated
// upgrade path (PR 1 default); invalid/expired returns 401. Combine with
// WithAuthChain instead of this option once data-plane validators are in
// the mix.
func WithMgmtPlaneValidator(v auth.Validator) ServerOption {
	return WithAuthChain(auth.Chain{v})
}

// WithAuthChain configures the server to run the supplied auth chain on
// every upgrade. Admit attaches the resulting identity to the
// connection's PropagationFields. ErrNoCredential (no validator admits)
// now returns 401 before Upgrade — PR 3 flipped this from the
// behaviour-preserving default of proceeding unauthenticated, closing
// pen-test finding C-3.
//
// Validator order matters — the first validator that admits wins, so
// list the most specific credential style first. The conventional order
// shipped by cmd/agent is clientKeys → oidc → edgeTrust → mgmt-plane.
//
// Empty chain still proceeds unauthenticated to keep the dev/test path
// working when no validator can be constructed (no mgmt-plane key, no
// externalAuth CRD). Set WithAllowUnauthenticated(false) at the server
// to also reject those requests.
func WithAuthChain(chain auth.Chain) ServerOption {
	return func(s *Server) {
		s.authChain = chain
	}
}

// WithDuplexSinkFactory sets the factory used to create a DuplexSink for
// each connection's persistent audio duplex stream. When nil (default),
// inbound BinaryMessageTypeMediaChunk frames are rejected with a graceful
// error — audio is not enabled. The factory is called lazily on the first
// inbound media chunk for a given connection.
//
// The factory signature deliberately references only facade-exported types
// (sessionID string, ResponseWriter) so internal/facade stays decoupled from
// internal/agent. The real sink implementation in internal/agent satisfies
// the DuplexSink interface and is injected here at binary wiring time in
// cmd/agent.
func WithDuplexSinkFactory(f func(sessionID string, w ResponseWriter) DuplexSink) ServerOption {
	return func(s *Server) {
		s.duplexSinkFactory = f
	}
}

// WithAllowUnauthenticated controls the fallback behaviour when the
// auth chain is empty (no validators configured). Defaults to true so
// standalone dev/test binaries without a k8s client or mgmt-plane key
// still accept WebSocket upgrades. Production deployments going through
// cmd/agent always have at least the mgmt-plane validator in the chain,
// so this flag does not affect them — they 401 on missing credentials
// regardless.
//
// Set to false to reject every unauthenticated upgrade including the
// empty-chain case. Useful for integration tests that want to prove the
// strict default.
func WithAllowUnauthenticated(allow bool) ServerOption {
	return func(s *Server) {
		s.allowUnauthenticated = allow
	}
}

// WithRouteStore sets the RouteStore used by the parked session registry to
// persist and remove pod-address route hints. Defaults to noopRouteStore{}.
func WithRouteStore(rs RouteStore) ServerOption {
	return func(s *Server) { s.routeStore = rs }
}

// WithPodAddr sets the "<podIP>:<port>" address for this pod. Written into
// route hints when a realtime session is parked so a peer can redirect
// reconnecting clients to the correct pod.
func WithPodAddr(addr string) ServerOption {
	return func(s *Server) { s.podAddr = addr }
}

// WithGraceWindow sets how long the parked session registry waits for a
// client to reconnect before expiring the parked audio stream. When ≤0,
// NewServer applies a conservative default of 15s.
func WithGraceWindow(d time.Duration) ServerOption {
	return func(s *Server) { s.graceWindow = d }
}
