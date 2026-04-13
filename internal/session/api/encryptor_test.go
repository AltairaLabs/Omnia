/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
