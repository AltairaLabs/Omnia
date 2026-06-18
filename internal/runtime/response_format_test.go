/*
Copyright 2026.

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

package runtime

import (
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveResponseFormat(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"report":{"type":"string"}}}`)

	t.Run("agent mode never sets a format", func(t *testing.T) {
		assert.Nil(t, resolveResponseFormat("agent", "json_schema", schema, "a"))
	})
	t.Run("function text sets no format", func(t *testing.T) {
		assert.Nil(t, resolveResponseFormat("function", "text", schema, "a"))
	})
	t.Run("function json sets json_object", func(t *testing.T) {
		rf := resolveResponseFormat("function", "json", schema, "a")
		require.NotNil(t, rf)
		assert.Equal(t, providers.ResponseFormatJSON, rf.Type)
		assert.Empty(t, rf.JSONSchema)
	})
	t.Run("function json_schema binds schema", func(t *testing.T) {
		rf := resolveResponseFormat("function", "json_schema", schema, "deep-research")
		require.NotNil(t, rf)
		assert.Equal(t, providers.ResponseFormatJSONSchema, rf.Type)
		assert.JSONEq(t, string(schema), string(rf.JSONSchema))
		assert.Equal(t, "deep-research", rf.SchemaName)
		assert.True(t, rf.Strict)
	})
	t.Run("function unset defaults to json_schema", func(t *testing.T) {
		rf := resolveResponseFormat("function", "", schema, "a")
		require.NotNil(t, rf)
		assert.Equal(t, providers.ResponseFormatJSONSchema, rf.Type)
	})
}
