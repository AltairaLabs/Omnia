/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/altairalabs/omnia/internal/session"
)

const (
	// envelopePayloadKey is the key inside an encrypted envelope map that holds the
	// base64-encoded ciphertext.
	envelopePayloadKey = "_payload"
	// errorMessageEncPrefix distinguishes an encrypted error message string from a
	// legacy plaintext one.
	errorMessageEncPrefix = "enc:v1:"

	fieldArguments    = "arguments"
	fieldResult       = "result"
	fieldData         = "data"
	fieldErrorMessage = "errorMessage"
)

// encryptEnvelope JSON-marshals v, encrypts the bytes, and returns a map carrying
// the ciphertext and key metadata under well-known keys.
func (e *encryptor) encryptEnvelope(ctx context.Context, v any) (map[string]any, *EncryptOutput, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal envelope value: %w", err)
	}
	out, err := e.provider.Encrypt(ctx, data)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt envelope: %w", err)
	}
	envelope := map[string]any{
		encryptionMetadataKey: map[string]any{
			"keyID":      out.KeyID,
			"keyVersion": out.KeyVersion,
			"algorithm":  out.Algorithm,
		},
		envelopePayloadKey: base64.StdEncoding.EncodeToString(out.Ciphertext),
	}
	return envelope, out, nil
}

// decryptEnvelope reverses encryptEnvelope: decodes _payload, decrypts, and
// unmarshals into the target pointer.
func (e *encryptor) decryptEnvelope(ctx context.Context, m map[string]any, into any) error {
	payload, ok := m[envelopePayloadKey].(string)
	if !ok {
		return fmt.Errorf("envelope missing %s", envelopePayloadKey)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return fmt.Errorf("decode envelope payload: %w", err)
	}
	plaintext, err := e.provider.Decrypt(ctx, ciphertext)
	if err != nil {
		return fmt.Errorf("decrypt envelope: %w", err)
	}
	return json.Unmarshal(plaintext, into)
}

// isEnvelopeMap returns true if m looks like an encrypted envelope.
func isEnvelopeMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasMeta := m[encryptionMetadataKey]
	_, hasPayload := m[envelopePayloadKey]
	return hasMeta && hasPayload
}

// encryptErrorMessage encrypts a non-empty string and returns an
// enc:v1:<base64> sentinel-prefixed representation. Empty input returns
// ("", nil, nil).
func (e *encryptor) encryptErrorMessage(ctx context.Context, s string) (string, *EncryptOutput, error) {
	if s == "" {
		return "", nil, nil
	}
	out, err := e.provider.Encrypt(ctx, []byte(s))
	if err != nil {
		return "", nil, fmt.Errorf("encrypt error message: %w", err)
	}
	return errorMessageEncPrefix + base64.StdEncoding.EncodeToString(out.Ciphertext), out, nil
}

// decryptErrorMessage reverses encryptErrorMessage. Strings that lack the
// sentinel prefix are returned unchanged (legacy plaintext).
func (e *encryptor) decryptErrorMessage(ctx context.Context, s string) (string, error) {
	if !strings.HasPrefix(s, errorMessageEncPrefix) {
		return s, nil
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, errorMessageEncPrefix))
	if err != nil {
		return "", fmt.Errorf("decode error message: %w", err)
	}
	plaintext, err := e.provider.Decrypt(ctx, ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt error message: %w", err)
	}
	return string(plaintext), nil
}

// eventFromOutput builds an EncryptionEvent for a given field from an
// EncryptOutput.
func eventFromOutput(field string, out *EncryptOutput) EncryptionEvent {
	return EncryptionEvent{
		Field:      field,
		KeyID:      out.KeyID,
		KeyVersion: out.KeyVersion,
		Algorithm:  out.Algorithm,
	}
}

// --- ToolCall ---

func (e *encryptor) EncryptToolCall(
	ctx context.Context, tc *session.ToolCall,
) (*session.ToolCall, []EncryptionEvent, error) {
	if tc == nil {
		return nil, nil, nil
	}
	encrypted := *tc
	events := make([]EncryptionEvent, 0, 3)

	if tc.Arguments != nil {
		env, out, err := e.encryptEnvelope(ctx, tc.Arguments)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting tool call arguments: %w", err)
		}
		// Arguments is map[string]any, so the envelope fits.
		encrypted.Arguments = env
		events = append(events, eventFromOutput(fieldArguments, out))
	}

	if tc.Result != nil {
		env, out, err := e.encryptEnvelope(ctx, tc.Result)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting tool call result: %w", err)
		}
		encrypted.Result = env
		events = append(events, eventFromOutput(fieldResult, out))
	}

	if tc.ErrorMessage != "" {
		s, out, err := e.encryptErrorMessage(ctx, tc.ErrorMessage)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting tool call error message: %w", err)
		}
		encrypted.ErrorMessage = s
		events = append(events, eventFromOutput(fieldErrorMessage, out))
	}

	return &encrypted, events, nil
}

func (e *encryptor) DecryptToolCall(
	ctx context.Context, tc *session.ToolCall,
) (*session.ToolCall, error) {
	if tc == nil {
		return nil, nil
	}
	decrypted := *tc

	args, err := e.decryptArgumentsField(ctx, tc.Arguments)
	if err != nil {
		return nil, fmt.Errorf("decrypting tool call arguments: %w", err)
	}
	decrypted.Arguments = args

	result, err := e.decryptAnyField(ctx, tc.Result)
	if err != nil {
		return nil, fmt.Errorf("decrypting tool call result: %w", err)
	}
	decrypted.Result = result

	errMsg, err := e.decryptErrorMessage(ctx, tc.ErrorMessage)
	if err != nil {
		return nil, fmt.Errorf("decrypting tool call error message: %w", err)
	}
	decrypted.ErrorMessage = errMsg

	return &decrypted, nil
}

// decryptArgumentsField decrypts a map[string]any field that may be an
// envelope. Returns the input unchanged if it isn't an envelope.
func (e *encryptor) decryptArgumentsField(ctx context.Context, m map[string]any) (map[string]any, error) {
	if !isEnvelopeMap(m) {
		return m, nil
	}
	var out map[string]any
	if err := e.decryptEnvelope(ctx, m, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// decryptAnyField decrypts an `any` field that may be an envelope map. Returns
// non-map values unchanged (they were plaintext originals).
func (e *encryptor) decryptAnyField(ctx context.Context, v any) (any, error) {
	m, ok := v.(map[string]any)
	if !ok || !isEnvelopeMap(m) {
		return v, nil
	}
	var out any
	if err := e.decryptEnvelope(ctx, m, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// --- RuntimeEvent ---

func (e *encryptor) EncryptRuntimeEvent(
	ctx context.Context, evt *session.RuntimeEvent,
) (*session.RuntimeEvent, []EncryptionEvent, error) {
	if evt == nil {
		return nil, nil, nil
	}
	encrypted := *evt
	events := make([]EncryptionEvent, 0, 2)

	if evt.Data != nil {
		env, out, err := e.encryptEnvelope(ctx, evt.Data)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting runtime event data: %w", err)
		}
		encrypted.Data = env
		events = append(events, eventFromOutput(fieldData, out))
	}

	if evt.ErrorMessage != "" {
		s, out, err := e.encryptErrorMessage(ctx, evt.ErrorMessage)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting runtime event error message: %w", err)
		}
		encrypted.ErrorMessage = s
		events = append(events, eventFromOutput(fieldErrorMessage, out))
	}

	return &encrypted, events, nil
}

func (e *encryptor) DecryptRuntimeEvent(
	ctx context.Context, evt *session.RuntimeEvent,
) (*session.RuntimeEvent, error) {
	if evt == nil {
		return nil, nil
	}
	decrypted := *evt

	data, err := e.decryptArgumentsField(ctx, evt.Data)
	if err != nil {
		return nil, fmt.Errorf("decrypting runtime event data: %w", err)
	}
	decrypted.Data = data

	errMsg, err := e.decryptErrorMessage(ctx, evt.ErrorMessage)
	if err != nil {
		return nil, fmt.Errorf("decrypting runtime event error message: %w", err)
	}
	decrypted.ErrorMessage = errMsg

	return &decrypted, nil
}
