/*
Copyright 2025-2026.

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

package facade

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session/httpclient"
)

// PolicyFetcher fetches the effective privacy policy for a namespace/agent.
// Implemented by *httpclient.Store.
type PolicyFetcher interface {
	GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*httpclient.PrivacyPolicyResponse, error)
}

// defaultRecordingPolicy is returned when no policy is found or the fetch fails.
// All recording is enabled to avoid silently dropping data on transient errors.
func defaultRecordingPolicy() *httpclient.PrivacyPolicyResponse {
	p := &httpclient.PrivacyPolicyResponse{}
	p.Recording.Enabled = true
	p.Recording.FacadeData = true
	p.Recording.RichData = true
	return p
}

// RecordingPolicyCache caches the effective privacy policy for a single
// namespace/agent pair (one cache per WebSocket session).
type RecordingPolicyCache struct {
	fetcher   PolicyFetcher
	namespace string
	agent     string
	ttl       time.Duration
	log       logr.Logger

	mu        sync.Mutex
	cached    *httpclient.PrivacyPolicyResponse
	fetchedAt time.Time
}

// NewRecordingPolicyCache creates a policy cache for one session.
func NewRecordingPolicyCache(
	fetcher PolicyFetcher, namespace, agent string, ttl time.Duration, log logr.Logger,
) *RecordingPolicyCache {
	return &RecordingPolicyCache{
		fetcher:   fetcher,
		namespace: namespace,
		agent:     agent,
		ttl:       ttl,
		log:       log.WithName("recording-policy-cache"),
	}
}

// Get returns the cached policy, refreshing if expired. Never returns nil.
func (c *RecordingPolicyCache) Get(ctx context.Context) *httpclient.PrivacyPolicyResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.fetchedAt) < c.ttl {
		return c.cached
	}

	policy, err := c.fetcher.GetPrivacyPolicy(ctx, c.namespace, c.agent)
	if err != nil {
		c.log.V(1).Info("privacy policy fetch failed, defaulting to recording enabled",
			"error", err.Error())
		c.cached = defaultRecordingPolicy()
	} else if policy == nil {
		c.cached = defaultRecordingPolicy()
	} else {
		c.cached = policy
	}
	c.fetchedAt = time.Now()
	return c.cached
}
