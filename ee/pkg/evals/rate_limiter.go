/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"fmt"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"golang.org/x/time/rate"
)

// Default rate limit values.
const (
	DefaultMaxEvalsPerSecond    = 50
	DefaultMaxConcurrentJudges  = 5
	errMsgAcquireRateToken      = "acquire rate limit token"
	errMsgAcquireJudgeRateToken = "acquire judge rate limit token"
	errMsgAcquireJudgeSemaphore = "acquire judge semaphore slot"
)

// RateLimiter controls eval execution throughput using a token bucket
// for overall rate and a semaphore for concurrent LLM judge calls.
type RateLimiter struct {
	limiter       *rate.Limiter
	judgeSem      chan struct{}
	maxPerSecond  int32
	maxConcurrent int32
}

// NewRateLimiter creates a RateLimiter from CRD config, applying defaults.
func NewRateLimiter(config *v1alpha1.EvalRateLimit) *RateLimiter {
	maxEvals := int32(DefaultMaxEvalsPerSecond)
	maxJudges := int32(DefaultMaxConcurrentJudges)

	if config != nil {
		if config.MaxEvalsPerSecond != nil {
			maxEvals = *config.MaxEvalsPerSecond
		}
		if config.MaxConcurrentJudgeCalls != nil {
			maxJudges = *config.MaxConcurrentJudgeCalls
		}
	}

	return &RateLimiter{
		limiter:       rate.NewLimiter(rate.Limit(maxEvals), int(maxEvals)),
		judgeSem:      make(chan struct{}, maxJudges),
		maxPerSecond:  maxEvals,
		maxConcurrent: maxJudges,
	}
}

// Acquire blocks until a rate limit token is available or the context is cancelled.
func (r *RateLimiter) Acquire(ctx context.Context) error {
	if err := r.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("%s: %w", errMsgAcquireRateToken, err)
	}
	return nil
}

// AcquireJudge acquires both a rate limit token and a judge semaphore slot.
// It blocks until both are available or the context is cancelled.
func (r *RateLimiter) AcquireJudge(ctx context.Context) error {
	if err := r.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("%s: %w", errMsgAcquireJudgeRateToken, err)
	}

	select {
	case r.judgeSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", errMsgAcquireJudgeSemaphore, ctx.Err())
	}
}

// ReleaseJudge releases a judge semaphore slot. Must be called after
// AcquireJudge returns successfully, typically via defer.
func (r *RateLimiter) ReleaseJudge() {
	<-r.judgeSem
}

// MaxEvalsPerSecond returns the configured maximum evals per second.
func (r *RateLimiter) MaxEvalsPerSecond() int32 {
	return r.maxPerSecond
}

// MaxConcurrentJudgeCalls returns the configured maximum concurrent judge calls.
func (r *RateLimiter) MaxConcurrentJudgeCalls() int32 {
	return r.maxConcurrent
}
