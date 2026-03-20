/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
)

// redactRequestBody reads the request body, redacts PII fields based on the
// endpoint path, and returns a new ReadCloser with the redacted content.
func redactRequestBody(
	body io.ReadCloser,
	path string,
	redactor redaction.Redactor,
	pii *omniav1alpha1.PIIConfig,
) (io.ReadCloser, error) {
	if body == nil || redactor == nil || pii == nil || !pii.Redact {
		return body, nil
	}

	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}

	if len(data) == 0 {
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	redacted, err := redactByEndpoint(data, path, redactor, pii)
	if err != nil {
		// On redaction error, return original body to avoid data loss.
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	return io.NopCloser(bytes.NewReader(redacted)), nil
}

// redactByEndpoint selects the appropriate redaction strategy based on URL path.
func redactByEndpoint(
	data []byte,
	path string,
	redactor redaction.Redactor,
	pii *omniav1alpha1.PIIConfig,
) ([]byte, error) {
	ctx := context.Background()

	switch {
	case strings.HasSuffix(path, "/messages"):
		return redactMessageBody(ctx, data, redactor, pii)
	case strings.HasSuffix(path, "/tool-calls"):
		return redactToolCallBody(ctx, data, redactor, pii)
	case strings.HasSuffix(path, "/provider-calls"):
		return redactProviderCallBody(ctx, data, redactor, pii)
	case strings.HasSuffix(path, "/events"):
		return redactEventBody(ctx, data, redactor, pii)
	case strings.Contains(path, "/eval-results") || strings.HasSuffix(path, "/evaluate"):
		return redactEvalResultBody(ctx, data, redactor, pii)
	default:
		return data, nil
	}
}

// redactMessageBody redacts the "content" field of a message.
func redactMessageBody(
	ctx context.Context, data []byte, r redaction.Redactor, pii *omniav1alpha1.PIIConfig,
) ([]byte, error) {
	return redactFields(ctx, data, r, pii, "content")
}

// redactToolCallBody redacts "arguments" and "result" fields.
func redactToolCallBody(
	ctx context.Context, data []byte, r redaction.Redactor, pii *omniav1alpha1.PIIConfig,
) ([]byte, error) {
	return redactFields(ctx, data, r, pii, "arguments", "result")
}

// redactProviderCallBody redacts "request" and "response" fields.
func redactProviderCallBody(
	ctx context.Context, data []byte, r redaction.Redactor, pii *omniav1alpha1.PIIConfig,
) ([]byte, error) {
	return redactFields(ctx, data, r, pii, "request", "response")
}

// redactFields deserializes JSON, redacts the named fields, and re-serializes.
func redactFields(
	ctx context.Context, data []byte, r redaction.Redactor,
	pii *omniav1alpha1.PIIConfig, fields ...string,
) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	for _, field := range fields {
		if err := redactStringField(ctx, m, field, r, pii); err != nil {
			return nil, err
		}
	}
	return json.Marshal(m)
}

// redactEventBody redacts values in the "metadata" map.
func redactEventBody(
	ctx context.Context, data []byte, r redaction.Redactor, pii *omniav1alpha1.PIIConfig,
) ([]byte, error) {
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	meta, ok := event["metadata"]
	if !ok {
		return data, nil
	}

	metaMap, ok := meta.(map[string]any)
	if !ok {
		return data, nil
	}

	for k, v := range metaMap {
		s, ok := v.(string)
		if !ok {
			continue
		}
		redacted, _, err := r.Redact(ctx, s, pii)
		if err != nil {
			return nil, err
		}
		metaMap[k] = redacted
	}
	event["metadata"] = metaMap
	return json.Marshal(event)
}

// redactEvalResultBody redacts "input", "output", and "expected" fields.
func redactEvalResultBody(
	ctx context.Context, data []byte, r redaction.Redactor, pii *omniav1alpha1.PIIConfig,
) ([]byte, error) {
	return redactFields(ctx, data, r, pii, "input", "output", "expected")
}

// redactStringField redacts a single string field in a JSON map.
func redactStringField(
	ctx context.Context, m map[string]any, key string,
	r redaction.Redactor, pii *omniav1alpha1.PIIConfig,
) error {
	v, ok := m[key]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	redacted, _, err := r.Redact(ctx, s, pii)
	if err != nil {
		return err
	}
	m[key] = redacted
	return nil
}
