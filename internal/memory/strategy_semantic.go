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

	"github.com/jackc/pgx/v5/pgxpool"
)

// SemanticStrategy is a stub for vector-embedding-based retrieval.
// It returns an error until the embedding pipeline is configured.
type SemanticStrategy struct{}

// Name returns the strategy identifier.
func (s *SemanticStrategy) Name() string { return "semantic" }

// Retrieve always returns an error indicating embeddings are not configured.
func (s *SemanticStrategy) Retrieve(_ context.Context, _ *pgxpool.Pool, _ map[string]string, _ string, _ int) ([]*Memory, error) {
	return nil, fmt.Errorf("memory: semantic retrieval requires embedding pipeline (not yet configured)")
}
