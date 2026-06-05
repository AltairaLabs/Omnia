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

package access

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDenyFilter_DropsRestricted(t *testing.T) {
	f, err := NewDenyFilter(`metadata.url.contains("restricted")`)
	require.NoError(t, err)

	assert.True(t, f.Allowed(map[string]any{"url": "https://sp/allowed/r.docx"}))
	assert.False(t, f.Allowed(map[string]any{"url": "https://sp/restricted/secret.docx"}))
}

func TestDenyFilter_EmptyExprAllowsAll(t *testing.T) {
	f, err := NewDenyFilter("")
	require.NoError(t, err)
	assert.True(t, f.Allowed(map[string]any{"url": "https://sp/restricted/x"}))
}

func TestDenyFilter_InvalidExprFailsConstruction(t *testing.T) {
	_, err := NewDenyFilter(`metadata.url.nope(`)
	require.Error(t, err) // fail-closed at construction; caller refuses to serve
}

func TestDenyFilter_MissingKeyFailsClosed(t *testing.T) {
	f, err := NewDenyFilter(`metadata.url.contains("restricted")`)
	require.NoError(t, err)
	// no "url" key → eval errors → treated as denied (drop)
	assert.False(t, f.Allowed(map[string]any{"site": "allowed"}))
}

func TestDenyFilter_NonBoolResultFailsClosed(t *testing.T) {
	// Expression evaluates to a string, not a bool — must be treated as deny.
	f, err := NewDenyFilter(`metadata.url`)
	require.NoError(t, err)
	assert.False(t, f.Allowed(map[string]any{"url": "https://sp/allowed/r.docx"}),
		"non-bool CEL result must be denied (fail-closed)")
}

func TestDenyFilter_NilMetadataFailsClosed(t *testing.T) {
	f, err := NewDenyFilter(`metadata.url.contains("restricted")`)
	require.NoError(t, err)
	assert.False(t, f.Allowed(nil),
		"nil metadata must be denied (fail-closed)")
}
