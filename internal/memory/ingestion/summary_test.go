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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSummarizer struct {
	out string
	err error
}

func (f fakeSummarizer) Summarize(_ context.Context, _ string) (string, error) {
	return f.out, f.err
}

func TestSummaryStrategy_OneItemWithSummaryMetadata(t *testing.T) {
	s := NewSummaryStrategy(fakeSummarizer{out: "a runbook for DB failover"})
	items, err := s.Ingest(context.Background(), SourceDoc{
		Title: "Runbook", URL: "https://sp/allowed/runbook.docx", Site: "allowed", Text: "long body...",
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "a runbook for DB failover", items[0].Content)
	assert.Equal(t, "summary", items[0].Metadata["kind"])
	assert.Equal(t, "https://sp/allowed/runbook.docx", items[0].Metadata["url"])
	assert.Equal(t, "Runbook", items[0].Metadata["title"])
	assert.Equal(t, "allowed", items[0].Metadata["site"])
	assert.Equal(t, 0, items[0].Metadata["index"])
}

func TestSummaryStrategy_PropagatesSummarizerError(t *testing.T) {
	s := NewSummaryStrategy(fakeSummarizer{err: errors.New("provider down")})
	_, err := s.Ingest(context.Background(), SourceDoc{Title: "t", URL: "u", Text: "body"})
	require.Error(t, err)
}

func TestSummaryStrategy_EmptyTextNoItems(t *testing.T) {
	s := NewSummaryStrategy(fakeSummarizer{out: "unused"})
	items, err := s.Ingest(context.Background(), SourceDoc{Title: "t", URL: "u", Text: "  "})
	require.NoError(t, err)
	assert.Empty(t, items)
}
