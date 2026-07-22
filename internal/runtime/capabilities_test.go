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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

func TestCapabilities_AdvertisesTheImplementedSet(t *testing.T) {
	caps := Capabilities()
	// The OOTB runtime implements every contract capability.
	assert.ElementsMatch(t, contract.KnownCapabilities(), caps)
	// Every advertised value is a known contract capability (no typos).
	for _, c := range caps {
		assert.Contains(t, contract.KnownCapabilities(), c)
	}
}

func TestWithDuplexAudio_SetsRequiredFormat(t *testing.T) {
	want := &DuplexAudioParams{Codec: "pcm", SampleRate: 24000, Channels: 1}
	s := NewServer(WithDuplexAudio(want))
	assert.Same(t, want, s.duplexAudio)
}

func TestWithContextWindow_AndMediaResolverWiring(t *testing.T) {
	// WithMediaBasePath wires a resolver; HasMediaResolver reports it.
	s := NewServer(WithMediaBasePath("/etc/omnia/media"), WithContextWindow(4096))
	assert.True(t, s.HasMediaResolver(), "WithMediaBasePath should wire a media resolver")
	assert.NotEmpty(t, s.sdkOptions, "WithContextWindow should append an sdk token-budget option")

	// A zero base path wires nothing; a zero context window appends nothing.
	empty := NewServer(WithMediaBasePath(""), WithContextWindow(0))
	assert.False(t, empty.HasMediaResolver())
	assert.Empty(t, empty.sdkOptions)
}
