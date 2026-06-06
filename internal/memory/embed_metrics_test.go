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
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmbeddingProvider is a minimal EmbeddingProvider whose Embed result
// is configurable so the metered decorator's success/error paths can be
// exercised.
type fakeEmbeddingProvider struct {
	vecs [][]float32
	err  error
}

func (f *fakeEmbeddingProvider) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return f.vecs, f.err
}

func (f *fakeEmbeddingProvider) Dimensions() int { return 3 }

func TestMeteredEmbeddingProvider_RecordsSuccess(t *testing.T) {
	inner := &fakeEmbeddingProvider{vecs: [][]float32{{0.1, 0.2, 0.3}}}
	p := NewMeteredEmbeddingProvider(inner)

	before := testutil.ToFloat64(embedRequestsTotal.WithLabelValues(embedOutcomeSuccess))
	vecs, err := p.Embed(context.Background(), []string{"hello"})
	require.NoError(t, err)
	assert.Len(t, vecs, 1)
	assert.Equal(t, 3, p.Dimensions())

	after := testutil.ToFloat64(embedRequestsTotal.WithLabelValues(embedOutcomeSuccess))
	assert.Equal(t, float64(1), after-before, "success outcome incremented")
}

func TestMeteredEmbeddingProvider_RecordsError(t *testing.T) {
	inner := &fakeEmbeddingProvider{err: errors.New("provider down")}
	p := NewMeteredEmbeddingProvider(inner)

	before := testutil.ToFloat64(embedRequestsTotal.WithLabelValues(embedOutcomeError))
	_, err := p.Embed(context.Background(), []string{"hello"})
	require.Error(t, err)

	after := testutil.ToFloat64(embedRequestsTotal.WithLabelValues(embedOutcomeError))
	assert.Equal(t, float64(1), after-before, "error outcome incremented")
}
