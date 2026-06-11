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

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/internal/memory"
)

type fakeEmbProvider struct{ dim int }

func (f fakeEmbProvider) Embed(context.Context, []string) ([][]float32, error) { return nil, nil }
func (f fakeEmbProvider) Dimensions() int                                      { return f.dim }

func TestResolveEmbeddingDim_NoProviderUsesDefault(t *testing.T) {
	assert.Equal(t, defaultEmbeddingDimensions, resolveEmbeddingDim(nil))
}

func TestResolveEmbeddingDim_UsesProviderDimension(t *testing.T) {
	svc := memory.NewEmbeddingService(nil, fakeEmbProvider{dim: 768}, logr.Discard())
	assert.Equal(t, 768, resolveEmbeddingDim(svc))
}

func TestResolveEmbeddingDim_ZeroProviderDimFallsBack(t *testing.T) {
	svc := memory.NewEmbeddingService(nil, fakeEmbProvider{dim: 0}, logr.Discard())
	assert.Equal(t, defaultEmbeddingDimensions, resolveEmbeddingDim(svc))
}
