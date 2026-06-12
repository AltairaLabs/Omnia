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
	"errors"
)

// hybridFanout is the per-ranker candidate cap fed to the FTS and cosine
// rank lists before RRF fusion. Multi-tier truncates Go-side to req.Limit
// after ranking, so this only needs to be wide enough to feed the fusion.
const hybridFanout = 100

// rrfK is the Reciprocal Rank Fusion constant (Cormack 2009; k=60),
// matching hybridRetrieveSQL's single-tier path.
const rrfK = 60.0

// RetrieveMultiTierHybrid — see the Store interface. Implemented in a
// follow-up commit; this stub keeps the package compiling.
func (s *PostgresMemoryStore) RetrieveMultiTierHybrid(ctx context.Context, req MultiTierRequest, queryEmbedding []float32) (*MultiTierResult, error) {
	return nil, errors.New("memory: RetrieveMultiTierHybrid not implemented")
}
