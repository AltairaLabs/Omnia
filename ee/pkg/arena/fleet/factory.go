/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package fleet

import (
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
)

func init() {
	providers.RegisterProviderFactory("fleet", createFleetProvider)
}

// createFleetProvider creates a fleet provider from a ProviderSpec.
// The "ws_url" must be set in AdditionalConfig. The provider is created
// but NOT connected — call Connect() separately after creation.
func createFleetProvider(spec providers.ProviderSpec) (providers.Provider, error) {
	wsURL, _ := spec.AdditionalConfig["ws_url"].(string)
	if wsURL == "" {
		return nil, fmt.Errorf("fleet provider %q requires 'ws_url' in additional_config", spec.ID)
	}

	return NewProvider(spec.ID, wsURL, nil), nil
}
