/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// mockAzkeysClient is a test double for the azkeysClient interface.
type mockAzkeysClient struct {
	WrapKeyFn func(
		ctx context.Context, keyName, keyVersion string,
		params azkeys.KeyOperationParameters, opts *azkeys.WrapKeyOptions,
	) (azkeys.WrapKeyResponse, error)

	UnwrapKeyFn func(
		ctx context.Context, keyName, keyVersion string,
		params azkeys.KeyOperationParameters, opts *azkeys.UnwrapKeyOptions,
	) (azkeys.UnwrapKeyResponse, error)

	GetKeyFn func(
		ctx context.Context, keyName, keyVersion string,
		opts *azkeys.GetKeyOptions,
	) (azkeys.GetKeyResponse, error)

	RotateKeyFn func(
		ctx context.Context, keyName string,
		opts *azkeys.RotateKeyOptions,
	) (azkeys.RotateKeyResponse, error)
}

func (m *mockAzkeysClient) WrapKey(
	ctx context.Context, keyName, keyVersion string,
	params azkeys.KeyOperationParameters, opts *azkeys.WrapKeyOptions,
) (azkeys.WrapKeyResponse, error) {
	return m.WrapKeyFn(ctx, keyName, keyVersion, params, opts)
}

func (m *mockAzkeysClient) UnwrapKey(
	ctx context.Context, keyName, keyVersion string,
	params azkeys.KeyOperationParameters, opts *azkeys.UnwrapKeyOptions,
) (azkeys.UnwrapKeyResponse, error) {
	return m.UnwrapKeyFn(ctx, keyName, keyVersion, params, opts)
}

func (m *mockAzkeysClient) GetKey(
	ctx context.Context, keyName, keyVersion string,
	opts *azkeys.GetKeyOptions,
) (azkeys.GetKeyResponse, error) {
	return m.GetKeyFn(ctx, keyName, keyVersion, opts)
}

func (m *mockAzkeysClient) RotateKey(
	ctx context.Context, keyName string,
	opts *azkeys.RotateKeyOptions,
) (azkeys.RotateKeyResponse, error) {
	return m.RotateKeyFn(ctx, keyName, opts)
}
