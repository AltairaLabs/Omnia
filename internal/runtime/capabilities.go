/*
Copyright 2026.

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

package runtime

import (
	"github.com/altairalabs/omnia/pkg/runtime/contract"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// Capabilities returns the contract capabilities this built-in runtime binary
// implements. It is specific to this binary — a third-party runtime advertises
// its own set by populating HealthResponse.capabilities in its own Health
// implementation (the future pkg/runtime SDK will offer a declarative helper).
// A runtime that advertises nothing is treated as pre-negotiation (legacy).
func Capabilities() []string {
	return contract.KnownCapabilities()
}

// WithDuplexAudio sets the required realtime audio format for duplex sessions
// (spec.duplex.audio). When non-nil, it becomes the bounded counter-offer the
// runtime advertises in RuntimeHello and prefers over the client's DuplexStart.
func WithDuplexAudio(p *DuplexAudioParams) ServerOption {
	return func(s *Server) {
		s.duplexAudio = p
	}
}

// sendRuntimeHello sends the RuntimeHello as the first ServerMessage on a
// Converse stream: the session's authoritative capability set plus, for a
// duplex session, the media counter-offer the facade relays to the client.
// media is nil on the text path (capabilities only).
func (s *Server) sendRuntimeHello(stream runtimev1.RuntimeService_ConverseServer, media *runtimev1.MediaNegotiation) error {
	return stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_RuntimeHello{
			RuntimeHello: &runtimev1.RuntimeHello{
				Capabilities: Capabilities(),
				Media:        media,
			},
		},
	})
}

// sendTextHelloOnce sends the capabilities-only RuntimeHello the first time it
// is called on a text Converse stream (tracked via *sent); a no-op afterwards.
func (s *Server) sendTextHelloOnce(stream runtimev1.RuntimeService_ConverseServer, sent *bool) error {
	if *sent {
		return nil
	}
	if err := s.sendRuntimeHello(stream, nil); err != nil {
		return err
	}
	*sent = true
	return nil
}
