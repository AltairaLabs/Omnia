/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

// ProviderType identifies a KMS provider.
type ProviderType string

const (
	// ProviderAzureKeyVault uses Azure Key Vault for key management.
	ProviderAzureKeyVault ProviderType = "azure-keyvault"
	// ProviderAWSKMS uses AWS Key Management Service.
	ProviderAWSKMS ProviderType = "aws-kms"
	// ProviderGCPKMS uses Google Cloud KMS.
	ProviderGCPKMS ProviderType = "gcp-kms"
	// ProviderVault uses HashiCorp Vault transit backend.
	ProviderVault ProviderType = "vault"
)

// ProviderConfig contains configuration for creating a KMS provider.
type ProviderConfig struct {
	// ProviderType is the type of KMS provider to use.
	ProviderType ProviderType
	// KeyID is the identifier of the key to use.
	KeyID string
	// VaultURL is the URL of the key vault (Azure Key Vault URL, Vault address, etc.).
	VaultURL string
	// Credentials contains provider-specific credential values from a K8s Secret.
	Credentials map[string]string
}
