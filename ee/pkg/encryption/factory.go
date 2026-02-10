/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import "fmt"

// NewProvider creates a new encryption Provider based on the given configuration.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.ProviderType {
	case ProviderAzureKeyVault:
		return newAzureKeyVaultProvider(cfg)
	case ProviderAWSKMS:
		return nil, fmt.Errorf("%w: aws-kms (see https://github.com/AltairaLabs/Omnia/issues/437)", ErrProviderNotImplemented)
	case ProviderGCPKMS:
		return nil, fmt.Errorf("%w: gcp-kms (see https://github.com/AltairaLabs/Omnia/issues/438)", ErrProviderNotImplemented)
	case ProviderVault:
		return nil, fmt.Errorf("%w: vault (see https://github.com/AltairaLabs/Omnia/issues/439)", ErrProviderNotImplemented)
	default:
		return nil, fmt.Errorf("unknown encryption provider type: %q", cfg.ProviderType)
	}
}
