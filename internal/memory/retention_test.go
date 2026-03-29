/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestNewRetentionWorker(t *testing.T) {
	store := newStore(t)
	log := zap.New(zap.UseDevMode(true))
	interval := 5 * time.Second

	w := NewRetentionWorker(store, interval, log)

	assert.NotNil(t, w)
	assert.Equal(t, store, w.store)
	assert.Equal(t, interval, w.interval)
}

func TestRetentionWorker_Run(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save a memory with past expiry.
	pastTime := time.Now().Add(-1 * time.Hour)
	mem := &Memory{
		Type:       "fact",
		Content:    "should be expired by worker",
		Confidence: 0.9,
		Scope:      scope,
		ExpiresAt:  &pastTime,
	}
	require.NoError(t, store.Save(ctx, mem))

	// Verify it exists before running the worker.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	log := zap.New(zap.UseDevMode(true))
	w := NewRetentionWorker(store, 10*time.Millisecond, log)

	cancelCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(cancelCtx)
	}()

	// Wait for at least one tick to fire.
	time.Sleep(100 * time.Millisecond)

	// Verify the expired memory is gone.
	results, err = store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results, "expired memory should have been deleted by retention worker")

	// Stop the worker.
	cancel()
	<-done
}
