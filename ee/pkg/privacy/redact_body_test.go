/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
)

func testPIIConfig() *omniav1alpha1.PIIConfig {
	return &omniav1alpha1.PIIConfig{
		Redact:   true,
		Patterns: []string{"ssn", "email"},
		Strategy: omniav1alpha1.RedactionStrategyReplace,
	}
}

func TestRedactRequestBody_NilBody(t *testing.T) {
	result, err := redactRequestBody(nil, "/messages", redaction.NewRedactor(), testPIIConfig())
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestRedactRequestBody_EmptyBody(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte{}))
	result, err := redactRequestBody(body, "/messages", redaction.NewRedactor(), testPIIConfig())
	assert.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.Empty(t, data)
}

func TestRedactRequestBody_DisabledPII(t *testing.T) {
	pii := &omniav1alpha1.PIIConfig{Redact: false}
	body := io.NopCloser(bytes.NewReader([]byte(`{"content":"secret"}`)))
	result, err := redactRequestBody(body, "/messages", redaction.NewRedactor(), pii)
	assert.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.Contains(t, string(data), "secret")
}

func TestRedactMessageBody_WithSSN(t *testing.T) {
	input := []byte(`{"content":"My SSN is 123-45-6789","role":"user"}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactMessageBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "123-45-6789")
	assert.Contains(t, string(result), "REDACTED")
}

func TestRedactToolCallBody_WithSSN(t *testing.T) {
	input := []byte(`{"name":"lookup","arguments":"SSN: 123-45-6789","result":"found user@example.com"}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactToolCallBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "123-45-6789")
	assert.NotContains(t, string(result), "user@example.com")
}

func TestRedactProviderCallBody(t *testing.T) {
	input := []byte(`{"request":"tell me about 123-45-6789","response":"SSN belongs to user@test.com"}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactProviderCallBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "123-45-6789")
	assert.NotContains(t, string(result), "user@test.com")
}

func TestRedactEventBody_MetadataValues(t *testing.T) {
	input := []byte(`{"type":"info","metadata":{"note":"contact user@test.com","safe":"no-pii"}}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactEventBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "user@test.com")
	assert.Contains(t, string(result), "no-pii")
}

func TestRedactEventBody_NoMetadata(t *testing.T) {
	input := []byte(`{"type":"info"}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactEventBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"info"}`, string(result))
}

func TestRedactEvalResultBody(t *testing.T) {
	input := []byte(`{"input":"SSN 123-45-6789","output":"found","expected":"match"}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactEvalResultBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.NotContains(t, string(result), "123-45-6789")
}

func TestRedactByEndpoint_UnknownPath(t *testing.T) {
	input := []byte(`{"data":"123-45-6789"}`)
	pii := testPIIConfig()
	r := redaction.NewRedactor()

	result, err := redactByEndpoint(input, "/api/v1/unknown", r, pii)
	require.NoError(t, err)
	// Unknown path should not be redacted
	assert.Contains(t, string(result), "123-45-6789")
}

func TestRedactStringField_MissingField(t *testing.T) {
	m := map[string]any{"other": "value"}
	r := redaction.NewRedactor()
	err := redactStringField(t.Context(), m, "missing", r, testPIIConfig())
	assert.NoError(t, err)
}

func TestRedactRequestBody_Messages(t *testing.T) {
	body := io.NopCloser(bytes.NewReader(
		[]byte(`{"content":"SSN is 123-45-6789","role":"user"}`),
	))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/sessions/abc/messages", r, testPIIConfig())
	require.NoError(t, err)

	data, _ := io.ReadAll(result)
	assert.NotContains(t, string(data), "123-45-6789")
}

func TestRedactRequestBody_ToolCalls(t *testing.T) {
	body := io.NopCloser(bytes.NewReader(
		[]byte(`{"arguments":"SSN 123-45-6789","result":"user@test.com"}`),
	))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/sessions/abc/tool-calls", r, testPIIConfig())
	require.NoError(t, err)

	data, _ := io.ReadAll(result)
	assert.NotContains(t, string(data), "123-45-6789")
	assert.NotContains(t, string(data), "user@test.com")
}

func TestRedactRequestBody_ProviderCalls(t *testing.T) {
	body := io.NopCloser(bytes.NewReader(
		[]byte(`{"request":"123-45-6789","response":"user@test.com"}`),
	))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/sessions/abc/provider-calls", r, testPIIConfig())
	require.NoError(t, err)

	data, _ := io.ReadAll(result)
	assert.NotContains(t, string(data), "123-45-6789")
}

func TestRedactRequestBody_NilRedactor(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte(`{"content":"secret"}`)))
	result, err := redactRequestBody(body, "/messages", nil, testPIIConfig())
	assert.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.Contains(t, string(data), "secret")
}

func TestRedactRequestBody_NilPII(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte(`{"content":"secret"}`)))
	result, err := redactRequestBody(body, "/messages", redaction.NewRedactor(), nil)
	assert.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.Contains(t, string(data), "secret")
}

func TestRedactRequestBody_Events(t *testing.T) {
	body := io.NopCloser(bytes.NewReader(
		[]byte(`{"type":"info","metadata":{"note":"user@test.com"}}`),
	))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/sessions/abc/events", r, testPIIConfig())
	require.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.NotContains(t, string(data), "user@test.com")
}

func TestRedactRequestBody_EvalResults(t *testing.T) {
	body := io.NopCloser(bytes.NewReader(
		[]byte(`{"input":"123-45-6789","output":"found","expected":"match"}`),
	))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/eval-results", r, testPIIConfig())
	require.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.NotContains(t, string(data), "123-45-6789")
}

func TestRedactRequestBody_Evaluate(t *testing.T) {
	body := io.NopCloser(bytes.NewReader(
		[]byte(`{"input":"user@test.com"}`),
	))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/sessions/abc/evaluate", r, testPIIConfig())
	require.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.NotContains(t, string(data), "user@test.com")
}

func TestRedactToolCallBody_BothFields(t *testing.T) {
	input := []byte(`{"name":"tool","arguments":"hello","result":"world"}`)
	pii := &omniav1alpha1.PIIConfig{Redact: true, Patterns: []string{"ssn"}}
	r := redaction.NewRedactor()
	result, err := redactToolCallBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.Contains(t, string(result), "hello") // no PII, so no change
}

func TestRedactProviderCallBody_BothFields(t *testing.T) {
	input := []byte(`{"request":"hello","response":"world"}`)
	pii := &omniav1alpha1.PIIConfig{Redact: true, Patterns: []string{"ssn"}}
	r := redaction.NewRedactor()
	result, err := redactProviderCallBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.Contains(t, string(result), "hello")
}

func TestRedactEvalResultBody_AllFields(t *testing.T) {
	input := []byte(`{"input":"in","output":"out","expected":"exp"}`)
	pii := &omniav1alpha1.PIIConfig{Redact: true, Patterns: []string{"ssn"}}
	r := redaction.NewRedactor()
	result, err := redactEvalResultBody(t.Context(), input, r, pii)
	require.NoError(t, err)
	assert.Contains(t, string(result), "in")
}

func TestRedactMessageBody_InvalidJSON(t *testing.T) {
	_, err := redactMessageBody(t.Context(), []byte(`not json`), redaction.NewRedactor(), testPIIConfig())
	assert.Error(t, err)
}

func TestRedactToolCallBody_InvalidJSON(t *testing.T) {
	_, err := redactToolCallBody(t.Context(), []byte(`not json`), redaction.NewRedactor(), testPIIConfig())
	assert.Error(t, err)
}

func TestRedactProviderCallBody_InvalidJSON(t *testing.T) {
	_, err := redactProviderCallBody(t.Context(), []byte(`not json`), redaction.NewRedactor(), testPIIConfig())
	assert.Error(t, err)
}

func TestRedactEventBody_InvalidJSON(t *testing.T) {
	_, err := redactEventBody(t.Context(), []byte(`not json`), redaction.NewRedactor(), testPIIConfig())
	assert.Error(t, err)
}

func TestRedactEvalResultBody_InvalidJSON(t *testing.T) {
	_, err := redactEvalResultBody(t.Context(), []byte(`not json`), redaction.NewRedactor(), testPIIConfig())
	assert.Error(t, err)
}

func TestRedactRequestBody_UnknownPath(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte(`{"data":"123-45-6789"}`)))
	r := redaction.NewRedactor()
	result, err := redactRequestBody(body, "/api/v1/sessions/abc/unknown", r, testPIIConfig())
	require.NoError(t, err)
	data, _ := io.ReadAll(result)
	assert.Contains(t, string(data), "123-45-6789")
}

func TestRedactStringField_NonStringField(t *testing.T) {
	m := map[string]any{"count": 42}
	r := redaction.NewRedactor()
	err := redactStringField(t.Context(), m, "count", r, testPIIConfig())
	assert.NoError(t, err)
	assert.Equal(t, 42, m["count"]) // unchanged
}
