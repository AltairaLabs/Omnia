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

import "context"

// ResumeState reports whether a session's working context can still be continued.
type ResumeState int

const (
	// ResumeStateUnknown is the zero value; treated as ResumeStateUnavailable.
	ResumeStateUnknown ResumeState = iota
	// ResumeStateResumable means the context exists and the conversation continues.
	ResumeStateResumable
	// ResumeStateNotFound means the context store answered definitively that no
	// context exists — the session expired or was never persisted.
	ResumeStateNotFound
	// ResumeStateUnavailable means the context store could not be consulted, so
	// resumability is unknown. This is a server fault, never a session expiry.
	ResumeStateUnavailable
)

// ResumeProber answers whether a session's working context still exists.
//
// Resumability is a property of the context store, which the runtime owns — a
// session-api row proves only that a conversation once existed, not that its
// turns survive. Handlers that front a runtime implement this; the facade
// type-asserts it on the message handler, as it does for ClientToolRouter, so
// the facade never imports internal/agent.
type ResumeProber interface {
	// HasConversation reports the resume disposition of one session.
	HasConversation(ctx context.Context, sessionID string) (ResumeState, error)
}
