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

package promptkit

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// spec.context.ttl is how long a conversation's working context survives in the
// context store between messages. These assert the configured value actually
// reaches the store, rather than being parsed and dropped at construction —
// the store was previously built with no options at all, so any value set on
// the CRD was silently replaced by PromptKit's own default.

func TestMemoryStore_HonoursConfiguredContextTTL(t *testing.T) {
	ctx := context.Background()

	store := statestore.NewMemoryStore(memoryStoreOptions(20 * time.Millisecond)...)
	t.Cleanup(store.Close)

	require.NoError(t, store.Save(ctx, &statestore.ConversationState{ID: "conv"}))

	// Present before the configured idle period elapses.
	state, err := store.Load(ctx, "conv")
	require.NoError(t, err)
	require.NotNil(t, state)

	// The memory store's TTL is idle time — Load refreshes LastAccessedAt — so
	// this must wait once rather than poll, or each poll would reset the clock.
	time.Sleep(60 * time.Millisecond)

	_, err = store.Load(ctx, "conv")
	require.Error(t, err,
		"context outlived spec.context.ttl, so the configured value never reached the store")
}

// A memory store's own default is one hour, so an entry must still be present
// well past the short TTL used above — proving the fallback is the store
// default rather than the last configured value leaking across.
func TestMemoryStore_NonPositiveTTLFallsBackToStoreDefault(t *testing.T) {
	ctx := context.Background()

	require.Nil(t, memoryStoreOptions(0))
	require.Nil(t, memoryStoreOptions(-time.Second))

	store := statestore.NewMemoryStore(memoryStoreOptions(0)...)
	t.Cleanup(store.Close)

	require.NoError(t, store.Save(ctx, &statestore.ConversationState{ID: "conv"}))
	time.Sleep(50 * time.Millisecond)

	state, err := store.Load(ctx, "conv")
	require.NoError(t, err)
	require.NotNil(t, state, "a non-positive TTL must not be forwarded as never-expire or expire-now")
}

func TestRedisStore_HonoursConfiguredContextTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	store := statestore.NewRedisStore(client, redisStoreOptions(30*time.Minute)...)

	require.NoError(t, store.Save(ctx, &statestore.ConversationState{ID: "conv"}))

	state, err := store.Load(ctx, "conv")
	require.NoError(t, err)
	require.NotNil(t, state)

	// Past the configured 30m but well inside PromptKit's 24h default: if the
	// configured value had been dropped, the entry would still be here.
	mr.FastForward(31 * time.Minute)

	_, err = store.Load(ctx, "conv")
	require.Error(t, err,
		"context outlived spec.context.ttl, so the configured value never reached the store")
}

func TestRedisStore_NonPositiveTTLFallsBackToStoreDefault(t *testing.T) {
	require.Nil(t, redisStoreOptions(0))
	require.Nil(t, redisStoreOptions(-time.Second))

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	store := statestore.NewRedisStore(client, redisStoreOptions(0)...)

	require.NoError(t, store.Save(ctx, &statestore.ConversationState{ID: "conv"}))
	mr.FastForward(31 * time.Minute)

	// Still present, because the store's own 24h default applies.
	state, err := store.Load(ctx, "conv")
	require.NoError(t, err)
	require.NotNil(t, state)
}
