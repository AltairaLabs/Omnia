/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestNewRateLimiter_NilConfig(t *testing.T) {
	rl := NewRateLimiter(nil)
	require.NotNil(t, rl)
	assert.Equal(t, int32(DefaultMaxEvalsPerSecond), rl.MaxEvalsPerSecond())
	assert.Equal(t, int32(DefaultMaxConcurrentJudges), rl.MaxConcurrentJudgeCalls())
}

func TestNewRateLimiter_WithConfig(t *testing.T) {
	maxEvals := int32(100)
	maxJudges := int32(10)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond:       &maxEvals,
		MaxConcurrentJudgeCalls: &maxJudges,
	})
	assert.Equal(t, int32(100), rl.MaxEvalsPerSecond())
	assert.Equal(t, int32(10), rl.MaxConcurrentJudgeCalls())
}

func TestNewRateLimiter_PartialConfig(t *testing.T) {
	maxEvals := int32(25)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond: &maxEvals,
	})
	assert.Equal(t, int32(25), rl.MaxEvalsPerSecond())
	assert.Equal(t, int32(DefaultMaxConcurrentJudges), rl.MaxConcurrentJudgeCalls())
}

func TestNewRateLimiter_EmptyConfig(t *testing.T) {
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{})
	assert.Equal(t, int32(DefaultMaxEvalsPerSecond), rl.MaxEvalsPerSecond())
	assert.Equal(t, int32(DefaultMaxConcurrentJudges), rl.MaxConcurrentJudgeCalls())
}

func TestAcquire_Success(t *testing.T) {
	rl := NewRateLimiter(nil)
	ctx := context.Background()

	err := rl.Acquire(ctx)
	require.NoError(t, err)
}

func TestAcquire_CancelledContext(t *testing.T) {
	// Use a very low rate so the limiter blocks quickly.
	maxEvals := int32(1)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond: &maxEvals,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Consume the burst token.
	err := rl.Acquire(ctx)
	require.NoError(t, err)

	// Cancel immediately so the next Acquire fails.
	cancel()

	err = rl.Acquire(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgAcquireRateToken)
}

func TestAcquireJudge_Success(t *testing.T) {
	rl := NewRateLimiter(nil)
	ctx := context.Background()

	err := rl.AcquireJudge(ctx)
	require.NoError(t, err)
	rl.ReleaseJudge()
}

func TestAcquireJudge_SemaphoreLimitEnforced(t *testing.T) {
	maxJudges := int32(2)
	maxEvals := int32(100)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond:       &maxEvals,
		MaxConcurrentJudgeCalls: &maxJudges,
	})

	ctx := context.Background()

	// Acquire both semaphore slots.
	err := rl.AcquireJudge(ctx)
	require.NoError(t, err)
	err = rl.AcquireJudge(ctx)
	require.NoError(t, err)

	// Third acquire should block. Use a short timeout to verify.
	shortCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err = rl.AcquireJudge(shortCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgAcquireJudgeSemaphore)

	// Release one slot, then acquire should succeed.
	rl.ReleaseJudge()

	err = rl.AcquireJudge(ctx)
	require.NoError(t, err)

	// Clean up.
	rl.ReleaseJudge()
	rl.ReleaseJudge()
}

func TestAcquireJudge_CancelledContext_RateLimit(t *testing.T) {
	maxEvals := int32(1)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond: &maxEvals,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Consume the burst token via rate limiter.
	err := rl.AcquireJudge(ctx)
	require.NoError(t, err)
	rl.ReleaseJudge()

	// Cancel so the rate limiter wait fails.
	cancel()

	err = rl.AcquireJudge(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errMsgAcquireJudgeRateToken)
}

func TestReleaseJudge_AllowsNextAcquire(t *testing.T) {
	maxJudges := int32(1)
	maxEvals := int32(100)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond:       &maxEvals,
		MaxConcurrentJudgeCalls: &maxJudges,
	})

	ctx := context.Background()

	err := rl.AcquireJudge(ctx)
	require.NoError(t, err)

	// Release and re-acquire.
	rl.ReleaseJudge()

	err = rl.AcquireJudge(ctx)
	require.NoError(t, err)
	rl.ReleaseJudge()
}

func TestAcquire_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(nil) // 50 evals/sec burst

	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := rl.Acquire(ctx); err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(20), successCount.Load(),
		"all 20 acquires should succeed within burst capacity")
}

func TestAcquireJudge_ConcurrentAccess(t *testing.T) {
	maxJudges := int32(3)
	maxEvals := int32(100)
	rl := NewRateLimiter(&v1alpha1.EvalRateLimit{
		MaxEvalsPerSecond:       &maxEvals,
		MaxConcurrentJudgeCalls: &maxJudges,
	})

	ctx := context.Background()
	var wg sync.WaitGroup
	var concurrentCount atomic.Int32
	var maxConcurrent atomic.Int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := rl.AcquireJudge(ctx); err != nil {
				return
			}
			current := concurrentCount.Add(1)
			// Track maximum concurrent usage.
			for {
				old := maxConcurrent.Load()
				if current <= old || maxConcurrent.CompareAndSwap(old, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			concurrentCount.Add(-1)
			rl.ReleaseJudge()
		}()
	}

	wg.Wait()
	assert.LessOrEqual(t, maxConcurrent.Load(), int32(3),
		"concurrent judge calls should not exceed max")
}
