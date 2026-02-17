/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
)

// aesGCMEncrypt encrypts plaintext with AES-256-GCM using the provided DEK.
// Returns nonce and ciphertext.
func aesGCMEncrypt(dek, plaintext []byte) (nonce, ciphertext []byte, err error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: AES cipher creation failed: %v", ErrEncryptionFailed, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: GCM creation failed: %v", ErrEncryptionFailed, err)
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("%w: failed to generate nonce: %v", ErrEncryptionFailed, err)
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return nonce, ciphertext, nil
}

// aesGCMDecrypt decrypts ciphertext with AES-256-GCM using the provided DEK and nonce.
func aesGCMDecrypt(dek, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("%w: AES cipher creation failed: %v", ErrDecryptionFailed, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: GCM creation failed: %v", ErrDecryptionFailed, err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: AES-GCM decryption failed: %v", ErrDecryptionFailed, err)
	}
	return plaintext, nil
}

// parseAndValidateEnvelope unmarshals and validates an envelope from ciphertext bytes.
func parseAndValidateEnvelope(data []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("%w: invalid envelope: %v", ErrDecryptionFailed, err)
	}
	if env.Version != envelopeVersion {
		return nil, fmt.Errorf("%w: unsupported envelope version: %d", ErrDecryptionFailed, env.Version)
	}
	return &env, nil
}

// sealEnvelope creates an envelope JSON from the components.
func sealEnvelope(wrappedDEK, nonce, ciphertext []byte, keyVersion string) ([]byte, error) {
	env := envelope{
		Version:    envelopeVersion,
		WrappedDEK: wrappedDEK,
		Nonce:      nonce,
		Ciphertext: ciphertext,
		KeyVersion: keyVersion,
	}
	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal envelope: %v", ErrEncryptionFailed, err)
	}
	return envBytes, nil
}
