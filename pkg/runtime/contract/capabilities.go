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

package contract

// Capability names a runtime advertises over HealthResponse.capabilities and
// the AgentRuntime status. This is an OPEN, growing set — a newer runtime may
// advertise a name not listed here, and consumers must display/ignore unknown
// names rather than reject them. These constants are the vocabulary "known as
// of this contract build", not an exhaustive enum.
const (
	CapabilityInvoke        = "invoke"            // one-shot Invoke RPC (function mode)
	CapabilityDuplexAudio   = "duplex_audio"      // duplex_start / audio_input / media_chunk
	CapabilityClientTools   = "client_tools"      // client-side tool execution round-trip
	CapabilityConsentGrants = "consent_grants"    // consent grant propagation
	CapabilityMediaStorage  = "media_storage_ref" // storage_ref attachment resolution
	CapabilityInterruption  = "interruption"      // realtime voice interruption
)

// KnownCapabilities returns the capability names this contract build defines.
// Consumers must treat capability lists as open — absence from this slice means
// "unknown to this build", not "invalid".
func KnownCapabilities() []string {
	return []string{
		CapabilityInvoke,
		CapabilityDuplexAudio,
		CapabilityClientTools,
		CapabilityConsentGrants,
		CapabilityMediaStorage,
		CapabilityInterruption,
	}
}
