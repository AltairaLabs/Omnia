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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numberedWords(n int) string {
	ws := make([]string, n)
	for i := range ws {
		ws[i] = fmt.Sprintf("w%d", i)
	}
	return strings.Join(ws, " ")
}

func TestChunkStrategy_SplitsWithOverlap(t *testing.T) {
	s := NewChunkStrategy(100, 20) // stride 80
	items, err := s.Ingest(context.Background(), SourceDoc{
		Title: "Runbook", URL: "https://sp/allowed/runbook.docx", Site: "allowed",
		Text: numberedWords(250),
	})
	require.NoError(t, err)
	// Windows start at 0, 80, 160; the 160 window reaches the end (250) so we
	// stop — 3 chunks, no redundant trailing subset.
	require.Len(t, items, 3)
	for i, it := range items {
		assert.Equal(t, "chunk", it.Metadata["kind"])
		assert.Equal(t, i, it.Metadata["index"])
		assert.Equal(t, "https://sp/allowed/runbook.docx", it.Metadata["url"])
		assert.Equal(t, "Runbook", it.Metadata["title"])
		assert.Equal(t, "allowed", it.Metadata["site"])
		assert.NotEmpty(t, it.Content)
	}
	// Boundary checks: windows start at w0, w80, w160; the last reaches w249.
	assert.True(t, strings.HasPrefix(items[0].Content, "w0 "))
	assert.True(t, strings.HasPrefix(items[1].Content, "w80 "))
	assert.True(t, strings.HasPrefix(items[2].Content, "w160 "))
	assert.True(t, strings.HasSuffix(items[2].Content, " w249"))
}

func TestChunkStrategy_ShortDocSingleChunk(t *testing.T) {
	s := NewChunkStrategy(100, 20)
	items, err := s.Ingest(context.Background(), SourceDoc{Title: "t", URL: "u", Text: "a b c"})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "a b c", items[0].Content)
}

func TestChunkStrategy_EmptyText(t *testing.T) {
	s := NewChunkStrategy(100, 20)
	items, err := s.Ingest(context.Background(), SourceDoc{Title: "t", URL: "u", Text: "   "})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestNewChunkStrategy_Clamps(t *testing.T) {
	assert.Equal(t, 200, NewChunkStrategy(0, 0).size)       // size<=0 → default 200
	assert.Equal(t, 0, NewChunkStrategy(100, -5).overlap)   // overlap<0 → 0
	assert.Equal(t, 99, NewChunkStrategy(100, 100).overlap) // overlap>=size → size-1
}
