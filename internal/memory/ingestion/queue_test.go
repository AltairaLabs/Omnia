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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestQueue(t *testing.T) *FileSummaryQueue {
	t.Helper()
	q, err := NewFileSummaryQueue(t.TempDir())
	require.NoError(t, err)
	return q
}

func sampleItem() WorkItem {
	return WorkItem{
		WorkspaceID:  "ws-1",
		Doc:          SourceDoc{Title: testDocTitle, URL: "https://sp/allowed/r.docx", Site: "allowed", Text: "alpha beta gamma"},
		Strategy:     StrategySummary,
		ChunkSize:    200,
		ChunkOverlap: 40,
		AboutKey:     "https://sp/allowed/r.docx",
	}
}

func TestFileSummaryQueue_EnqueueListGet_RoundTrips(t *testing.T) {
	q := newTestQueue(t)
	require.NoError(t, q.Enqueue(context.Background(), sampleItem()))

	listed, err := q.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "alpha beta gamma", listed[0].Doc.Text)
	assert.Equal(t, StrategySummary, listed[0].Strategy)

	got, err := q.Get(context.Background(), "ws-1", "https://sp/allowed/r.docx")
	require.NoError(t, err)
	assert.Equal(t, testDocTitle, got.Doc.Title)
	assert.Equal(t, 200, got.ChunkSize)
}

func TestFileSummaryQueue_Enqueue_OverwritesSameKey(t *testing.T) {
	q := newTestQueue(t)
	it := sampleItem()
	require.NoError(t, q.Enqueue(context.Background(), it))
	it.Doc.Text = "rewritten"
	require.NoError(t, q.Enqueue(context.Background(), it))

	listed, err := q.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "rewritten", listed[0].Doc.Text)
}

func TestFileSummaryQueue_Complete_DeletesAndIsIdempotent(t *testing.T) {
	q := newTestQueue(t)
	require.NoError(t, q.Enqueue(context.Background(), sampleItem()))
	require.NoError(t, q.Complete(context.Background(), "ws-1", "https://sp/allowed/r.docx"))

	listed, err := q.List(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, listed)

	// second Complete on a missing item is a no-op
	require.NoError(t, q.Complete(context.Background(), "ws-1", "https://sp/allowed/r.docx"))
}

func TestFileSummaryQueue_Get_MissingReturnsNotFound(t *testing.T) {
	q := newTestQueue(t)
	_, err := q.Get(context.Background(), "ws-1", "nope")
	assert.True(t, errors.Is(err, ErrWorkItemNotFound))
}

func TestFileSummaryQueue_List_RespectsLimit(t *testing.T) {
	q := newTestQueue(t)
	for _, k := range []string{"a", "b", "c"} {
		it := sampleItem()
		it.AboutKey = k
		require.NoError(t, q.Enqueue(context.Background(), it))
	}
	listed, err := q.List(context.Background(), 2)
	require.NoError(t, err)
	assert.Len(t, listed, 2)
}

func TestFileSummaryQueue_List_SkipsCorruptFiles(t *testing.T) {
	q := newTestQueue(t)
	require.NoError(t, q.Enqueue(context.Background(), sampleItem()))
	require.NoError(t, os.WriteFile(filepath.Join(q.dir, "garbage.json"), []byte("{not json"), 0o600))

	listed, err := q.List(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, listed, 1) // the valid item; corrupt file skipped
}
