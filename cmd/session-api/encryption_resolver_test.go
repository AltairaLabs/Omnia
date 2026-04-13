/*
Copyright 2026.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session/api"
)

// stubEncryptor is a trivial api.Encryptor for tests.
type stubEncryptor struct{ tag string }

func (s stubEncryptor) Encrypt(p []byte) ([]byte, error) {
	return append([]byte(s.tag+":"), p...), nil
}
func (s stubEncryptor) Decrypt(c []byte) ([]byte, error) {
	prefix := s.tag + ":"
	if len(c) < len(prefix) {
		return c, nil
	}
	return c[len(prefix):], nil
}

// countingFactory counts Build calls and tags each encryptor with the config
// so we can assert which one was returned.
type countingFactory struct {
	builds atomic.Int32
	failOn string
}

func (f *countingFactory) Build(cfg omniav1alpha1.EncryptionConfig) (api.Encryptor, error) {
	f.builds.Add(1)
	if f.failOn != "" && cfg.KeyID == f.failOn {
		return nil, errors.New("kms init failed")
	}
	return stubEncryptor{tag: string(cfg.KMSProvider) + "-" + cfg.KeyID}, nil
}

func enabledCfg(provider omniav1alpha1.KMSProvider, keyID string) *omniav1alpha1.EncryptionConfig {
	return &omniav1alpha1.EncryptionConfig{
		Enabled: true, KMSProvider: provider, KeyID: keyID,
	}
}

func TestPerPolicyEncryptorResolver_NoPolicy(t *testing.T) {
	factory := &countingFactory{}
	src := func(string) (*omniav1alpha1.EncryptionConfig, bool) { return nil, false }
	r := NewPerPolicyEncryptorResolver(src, factory, testr.New(t))
	enc, ok := r.EncryptorForSession("any")
	assert.False(t, ok)
	assert.Nil(t, enc)
	assert.Zero(t, factory.builds.Load())
}

func TestPerPolicyEncryptorResolver_Disabled(t *testing.T) {
	factory := &countingFactory{}
	src := func(string) (*omniav1alpha1.EncryptionConfig, bool) {
		return &omniav1alpha1.EncryptionConfig{Enabled: false}, true
	}
	r := NewPerPolicyEncryptorResolver(src, factory, testr.New(t))
	enc, ok := r.EncryptorForSession("any")
	assert.False(t, ok)
	assert.Nil(t, enc)
	assert.Zero(t, factory.builds.Load())
}

func TestPerPolicyEncryptorResolver_CachesBySamePair(t *testing.T) {
	factory := &countingFactory{}
	src := func(string) (*omniav1alpha1.EncryptionConfig, bool) {
		return enabledCfg("aws-kms", "k1"), true
	}
	r := NewPerPolicyEncryptorResolver(src, factory, testr.New(t))

	_, ok := r.EncryptorForSession("s1")
	require.True(t, ok)
	_, ok = r.EncryptorForSession("s2")
	require.True(t, ok)

	assert.Equal(t, int32(1), factory.builds.Load(), "expected one build for one (provider,keyID) pair")
}

func TestPerPolicyEncryptorResolver_DistinctPairsEachBuildOnce(t *testing.T) {
	factory := &countingFactory{}
	cfgs := map[string]*omniav1alpha1.EncryptionConfig{
		"s-aws":    enabledCfg("aws-kms", "k1"),
		"s-azure":  enabledCfg("azure-keyvault", "k1"),
		"s-aws-k2": enabledCfg("aws-kms", "k2"),
	}
	src := func(id string) (*omniav1alpha1.EncryptionConfig, bool) { return cfgs[id], true }
	r := NewPerPolicyEncryptorResolver(src, factory, testr.New(t))

	for id := range cfgs {
		_, ok := r.EncryptorForSession(id)
		require.True(t, ok, "expected encryptor for %s", id)
	}
	// Second pass: all cache hits.
	for id := range cfgs {
		_, ok := r.EncryptorForSession(id)
		require.True(t, ok)
	}

	assert.Equal(t, int32(3), factory.builds.Load(), "one build per distinct (provider,keyID) pair")
}

func TestPerPolicyEncryptorResolver_FactoryErrorReturnsFalse(t *testing.T) {
	factory := &countingFactory{failOn: "bad-key"}
	src := func(string) (*omniav1alpha1.EncryptionConfig, bool) {
		return enabledCfg("aws-kms", "bad-key"), true
	}
	r := NewPerPolicyEncryptorResolver(src, factory, testr.New(t))

	enc, ok := r.EncryptorForSession("s1")
	assert.False(t, ok)
	assert.Nil(t, enc)
	// Retrying should call Build again — failure is not cached.
	enc, ok = r.EncryptorForSession("s1")
	assert.False(t, ok)
	assert.Nil(t, enc)
	assert.Equal(t, int32(2), factory.builds.Load())
}

func TestPerPolicyEncryptorResolver_InvalidateForcesRebuild(t *testing.T) {
	factory := &countingFactory{}
	src := func(string) (*omniav1alpha1.EncryptionConfig, bool) {
		return enabledCfg("aws-kms", "k1"), true
	}
	r := NewPerPolicyEncryptorResolver(src, factory, testr.New(t))

	_, _ = r.EncryptorForSession("s1")
	require.Equal(t, int32(1), factory.builds.Load())

	r.Invalidate("aws-kms", "k1")

	_, _ = r.EncryptorForSession("s1")
	assert.Equal(t, int32(2), factory.builds.Load())
}
