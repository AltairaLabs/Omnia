/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/altairalabs/omnia/internal/session"
)

// Encoding constants for field-level encryption.
const (
	// errMsgEncPrefix marks an encrypted error-message string.
	// Presence of this prefix distinguishes ciphertext from legacy plaintext.
	errMsgEncPrefix = "enc:v1:"

	// envelopePayloadKey is the key inside an encrypted envelope map that holds
	// the base64-encoded ciphertext.
	envelopePayloadKey = "_payload"

	// envelopeMarkerKey is present alongside _payload to identify an envelope.
	envelopeMarkerKey = "_enc"
)

// encryptString encrypts a plaintext string using enc and returns an
// "enc:v1:<base64>" representation. When enc is nil or s is empty, s is
// returned unchanged.
func encryptString(enc Encryptor, s string) (string, error) {
	if enc == nil || s == "" {
		return s, nil
	}
	ciphertext, err := enc.Encrypt([]byte(s))
	if err != nil {
		return "", fmt.Errorf("encrypt string: %w", err)
	}
	return errMsgEncPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptString reverses encryptString. Strings without the "enc:v1:" prefix
// are returned unchanged (legacy plaintext / non-encrypted sessions).
func decryptString(enc Encryptor, s string) (string, error) {
	if enc == nil || !strings.HasPrefix(s, errMsgEncPrefix) {
		return s, nil
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, errMsgEncPrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted string: %w", err)
	}
	plaintext, err := enc.Decrypt(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt string: %w", err)
	}
	return string(plaintext), nil
}

// encryptEnvelope JSON-marshals v, encrypts the bytes, and returns an envelope
// map with _enc and _payload keys. When enc is nil, v is returned unchanged as
// a map[string]any (nil v returns nil).
func encryptEnvelope(enc Encryptor, v any) (map[string]any, error) {
	if enc == nil {
		if v == nil {
			return nil, nil
		}
		if m, ok := v.(map[string]any); ok {
			return m, nil
		}
		return nil, nil
	}
	if v == nil {
		return nil, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope value: %w", err)
	}
	ciphertext, err := enc.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt envelope: %w", err)
	}
	return map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

// decryptEnvelopeMap reverses encryptEnvelope for a map[string]any field.
// Non-envelope maps are returned unchanged (legacy plaintext).
func decryptEnvelopeMap(enc Encryptor, m map[string]any) (map[string]any, error) {
	if enc == nil || !isEnvelopeMap(m) {
		return m, nil
	}
	payload, ok := m[envelopePayloadKey].(string)
	if !ok {
		return nil, fmt.Errorf("envelope missing %s", envelopePayloadKey)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode envelope payload: %w", err)
	}
	plaintext, err := enc.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt envelope: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plaintext, &out); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return out, nil
}

// decryptEnvelopeAny reverses encryptEnvelope for an any-typed field.
// Non-envelope values (including non-map types) are returned unchanged.
func decryptEnvelopeAny(enc Encryptor, v any) (any, error) {
	if enc == nil {
		return v, nil
	}
	m, ok := v.(map[string]any)
	if !ok || !isEnvelopeMap(m) {
		return v, nil
	}
	payload, ok := m[envelopePayloadKey].(string)
	if !ok {
		return nil, fmt.Errorf("envelope missing %s", envelopePayloadKey)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode envelope payload: %w", err)
	}
	plaintext, err := enc.Decrypt(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt envelope: %w", err)
	}
	var out any
	if err := json.Unmarshal(plaintext, &out); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return out, nil
}

// isEnvelopeMap returns true if m looks like an encrypted envelope.
func isEnvelopeMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasMark := m[envelopeMarkerKey]
	_, hasPayload := m[envelopePayloadKey]
	return hasMark && hasPayload
}

// --- Message ---

// encryptMessage encrypts the Content field of msg in-place. Encrypted content
// uses the enc:v1: prefix. Metadata values are not encrypted here — they are
// stored as supplementary operational data, not sensitive content.
// When enc is nil, msg is returned unchanged.
func encryptMessage(enc Encryptor, msg *session.Message) error {
	if enc == nil || msg == nil {
		return nil
	}
	content, err := encryptString(enc, msg.Content)
	if err != nil {
		return fmt.Errorf("encrypt message content: %w", err)
	}
	msg.Content = content
	return nil
}

// decryptMessage decrypts the Content field of msg in-place.
// When enc is nil or the field lacks the enc:v1: prefix (legacy plaintext),
// msg is returned unchanged.
func decryptMessage(enc Encryptor, msg *session.Message) error {
	if enc == nil || msg == nil {
		return nil
	}
	content, err := decryptString(enc, msg.Content)
	if err != nil {
		return fmt.Errorf("decrypt message content: %w", err)
	}
	msg.Content = content
	return nil
}

// --- ToolCall ---

// encryptToolCall encrypts Arguments (envelope), Result (envelope), and
// ErrorMessage (enc:v1: prefix). Name and status stay plaintext.
func encryptToolCall(enc Encryptor, tc *session.ToolCall) error {
	if enc == nil || tc == nil {
		return nil
	}

	if tc.Arguments != nil {
		env, err := encryptEnvelope(enc, tc.Arguments)
		if err != nil {
			return fmt.Errorf("encrypt tool call arguments: %w", err)
		}
		tc.Arguments = env
	}

	if tc.Result != nil {
		env, err := encryptEnvelope(enc, tc.Result)
		if err != nil {
			return fmt.Errorf("encrypt tool call result: %w", err)
		}
		tc.Result = env
	}

	if tc.ErrorMessage != "" {
		s, err := encryptString(enc, tc.ErrorMessage)
		if err != nil {
			return fmt.Errorf("encrypt tool call error message: %w", err)
		}
		tc.ErrorMessage = s
	}

	return nil
}

// decryptToolCall decrypts Arguments, Result, and ErrorMessage.
// Fields without envelope markers or enc:v1: prefixes pass through unchanged.
func decryptToolCall(enc Encryptor, tc *session.ToolCall) error {
	if enc == nil || tc == nil {
		return nil
	}

	args, err := decryptEnvelopeMap(enc, tc.Arguments)
	if err != nil {
		return fmt.Errorf("decrypt tool call arguments: %w", err)
	}
	tc.Arguments = args

	result, err := decryptEnvelopeAny(enc, tc.Result)
	if err != nil {
		return fmt.Errorf("decrypt tool call result: %w", err)
	}
	tc.Result = result

	errMsg, err := decryptString(enc, tc.ErrorMessage)
	if err != nil {
		return fmt.Errorf("decrypt tool call error message: %w", err)
	}
	tc.ErrorMessage = errMsg

	return nil
}

// --- RuntimeEvent ---

// encryptRuntimeEvent encrypts Data (envelope) and ErrorMessage (enc:v1: prefix).
// EventType stays plaintext for analytics.
func encryptRuntimeEvent(enc Encryptor, evt *session.RuntimeEvent) error {
	if enc == nil || evt == nil {
		return nil
	}

	if evt.Data != nil {
		env, err := encryptEnvelope(enc, evt.Data)
		if err != nil {
			return fmt.Errorf("encrypt runtime event data: %w", err)
		}
		evt.Data = env
	}

	if evt.ErrorMessage != "" {
		s, err := encryptString(enc, evt.ErrorMessage)
		if err != nil {
			return fmt.Errorf("encrypt runtime event error message: %w", err)
		}
		evt.ErrorMessage = s
	}

	return nil
}

// decryptRuntimeEvent decrypts Data and ErrorMessage.
// EventType and timing stay plaintext.
func decryptRuntimeEvent(enc Encryptor, evt *session.RuntimeEvent) error {
	if enc == nil || evt == nil {
		return nil
	}

	data, err := decryptEnvelopeMap(enc, evt.Data)
	if err != nil {
		return fmt.Errorf("decrypt runtime event data: %w", err)
	}
	evt.Data = data

	errMsg, err := decryptString(enc, evt.ErrorMessage)
	if err != nil {
		return fmt.Errorf("decrypt runtime event error message: %w", err)
	}
	evt.ErrorMessage = errMsg

	return nil
}
