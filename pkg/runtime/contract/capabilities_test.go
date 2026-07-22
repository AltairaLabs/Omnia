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

import "testing"

func TestKnownCapabilities_ContainsWellKnownNames(t *testing.T) {
	caps := KnownCapabilities()
	if len(caps) == 0 {
		t.Fatal("KnownCapabilities returned no capabilities")
	}

	found := make(map[string]bool, len(caps))
	for _, c := range caps {
		found[c] = true
	}
	for _, want := range []string{
		CapabilityInvoke, CapabilityDuplexAudio, CapabilityClientTools,
		CapabilityConsentGrants, CapabilityMediaStorage, CapabilityInterruption,
	} {
		if !found[want] {
			t.Errorf("KnownCapabilities missing %q", want)
		}
	}
}
