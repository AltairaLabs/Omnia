/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package api

// Encryptor encrypts/decrypts opaque byte slices. Implementations live
// outside this package (see ee/pkg/encryption).
type Encryptor interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// EncryptorResolver returns the Encryptor that should be used for the given
// session. Returns (nil, false) when no encryption applies — the caller must
// treat that as plaintext passthrough.
type EncryptorResolver interface {
	EncryptorForSession(sessionID string) (Encryptor, bool)
}

// EncryptorResolverFunc adapts a function to the EncryptorResolver interface.
type EncryptorResolverFunc func(sessionID string) (Encryptor, bool)

// EncryptorForSession implements EncryptorResolver.
func (f EncryptorResolverFunc) EncryptorForSession(sessionID string) (Encryptor, bool) {
	return f(sessionID)
}
