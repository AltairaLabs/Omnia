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
	"sync"

	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session/api"
)

// EncryptionConfigForSession returns the EncryptionConfig that applies to the
// given session, or (nil, false) when no policy applies (plaintext).
type EncryptionConfigForSession func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool)

// EncryptorFactory builds an api.Encryptor from a fully specified EncryptionConfig.
// Implemented concretely in cmd/session-api/main.go wiring (Task 11) against
// ee/pkg/encryption.
type EncryptorFactory interface {
	Build(cfg omniav1alpha1.EncryptionConfig) (api.Encryptor, error)
}

type cacheKey struct{ provider, keyID string }

// PerPolicyEncryptorResolver implements api.EncryptorResolver by resolving the
// EncryptionConfig for each session and caching built encryptors keyed on
// (kmsProvider, keyID). Encryptor construction can be expensive (KMS init,
// credential reads) so amortising across sessions matters — but a single
// session-api instance typically serves one service group with at most a few
// overrides via AgentRuntime.privacyPolicyRef, so the cache stays small.
type PerPolicyEncryptorResolver struct {
	source  EncryptionConfigForSession
	factory EncryptorFactory
	cache   sync.Map // cacheKey -> api.Encryptor
	log     logr.Logger
}

// NewPerPolicyEncryptorResolver constructs a resolver wired to a policy source
// and an encryptor factory.
func NewPerPolicyEncryptorResolver(src EncryptionConfigForSession, f EncryptorFactory, log logr.Logger) *PerPolicyEncryptorResolver {
	return &PerPolicyEncryptorResolver{
		source:  src,
		factory: f,
		log:     log.WithName("encryption-resolver"),
	}
}

// EncryptorForSession implements api.EncryptorResolver.
func (r *PerPolicyEncryptorResolver) EncryptorForSession(sessionID string) (api.Encryptor, bool) {
	cfg, ok := r.source(sessionID)
	if !ok || cfg == nil || !cfg.Enabled {
		return nil, false
	}
	key := cacheKey{provider: string(cfg.KMSProvider), keyID: cfg.KeyID}
	if v, hit := r.cache.Load(key); hit {
		enc, _ := v.(api.Encryptor)
		return enc, enc != nil
	}
	enc, err := r.factory.Build(*cfg)
	if err != nil {
		r.log.Error(err, "encryptor build failed",
			"kmsProvider", cfg.KMSProvider, "keyID", cfg.KeyID)
		return nil, false
	}
	actual, _ := r.cache.LoadOrStore(key, enc)
	out, _ := actual.(api.Encryptor)
	return out, out != nil
}

// Invalidate drops the cached Encryptor for (provider, keyID). Callers (e.g.
// the PolicyWatcher reload hook) call this when an EncryptionConfig changes
// so the next EncryptorForSession rebuilds via the factory.
func (r *PerPolicyEncryptorResolver) Invalidate(provider, keyID string) {
	r.cache.Delete(cacheKey{provider: provider, keyID: keyID})
	r.log.V(1).Info("encryptor cache invalidated",
		"kmsProvider", provider, "keyID", keyID)
}
