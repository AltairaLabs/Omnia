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
	"time"

	"github.com/go-logr/logr"
)

// RetentionWorker periodically expires memories past their TTL.
type RetentionWorker struct {
	store    *PostgresMemoryStore
	interval time.Duration
	log      logr.Logger
}

// NewRetentionWorker creates a new RetentionWorker with the given store, interval, and logger.
func NewRetentionWorker(store *PostgresMemoryStore, interval time.Duration, log logr.Logger) *RetentionWorker {
	return &RetentionWorker{
		store:    store,
		interval: interval,
		log:      log,
	}
}

// Run blocks until ctx is cancelled, expiring memories on each interval tick.
func (w *RetentionWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.log.Info("retention worker started", "interval", w.interval)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("retention worker stopped")
			return
		case <-ticker.C:
			expired, err := w.store.ExpireMemories(ctx)
			if err != nil {
				w.log.Error(err, "retention expiry failed")
				continue
			}
			if expired > 0 {
				w.log.Info("memories expired", "count", expired)
			}
		}
	}
}
