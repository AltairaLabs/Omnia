/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package fleet

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFleetProvider(t *testing.T) {
	t.Run("creates provider with ws_url", func(t *testing.T) {
		spec := providers.ProviderSpec{
			ID:   "agent-bot",
			Type: "fleet",
			AdditionalConfig: map[string]interface{}{
				"ws_url": "ws://agent:8080/ws",
			},
		}
		p, err := createFleetProvider(spec)
		require.NoError(t, err)
		require.NotNil(t, p)
		assert.Equal(t, "agent-bot", p.ID())
	})

	t.Run("returns error without ws_url", func(t *testing.T) {
		spec := providers.ProviderSpec{
			ID:   "no-url",
			Type: "fleet",
		}
		_, err := createFleetProvider(spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ws_url")
	})

	t.Run("returns error with empty ws_url", func(t *testing.T) {
		spec := providers.ProviderSpec{
			ID:   "empty-url",
			Type: "fleet",
			AdditionalConfig: map[string]interface{}{
				"ws_url": "",
			},
		}
		_, err := createFleetProvider(spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ws_url")
	})

	t.Run("factory is registered via init", func(t *testing.T) {
		// Verify CreateProviderFromSpec can handle "fleet" type
		spec := providers.ProviderSpec{
			ID:   "init-test",
			Type: "fleet",
			AdditionalConfig: map[string]interface{}{
				"ws_url": "ws://test:8080/ws",
			},
		}
		p, err := providers.CreateProviderFromSpec(spec)
		require.NoError(t, err)
		assert.Equal(t, "init-test", p.ID())
	})
}
