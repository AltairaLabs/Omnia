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
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session/httpclient"
)

type mockPolicyFetcher struct {
	policy    *httpclient.PrivacyPolicyResponse
	err       error
	callCount atomic.Int32
}

func (m *mockPolicyFetcher) GetPrivacyPolicy(_ context.Context, _, _ string) (*httpclient.PrivacyPolicyResponse, error) {
	m.callCount.Add(1)
	return m.policy, m.err
}

func TestRecordingPolicyCache_FetchesOnFirstCall(t *testing.T) {
	policy := &httpclient.PrivacyPolicyResponse{}
	policy.Recording.Enabled = true
	policy.Recording.RichData = false

	fetcher := &mockPolicyFetcher{policy: policy}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second, logr.Discard())

	got := cache.Get(context.Background())
	require.NotNil(t, got)
	assert.True(t, got.Recording.Enabled)
	assert.False(t, got.Recording.RichData)
	assert.Equal(t, int32(1), fetcher.callCount.Load())
}

func TestRecordingPolicyCache_ReturnsCachedWithinTTL(t *testing.T) {
	policy := &httpclient.PrivacyPolicyResponse{}
	policy.Recording.Enabled = true
	fetcher := &mockPolicyFetcher{policy: policy}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second, logr.Discard())

	cache.Get(context.Background())
	cache.Get(context.Background())
	cache.Get(context.Background())

	assert.Equal(t, int32(1), fetcher.callCount.Load())
}

func TestRecordingPolicyCache_RefetchesAfterTTLExpiry(t *testing.T) {
	policy := &httpclient.PrivacyPolicyResponse{}
	policy.Recording.Enabled = true
	fetcher := &mockPolicyFetcher{policy: policy}
	// TTL of 1ms so it expires immediately
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 1*time.Millisecond, logr.Discard())

	cache.Get(context.Background())
	time.Sleep(5 * time.Millisecond)
	cache.Get(context.Background())

	assert.Equal(t, int32(2), fetcher.callCount.Load())
}

func TestRecordingPolicyCache_FetchError_DefaultsToRecordingEnabled(t *testing.T) {
	fetcher := &mockPolicyFetcher{err: errors.New("boom")}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second, logr.Discard())

	got := cache.Get(context.Background())
	require.NotNil(t, got)
	assert.True(t, got.Recording.Enabled)
	assert.True(t, got.Recording.RichData)
	assert.True(t, got.Recording.FacadeData)
}

func TestRecordingPolicyCache_NilPolicy_DefaultsToRecordingEnabled(t *testing.T) {
	fetcher := &mockPolicyFetcher{policy: nil}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second, logr.Discard())

	got := cache.Get(context.Background())
	require.NotNil(t, got)
	assert.True(t, got.Recording.Enabled)
	assert.True(t, got.Recording.RichData)
	assert.True(t, got.Recording.FacadeData)
}
