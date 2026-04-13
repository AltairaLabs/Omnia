/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/session"
)

// Encryptor is the subset of encryption operations session-api requires.
// ee/pkg/encryption.Encryptor satisfies this interface via an adapter in
// cmd/session-api that drops the []EncryptionEvent return.
//
// When nil (non-enterprise), session-api reads/writes plaintext.
type Encryptor interface {
	EncryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error)
	DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error)

	EncryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error)
	DecryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error)

	EncryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error)
	DecryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error)
}
