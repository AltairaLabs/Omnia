/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package api

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session/providers"
)

type stubEncryptor struct{ tag string }

func (s stubEncryptor) Encrypt(p []byte) ([]byte, error) {
	return append([]byte("E"+s.tag+":"), p...), nil
}

func (s stubEncryptor) Decrypt(c []byte) ([]byte, error) {
	prefixLen := len("E" + s.tag + ":")
	if len(c) < prefixLen {
		return c, nil
	}
	return c[prefixLen:], nil
}

func TestEncryptorResolverFunc_ReturnsEncryptor(t *testing.T) {
	var resolver EncryptorResolver = EncryptorResolverFunc(func(id string) (Encryptor, bool) {
		if id == "encrypted-session" {
			return stubEncryptor{tag: "A"}, true
		}
		return nil, false
	})

	enc, ok := resolver.EncryptorForSession("encrypted-session")
	require.True(t, ok)
	require.NotNil(t, enc)

	out, err := enc.Encrypt([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, []byte("EA:hello"), out)

	back, err := enc.Decrypt(out)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), back)
}

func TestEncryptorResolverFunc_ReturnsFalseForUnknownSession(t *testing.T) {
	resolver := EncryptorResolverFunc(func(_ string) (Encryptor, bool) {
		return nil, false
	})

	enc, ok := resolver.EncryptorForSession("plaintext-session")
	assert.False(t, ok)
	assert.Nil(t, enc)
}

func TestHandler_EncryptorResolver_NilByDefault(t *testing.T) {
	svc := NewSessionService(providers.NewRegistry(), ServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	assert.Nil(t, h.EncryptorResolver(), "resolver must be nil before SetEncryptorResolver")
}

func TestHandler_EncryptorResolver_ReturnedAfterSet(t *testing.T) {
	svc := NewSessionService(providers.NewRegistry(), ServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())

	r := EncryptorResolverFunc(func(_ string) (Encryptor, bool) { return nil, false })
	h.SetEncryptorResolver(r)

	assert.NotNil(t, h.EncryptorResolver(), "resolver must be non-nil after SetEncryptorResolver")
}
