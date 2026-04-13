/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/encryption"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
)

// kmsEncryptorFactory builds a sessionapi.Encryptor for a given EncryptionConfig
// using ee/pkg/encryption's KMS provider machinery.
type kmsEncryptorFactory struct {
	kubeClient client.Client
	namespace  string // namespace where SecretRef lives
	log        logr.Logger
}

// Build implements EncryptorFactory.
func (f *kmsEncryptorFactory) Build(cfg omniav1alpha1.EncryptionConfig) (sessionapi.Encryptor, error) {
	provCfg, err := encryption.ProviderConfigFromEncryptionSpec(
		context.Background(), f.kubeClient, f.namespace, &cfg,
	)
	if err != nil {
		return nil, fmt.Errorf("provider config: %w", err)
	}

	provider, err := encryption.NewProvider(provCfg)
	if err != nil {
		return nil, fmt.Errorf("provider build: %w", err)
	}

	f.log.V(1).Info("kms encryptor built",
		"kmsProvider", cfg.KMSProvider,
		"keyID", cfg.KeyID,
	)
	return &providerEncryptorAdapter{provider: provider}, nil
}

// providerEncryptorAdapter adapts ee/pkg/encryption.Provider to the
// sessionapi.Encryptor interface (raw byte operations).
// The ee/pkg/encryption.Encryptor works at the session-artifact level
// (Message, ToolCall, RuntimeEvent); the sessionapi.Encryptor is the
// lower-level interface the HTTP handler uses for opaque byte payloads.
// Wrapping Provider directly avoids the message-level abstraction.
type providerEncryptorAdapter struct {
	provider encryption.Provider
}

// Encrypt implements sessionapi.Encryptor.
func (a *providerEncryptorAdapter) Encrypt(plaintext []byte) ([]byte, error) {
	out, err := a.provider.Encrypt(context.Background(), plaintext)
	if err != nil {
		return nil, fmt.Errorf("kms encrypt: %w", err)
	}
	return out.Ciphertext, nil
}

// Decrypt implements sessionapi.Encryptor.
func (a *providerEncryptorAdapter) Decrypt(ciphertext []byte) ([]byte, error) {
	plaintext, err := a.provider.Decrypt(context.Background(), ciphertext)
	if err != nil {
		return nil, fmt.Errorf("kms decrypt: %w", err)
	}
	return plaintext, nil
}
