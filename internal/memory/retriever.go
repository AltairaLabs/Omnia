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
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
)

// OmniaRetriever implements memory retrieval by delegating to a RetrievalStrategy.
// Called by PromptKit's retrieval pipeline stage.
type OmniaRetriever struct {
	store    *PostgresMemoryStore
	strategy RetrievalStrategy
	limit    int
	log      logr.Logger
}

// NewOmniaRetriever creates a new OmniaRetriever.
func NewOmniaRetriever(store *PostgresMemoryStore, strategy RetrievalStrategy, limit int, log logr.Logger) *OmniaRetriever {
	return &OmniaRetriever{
		store:    store,
		strategy: strategy,
		limit:    limit,
		log:      log,
	}
}

// RetrieveContext finds relevant memories given conversation context.
// Extracts the last user message and delegates to the configured strategy.
func (r *OmniaRetriever) RetrieveContext(ctx context.Context, scope map[string]string, messages []types.Message) ([]*Memory, error) {
	query, ok := lastUserMessage(messages)
	if !ok {
		r.log.V(1).Info("retrieval skipped",
			"reason", "no user message",
			"messageCount", len(messages))
		return nil, nil
	}

	r.log.V(1).Info("retrieval starting",
		"strategy", r.strategy.Name(),
		"queryLength", len(query),
		"limit", r.limit)

	results, err := r.strategy.Retrieve(ctx, r.store.Pool(), scope, query, r.limit)
	if err != nil {
		return nil, fmt.Errorf("memory: retrieve via %s: %w", r.strategy.Name(), err)
	}

	r.log.V(1).Info("retrieval complete",
		"strategy", r.strategy.Name(),
		"resultCount", len(results))

	return results, nil
}

// lastUserMessage scans messages from the end and returns the last user message content.
func lastUserMessage(messages []types.Message) (string, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content, true
		}
	}
	return "", false
}
