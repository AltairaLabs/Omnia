/*
Copyright 2025.

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
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/tracing"
)

// runtimeDialMaxRetries / runtimeDialBackoffCap mirror the constants
// embedded in the legacy WebSocket-path retry loop. Extracted so both
// the WebSocket and the function-mode pod use the same dial semantics
// (review B3 / S2 on #1108).
const (
	runtimeDialMaxRetries  = 10
	runtimeDialInitialWait = 500 * time.Millisecond
	runtimeDialBackoffCap  = 5 * time.Second
	runtimeDialTimeout     = 5 * time.Second
)

// dialRuntime opens a RuntimeClient with exponential-backoff retries.
// The runtime sidecar may still be starting when the facade comes up,
// so the first few attempts routinely fail. dialRuntime returns the
// last error if every attempt within runtimeDialMaxRetries failed.
//
// Tests may override the retry/backoff via dialRuntimeTestOverride
// (see runtime_dial_test.go) to avoid 30+s loops.
func dialRuntime(cfg dialRuntimeConfig, log logr.Logger) (*facade.RuntimeClient, error) {
	if cfg.maxRetries == 0 {
		cfg.maxRetries = runtimeDialMaxRetries
	}
	if cfg.initialBackoff == 0 {
		cfg.initialBackoff = runtimeDialInitialWait
	}
	if cfg.backoffCap == 0 {
		cfg.backoffCap = runtimeDialBackoffCap
	}
	if cfg.dialTimeout == 0 {
		cfg.dialTimeout = runtimeDialTimeout
	}

	clientCfg := facade.RuntimeClientConfig{
		Address:     cfg.address,
		DialTimeout: cfg.dialTimeout,
		Log:         log,
	}
	if cfg.tracingProvider != nil {
		clientCfg.TracerProvider = cfg.tracingProvider.TracerProvider()
	}

	var lastErr error
	backoff := cfg.initialBackoff
	for i := 0; i < cfg.maxRetries; i++ {
		client, err := facade.NewRuntimeClient(clientCfg)
		if err == nil {
			log.Info("connected to runtime",
				"address", cfg.address, "attempt", i+1)
			return client, nil
		}
		lastErr = err
		log.Info("waiting for runtime to be ready",
			"address", cfg.address, "attempt", i+1, "error", err.Error())
		if i+1 < cfg.maxRetries {
			cfg.sleep(backoff)
			backoff *= 2
			if backoff > cfg.backoffCap {
				backoff = cfg.backoffCap
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("runtime dial failed without a specific error")
	}
	return nil, lastErr
}

// dialRuntimeConfig is the input bundle for dialRuntime. The sleep
// hook lets tests substitute a no-op so the retry loop runs fast.
type dialRuntimeConfig struct {
	address         string
	tracingProvider *tracing.Provider

	// Retry tuning. Zero values fall back to runtimeDial* defaults.
	maxRetries     int
	initialBackoff time.Duration
	backoffCap     time.Duration
	dialTimeout    time.Duration

	// sleep defaults to time.Sleep when nil.
	sleep func(time.Duration)
}

// newDialRuntimeConfig builds a dialRuntimeConfig with the production
// defaults (real time.Sleep). The caller may then override fields.
func newDialRuntimeConfig(address string, tp *tracing.Provider) dialRuntimeConfig {
	return dialRuntimeConfig{
		address:         address,
		tracingProvider: tp,
		sleep:           time.Sleep,
	}
}
