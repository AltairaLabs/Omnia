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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDocTitle = "Runbook"

func testDoc() SourceDoc {
	return SourceDoc{Title: testDocTitle, URL: "https://sp/allowed/r.docx", Site: "allowed"}
}

func TestPostProcess_Summary_SingleItem(t *testing.T) {
	items := PostProcess("the condensed summary", Config{Strategy: StrategySummary}, testDoc())
	require.Len(t, items, 1)
	assert.Equal(t, "the condensed summary", items[0].Content)
	assert.Equal(t, KindSummary, items[0].Metadata[MetaKeyKind])
	assert.Equal(t, 0, items[0].Metadata[MetaKeyIndex])
}

func TestPostProcess_SummaryThenChunk_ChunksTheSummary(t *testing.T) {
	cfg := Config{Strategy: StrategySummaryThenChunk, ChunkSize: 2, ChunkOverlap: 0}
	items := PostProcess("alpha beta gamma delta", cfg, testDoc())
	require.Len(t, items, 2) // ["alpha beta", "gamma delta"]
	assert.Equal(t, "alpha beta", items[0].Content)
	assert.Equal(t, KindSummaryChunk, items[0].Metadata[MetaKeyKind])
	assert.Equal(t, 0, items[0].Metadata[MetaKeyIndex])
	assert.Equal(t, "gamma delta", items[1].Content)
	assert.Equal(t, KindSummaryChunk, items[1].Metadata[MetaKeyKind])
	assert.Equal(t, 1, items[1].Metadata[MetaKeyIndex])
}

func TestPostProcess_EmptySummary_NoItems(t *testing.T) {
	assert.Empty(t, PostProcess("   ", Config{Strategy: StrategySummary}, testDoc()))
	assert.Empty(t, PostProcess("", Config{Strategy: StrategySummaryThenChunk, ChunkSize: 2}, testDoc()))
}
