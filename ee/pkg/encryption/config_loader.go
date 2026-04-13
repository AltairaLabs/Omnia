/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// ProviderConfigFromEncryptionSpec builds a ProviderConfig from an
// EncryptionConfig, loading credentials from the referenced Secret if any.
// namespace is the namespace the Secret lives in (typically "omnia-system").
func ProviderConfigFromEncryptionSpec(
	ctx context.Context,
	c client.Client,
	namespace string,
	enc *omniav1alpha1.EncryptionConfig,
) (ProviderConfig, error) {
	if enc == nil {
		return ProviderConfig{}, nil
	}
	cfg := ProviderConfig{
		ProviderType: ProviderType(enc.KMSProvider),
		KeyID:        enc.KeyID,
	}

	if enc.SecretRef != nil {
		creds, err := loadSecretCredentials(ctx, c, namespace, enc.SecretRef.Name)
		if err != nil {
			return cfg, err
		}
		cfg.Credentials = creds
		if v, ok := creds["vault-url"]; ok {
			cfg.VaultURL = v
		}
	}

	return cfg, nil
}

// loadSecretCredentials loads a K8s Secret's Data map into string-string form.
func loadSecretCredentials(
	ctx context.Context, c client.Client, namespace, secretName string,
) (map[string]string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("loading secret %q: %w", secretName, err)
	}

	creds := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		creds[k] = string(v)
	}
	return creds, nil
}
