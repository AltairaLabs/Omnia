/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestAuditLogger_Log_EmitsStructuredEntry(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	audit := NewAuditLogger(logger)

	entry := AuditEntry{
		Decision: "deny",
		Policy:   "refund-policy",
		Rule:     "amount-limit",
		Agent:    "support-agent",
		Tool:     "process_refund",
		User:     "alice@example.com",
		Session:  "sess-123",
		Message:  "Amount exceeds limit",
		Mode:     "enforce",
	}

	audit.Log(entry)

	var logOutput map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logOutput); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	assertLogField(t, logOutput, "decision", "deny")
	assertLogField(t, logOutput, "policy", "refund-policy")
	assertLogField(t, logOutput, "rule", "amount-limit")
	assertLogField(t, logOutput, "agent", "support-agent")
	assertLogField(t, logOutput, "tool", "process_refund")
	assertLogField(t, logOutput, "user", "alice@example.com")
	assertLogField(t, logOutput, "session", "sess-123")
	assertLogField(t, logOutput, "message", "Amount exceeds limit")
	assertLogField(t, logOutput, "mode", "enforce")
}

func TestAuditLogger_Log_AllowDecision(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	audit := NewAuditLogger(logger)

	entry := AuditEntry{
		Decision: "allow",
		Policy:   "open-policy",
		Rule:     "",
		Agent:    "agent-1",
		Tool:     "read_data",
		User:     "bob@example.com",
		Session:  "sess-456",
		Mode:     "enforce",
	}

	audit.Log(entry)

	var logOutput map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logOutput); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	assertLogField(t, logOutput, "decision", "allow")
	assertLogField(t, logOutput, "msg", "policy.audit")
}

func assertLogField(t *testing.T, output map[string]interface{}, key, expected string) {
	t.Helper()
	val, ok := output[key]
	if !ok {
		t.Errorf("log output missing key %q", key)
		return
	}
	str, ok := val.(string)
	if !ok {
		t.Errorf("log field %q is not a string: %v", key, val)
		return
	}
	if str != expected {
		t.Errorf("log field %q = %q, want %q", key, str, expected)
	}
}

func TestRedactFields_RedactsSpecifiedFields(t *testing.T) {
	body := map[string]interface{}{
		"name":        "John Doe",
		"ssn":         "123-45-6789",
		"credit_card": "4111-1111-1111-1111",
		"amount":      100,
	}

	redacted := RedactFields(body, []string{"ssn", "credit_card"})

	if redacted["ssn"] != "[REDACTED]" {
		t.Errorf("ssn = %v, want [REDACTED]", redacted["ssn"])
	}
	if redacted["credit_card"] != "[REDACTED]" {
		t.Errorf("credit_card = %v, want [REDACTED]", redacted["credit_card"])
	}
	if redacted["name"] != "John Doe" {
		t.Errorf("name = %v, want %q", redacted["name"], "John Doe")
	}
	if redacted["amount"] != 100 {
		t.Errorf("amount = %v, want 100", redacted["amount"])
	}
}

func TestRedactFields_PreservesOriginal(t *testing.T) {
	body := map[string]interface{}{
		"password": "secret123",
		"user":     "alice",
	}

	_ = RedactFields(body, []string{"password"})

	if body["password"] != "secret123" {
		t.Errorf("original body was modified: password = %v", body["password"])
	}
}

func TestRedactFields_EmptyFieldsList(t *testing.T) {
	body := map[string]interface{}{"key": "value"}

	result := RedactFields(body, nil)
	if result["key"] != "value" {
		t.Errorf("key = %v, want %q", result["key"], "value")
	}
}

func TestRedactFields_EmptyBody(t *testing.T) {
	result := RedactFields(nil, []string{"ssn"})
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

func TestRedactFields_NoMatchingFields(t *testing.T) {
	body := map[string]interface{}{
		"name": "Alice",
		"age":  30,
	}

	result := RedactFields(body, []string{"ssn", "password"})
	if result["name"] != "Alice" {
		t.Errorf("name = %v, want %q", result["name"], "Alice")
	}
	if result["age"] != 30 {
		t.Errorf("age = %v, want 30", result["age"])
	}
}

func TestIsAuditEnabled_NilDefaultsTrue(t *testing.T) {
	if !IsAuditEnabled(nil) {
		t.Error("IsAuditEnabled(nil) = false, want true")
	}
}

func TestIsAuditEnabled_TruePointer(t *testing.T) {
	b := true
	if !IsAuditEnabled(&b) {
		t.Error("IsAuditEnabled(true) = false, want true")
	}
}

func TestIsAuditEnabled_FalsePointer(t *testing.T) {
	b := false
	if IsAuditEnabled(&b) {
		t.Error("IsAuditEnabled(false) = true, want false")
	}
}

func TestBuildRedactSet(t *testing.T) {
	set := buildRedactSet([]string{"a", "b", "c"})
	if len(set) != 3 {
		t.Errorf("set length = %d, want 3", len(set))
	}
	for _, key := range []string{"a", "b", "c"} {
		if _, ok := set[key]; !ok {
			t.Errorf("set missing key %q", key)
		}
	}
}

func TestNewAuditLogger(t *testing.T) {
	logger := slog.Default()
	audit := NewAuditLogger(logger)
	if audit == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
	if audit.logger != logger {
		t.Error("NewAuditLogger did not set logger correctly")
	}
}
