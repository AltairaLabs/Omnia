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

package api

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// fixedSearchStore embeds mockMemoryStore and overrides Retrieve to return
// canned memories. embeddingSvc is nil so searchMemoriesInner routes to the
// FTS Retrieve path, which fixedSearchStore intercepts.
type fixedSearchStore struct {
	mockMemoryStore
	out []*memory.Memory
}

func (f *fixedSearchStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ memory.RetrieveOptions) ([]*memory.Memory, error) {
	return f.out, nil
}

func TestRetrieveSemantic_AppliesDenyFilter(t *testing.T) {
	store := &fixedSearchStore{out: []*memory.Memory{
		{ID: "a", Content: "allowed chunk", Metadata: map[string]any{"url": "https://sp/allowed/r.docx"}},
		{ID: "b", Content: "secret chunk", Metadata: map[string]any{"url": "https://sp/restricted/s.docx"}},
	}}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())

	got, err := svc.RetrieveSemantic(context.Background(), "ws-1", "failover",
		`metadata.url.contains("restricted")`, 5)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

func TestRetrieveSemantic_NoFilterReturnsAll(t *testing.T) {
	store := &fixedSearchStore{out: []*memory.Memory{
		{ID: "a", Metadata: map[string]any{"url": "u1"}},
		{ID: "b", Metadata: map[string]any{"url": "u2"}},
	}}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	got, err := svc.RetrieveSemantic(context.Background(), "ws-1", "q", "", 5)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestRetrieveSemantic_InvalidCELErrors(t *testing.T) {
	svc := NewMemoryService(&fixedSearchStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	_, err := svc.RetrieveSemantic(context.Background(), "ws-1", "q", "metadata.url.bad(", 5)
	require.Error(t, err) // fail-closed: refuse to serve on bad policy
}
