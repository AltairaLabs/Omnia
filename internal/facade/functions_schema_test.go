/*
Copyright 2025.

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

package facade

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileSchema_Valid(t *testing.T) {
	s, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestCompileSchema_Empty(t *testing.T) {
	_, err := CompileSchema(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestCompileSchema_BadJSON(t *testing.T) {
	_, err := CompileSchema([]byte(`{not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestCompileSchema_InvalidSchema(t *testing.T) {
	// "type" must be a string or array of strings; an integer is rejected
	// during compile.
	_, err := CompileSchema([]byte(`{"type":42}`))
	require.Error(t, err)
}

func TestValidateJSON_HappyPath(t *testing.T) {
	s, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)

	assert.NoError(t, ValidateJSON(s, []byte(`{"q":"x"}`)))
}

func TestValidateJSON_NilSchema(t *testing.T) {
	err := ValidateJSON(nil, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestValidateJSON_EmptyPayload(t *testing.T) {
	s, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	err = ValidateJSON(s, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestValidateJSON_BadJSON(t *testing.T) {
	s, err := CompileSchema([]byte(`{"type":"object"}`))
	require.NoError(t, err)

	err = ValidateJSON(s, []byte(`{not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestValidateJSON_SchemaViolation(t *testing.T) {
	s, err := CompileSchema([]byte(`{"type":"object","required":["q"]}`))
	require.NoError(t, err)

	err = ValidateJSON(s, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not satisfy schema")
}
