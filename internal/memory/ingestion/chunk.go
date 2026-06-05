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

package ingestion

import (
	"context"
	"strings"
)

// ChunkStrategy splits a document into verbatim, overlapping word windows —
// the traditional-RAG point on the spectrum. Pure CPU; no provider needed.
type ChunkStrategy struct {
	size    int // words per chunk
	overlap int // words shared between adjacent chunks
}

// NewChunkStrategy builds a ChunkStrategy. size<=0 defaults to 200; overlap is
// clamped to [0, size-1] so stride is always positive.
func NewChunkStrategy(size, overlap int) *ChunkStrategy {
	if size <= 0 {
		size = 200
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size - 1
	}
	return &ChunkStrategy{size: size, overlap: overlap}
}

// Ingest returns one Item per window. Empty/whitespace text yields no items.
func (c *ChunkStrategy) Ingest(_ context.Context, doc SourceDoc) ([]Item, error) {
	words := strings.Fields(doc.Text)
	if len(words) == 0 {
		return nil, nil
	}
	stride := c.size - c.overlap
	var items []Item
	for start, idx := 0, 0; start < len(words); start, idx = start+stride, idx+1 {
		end := start + c.size
		if end > len(words) {
			end = len(words)
		}
		items = append(items, Item{
			Content:  strings.Join(words[start:end], " "),
			Metadata: baseMetadata(doc, "chunk", idx),
		})
		if end == len(words) {
			break
		}
	}
	return items, nil
}
