/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingProvider is a Provider that returns errors on demand for negative tests.
type failingProvider struct {
	encryptErr error
	decryptErr error
}

func (f *failingProvider) Encrypt(_ context.Context, _ []byte) (*EncryptOutput, error) {
	if f.encryptErr != nil {
		return nil, f.encryptErr
	}
	return &EncryptOutput{Ciphertext: []byte("x"), KeyID: "k", KeyVersion: "1", Algorithm: "test"}, nil
}

func (f *failingProvider) Decrypt(_ context.Context, _ []byte) ([]byte, error) {
	if f.decryptErr != nil {
		return nil, f.decryptErr
	}
	return []byte("plaintext"), nil
}

func (f *failingProvider) GetKeyMetadata(_ context.Context) (*KeyMetadata, error) {
	return &KeyMetadata{KeyID: "k", KeyVersion: "1", Algorithm: "test", CreatedAt: time.Now(), Enabled: true}, nil
}

func (f *failingProvider) RotateKey(_ context.Context) (*KeyRotationResult, error) {
	return &KeyRotationResult{}, nil
}

func (f *failingProvider) Close() error { return nil }

func newTestEncryptor() Encryptor {
	return NewEncryptor(newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", ""))
}

func eventFieldSet(events []EncryptionEvent) map[string]bool {
	out := make(map[string]bool, len(events))
	for _, e := range events {
		out[e.Field] = true
	}
	return out
}

func TestEncryptor_EncryptToolCall_EncryptsArgumentsAndResult(t *testing.T) {
	enc := newTestEncryptor()

	tc := &session.ToolCall{
		ID:        "tc-1",
		Name:      "web_search",
		Arguments: map[string]any{"query": "sensitive search"},
		Result:    "sensitive result",
	}

	encrypted, events, err := enc.EncryptToolCall(context.Background(), tc)
	require.NoError(t, err)
	require.NotNil(t, encrypted)

	// Name stays plaintext.
	assert.Equal(t, "web_search", encrypted.Name)
	assert.Equal(t, "tc-1", encrypted.ID)

	// Arguments is an envelope.
	argsMap := encrypted.Arguments
	require.NotNil(t, argsMap)
	assert.Contains(t, argsMap, encryptionMetadataKey)
	assert.Contains(t, argsMap, envelopePayloadKey)

	// Result is an envelope map.
	resultMap, ok := encrypted.Result.(map[string]any)
	require.True(t, ok, "Result should be envelope map, got %T", encrypted.Result)
	assert.Contains(t, resultMap, encryptionMetadataKey)

	fields := eventFieldSet(events)
	assert.True(t, fields[fieldArguments])
	assert.True(t, fields[fieldResult])
	assert.False(t, fields[fieldErrorMessage])
}

func TestEncryptor_EncryptToolCall_ErrorMessage(t *testing.T) {
	enc := newTestEncryptor()

	tc := &session.ToolCall{
		ID:           "tc-1",
		Name:         "search",
		ErrorMessage: "sensitive error detail",
	}

	encrypted, events, err := enc.EncryptToolCall(context.Background(), tc)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(encrypted.ErrorMessage, errorMessageEncPrefix),
		"expected enc:v1: prefix, got %q", encrypted.ErrorMessage)
	assert.NotContains(t, encrypted.ErrorMessage, "sensitive error detail")
	assert.True(t, eventFieldSet(events)[fieldErrorMessage])
}

func TestEncryptor_EncryptToolCall_Nil(t *testing.T) {
	enc := newTestEncryptor()
	encrypted, events, err := enc.EncryptToolCall(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, encrypted)
	assert.Nil(t, events)
}

func TestEncryptor_EncryptToolCall_EmptyFieldsSkipped(t *testing.T) {
	enc := newTestEncryptor()
	tc := &session.ToolCall{ID: "tc-1", Name: "noop"}
	encrypted, events, err := enc.EncryptToolCall(context.Background(), tc)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Nil(t, encrypted.Arguments)
	assert.Nil(t, encrypted.Result)
	assert.Equal(t, "", encrypted.ErrorMessage)
}

func TestEncryptor_DecryptToolCall_RoundTrip(t *testing.T) {
	enc := newTestEncryptor()
	ctx := context.Background()

	original := &session.ToolCall{
		ID:           "tc-1",
		Name:         "web_search",
		Arguments:    map[string]any{"query": "test", "count": float64(3)},
		Result:       "found it",
		ErrorMessage: "some error",
	}

	encrypted, _, err := enc.EncryptToolCall(ctx, original)
	require.NoError(t, err)

	decrypted, err := enc.DecryptToolCall(ctx, encrypted)
	require.NoError(t, err)

	assert.Equal(t, original.Name, decrypted.Name)
	assert.Equal(t, original.Arguments, decrypted.Arguments)
	assert.Equal(t, original.Result, decrypted.Result)
	assert.Equal(t, original.ErrorMessage, decrypted.ErrorMessage)
}

func TestEncryptor_DecryptToolCall_RoundTrip_MapResult(t *testing.T) {
	enc := newTestEncryptor()
	ctx := context.Background()

	original := &session.ToolCall{
		ID:        "tc-1",
		Name:      "web_search",
		Arguments: map[string]any{"query": "x"},
		Result:    map[string]any{"hits": float64(2), "top": "foo"},
	}

	encrypted, _, err := enc.EncryptToolCall(ctx, original)
	require.NoError(t, err)

	decrypted, err := enc.DecryptToolCall(ctx, encrypted)
	require.NoError(t, err)

	assert.Equal(t, original.Result, decrypted.Result)
}

func TestEncryptor_DecryptToolCall_Plaintext_PassesThrough(t *testing.T) {
	enc := newTestEncryptor()
	tc := &session.ToolCall{
		Name:      "search",
		Arguments: map[string]any{"q": "plaintext"},
		Result:    "plaintext result",
	}

	decrypted, err := enc.DecryptToolCall(context.Background(), tc)
	require.NoError(t, err)
	assert.Equal(t, tc.Arguments, decrypted.Arguments)
	assert.Equal(t, tc.Result, decrypted.Result)
	assert.Equal(t, "", decrypted.ErrorMessage)
}

func TestEncryptor_DecryptToolCall_Nil(t *testing.T) {
	enc := newTestEncryptor()
	out, err := enc.DecryptToolCall(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestEncryptor_EncryptToolCall_OriginalNotMutated(t *testing.T) {
	enc := newTestEncryptor()
	original := &session.ToolCall{
		ID:           "tc-1",
		Name:         "search",
		Arguments:    map[string]any{"q": "x"},
		Result:       "r",
		ErrorMessage: "e",
	}
	_, _, err := enc.EncryptToolCall(context.Background(), original)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"q": "x"}, original.Arguments)
	assert.Equal(t, "r", original.Result)
	assert.Equal(t, "e", original.ErrorMessage)
}

func TestEncryptor_EncryptRuntimeEvent_EncryptsData(t *testing.T) {
	enc := newTestEncryptor()

	evt := &session.RuntimeEvent{
		ID:           "evt-1",
		EventType:    "pipeline.completed",
		Data:         map[string]any{"output": "sensitive"},
		ErrorMessage: "sensitive error",
	}

	encrypted, events, err := enc.EncryptRuntimeEvent(context.Background(), evt)
	require.NoError(t, err)

	assert.Equal(t, "pipeline.completed", encrypted.EventType)
	assert.Contains(t, encrypted.Data, encryptionMetadataKey)
	assert.Contains(t, encrypted.Data, envelopePayloadKey)
	assert.True(t, strings.HasPrefix(encrypted.ErrorMessage, errorMessageEncPrefix))

	fields := eventFieldSet(events)
	assert.True(t, fields[fieldData])
	assert.True(t, fields[fieldErrorMessage])
}

func TestEncryptor_EncryptRuntimeEvent_Nil(t *testing.T) {
	enc := newTestEncryptor()
	encrypted, events, err := enc.EncryptRuntimeEvent(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, encrypted)
	assert.Nil(t, events)
}

func TestEncryptor_EncryptRuntimeEvent_EmptyFieldsSkipped(t *testing.T) {
	enc := newTestEncryptor()
	evt := &session.RuntimeEvent{ID: "e-1", EventType: "pipeline.started"}
	encrypted, events, err := enc.EncryptRuntimeEvent(context.Background(), evt)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Nil(t, encrypted.Data)
	assert.Equal(t, "", encrypted.ErrorMessage)
}

func TestEncryptor_DecryptRuntimeEvent_RoundTrip(t *testing.T) {
	enc := newTestEncryptor()
	ctx := context.Background()

	original := &session.RuntimeEvent{
		ID:           "evt-1",
		EventType:    "pipeline.completed",
		Data:         map[string]any{"output": "hello", "count": float64(5)},
		ErrorMessage: "failed",
	}

	encrypted, _, err := enc.EncryptRuntimeEvent(ctx, original)
	require.NoError(t, err)

	decrypted, err := enc.DecryptRuntimeEvent(ctx, encrypted)
	require.NoError(t, err)

	assert.Equal(t, original.EventType, decrypted.EventType)
	assert.Equal(t, original.Data, decrypted.Data)
	assert.Equal(t, original.ErrorMessage, decrypted.ErrorMessage)
}

func TestEncryptor_DecryptRuntimeEvent_Plaintext_PassesThrough(t *testing.T) {
	enc := newTestEncryptor()
	evt := &session.RuntimeEvent{
		EventType: "pipeline.completed",
		Data:      map[string]any{"key": "plaintext value"},
	}

	decrypted, err := enc.DecryptRuntimeEvent(context.Background(), evt)
	require.NoError(t, err)
	assert.Equal(t, evt.Data, decrypted.Data)
	assert.Equal(t, "", decrypted.ErrorMessage)
}

func TestEncryptor_DecryptRuntimeEvent_Nil(t *testing.T) {
	enc := newTestEncryptor()
	out, err := enc.DecryptRuntimeEvent(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestEncryptor_DecryptErrorMessage_InvalidBase64(t *testing.T) {
	e := &encryptor{provider: newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")}
	_, err := e.decryptErrorMessage(context.Background(), errorMessageEncPrefix+"!!!not-base64!!!")
	assert.Error(t, err)
}

func TestEncryptor_DecryptEnvelope_MissingPayload(t *testing.T) {
	e := &encryptor{provider: newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")}
	var out any
	err := e.decryptEnvelope(context.Background(), map[string]any{encryptionMetadataKey: map[string]any{}}, &out)
	assert.Error(t, err)
}

func TestEncryptor_DecryptEnvelope_InvalidBase64(t *testing.T) {
	e := &encryptor{provider: newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")}
	var out any
	err := e.decryptEnvelope(context.Background(), map[string]any{
		encryptionMetadataKey: map[string]any{},
		envelopePayloadKey:    "!!!bad!!!",
	}, &out)
	assert.Error(t, err)
}

func TestEncryptor_EncryptEnvelope_MarshalError(t *testing.T) {
	e := &encryptor{provider: &failingProvider{}}
	// math.NaN is not JSON-marshalable.
	_, _, err := e.encryptEnvelope(context.Background(), math.NaN())
	assert.Error(t, err)
}

func TestEncryptor_EncryptEnvelope_ProviderError(t *testing.T) {
	e := &encryptor{provider: &failingProvider{encryptErr: errors.New("boom")}}
	_, _, err := e.encryptEnvelope(context.Background(), map[string]any{"a": 1})
	assert.Error(t, err)
}

func TestEncryptor_EncryptErrorMessage_ProviderError(t *testing.T) {
	e := &encryptor{provider: &failingProvider{encryptErr: errors.New("boom")}}
	_, _, err := e.encryptErrorMessage(context.Background(), "oops")
	assert.Error(t, err)
}

func TestEncryptor_EncryptToolCall_ProviderError(t *testing.T) {
	e := NewEncryptor(&failingProvider{encryptErr: errors.New("boom")})
	_, _, err := e.EncryptToolCall(context.Background(), &session.ToolCall{Arguments: map[string]any{"x": 1}})
	assert.Error(t, err)
	_, _, err = e.EncryptToolCall(context.Background(), &session.ToolCall{Result: "x"})
	assert.Error(t, err)
	_, _, err = e.EncryptToolCall(context.Background(), &session.ToolCall{ErrorMessage: "x"})
	assert.Error(t, err)
}

func TestEncryptor_EncryptRuntimeEvent_ProviderError(t *testing.T) {
	e := NewEncryptor(&failingProvider{encryptErr: errors.New("boom")})
	_, _, err := e.EncryptRuntimeEvent(context.Background(), &session.RuntimeEvent{Data: map[string]any{"x": 1}})
	assert.Error(t, err)
	_, _, err = e.EncryptRuntimeEvent(context.Background(), &session.RuntimeEvent{ErrorMessage: "x"})
	assert.Error(t, err)
}

func TestEncryptor_DecryptToolCall_ProviderError(t *testing.T) {
	// Build an envelope with a real provider, then decrypt with a failing provider.
	good := newTestEncryptor()
	tc, _, err := good.EncryptToolCall(context.Background(), &session.ToolCall{
		Arguments: map[string]any{"x": 1},
	})
	require.NoError(t, err)

	bad := NewEncryptor(&failingProvider{decryptErr: errors.New("nope")})
	_, err = bad.DecryptToolCall(context.Background(), tc)
	assert.Error(t, err)
}

func TestEncryptor_DecryptRuntimeEvent_ErrorMessageDecryptFails(t *testing.T) {
	bad := NewEncryptor(&failingProvider{decryptErr: errors.New("nope")})
	_, err := bad.DecryptRuntimeEvent(context.Background(), &session.RuntimeEvent{
		ErrorMessage: errorMessageEncPrefix + "AAAA",
	})
	assert.Error(t, err)
}

func TestEncryptor_DecryptToolCall_ResultEnvelopeFails(t *testing.T) {
	good := newTestEncryptor()
	tc, _, err := good.EncryptToolCall(context.Background(), &session.ToolCall{
		Result: "x",
	})
	require.NoError(t, err)

	bad := NewEncryptor(&failingProvider{decryptErr: errors.New("nope")})
	_, err = bad.DecryptToolCall(context.Background(), tc)
	assert.Error(t, err)
}

func TestIsEnvelopeMap(t *testing.T) {
	assert.False(t, isEnvelopeMap(nil))
	assert.False(t, isEnvelopeMap(map[string]any{}))
	assert.False(t, isEnvelopeMap(map[string]any{encryptionMetadataKey: "x"}))
	assert.False(t, isEnvelopeMap(map[string]any{envelopePayloadKey: "x"}))
	assert.True(t, isEnvelopeMap(map[string]any{
		encryptionMetadataKey: map[string]any{},
		envelopePayloadKey:    "x",
	}))
}
