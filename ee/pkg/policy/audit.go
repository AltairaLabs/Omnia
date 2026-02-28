/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"log/slog"
)

// Audit header constants for extracting context from request headers.
const (
	HeaderAgentName = "X-Omnia-Agent-Name"
	HeaderUser      = "X-Omnia-User"
	HeaderSessionID = "X-Omnia-Session-Id"
)

// Redaction constant.
const redactedValue = "[REDACTED]"

// AuditEntry represents a structured audit log entry for a policy decision.
type AuditEntry struct {
	Decision string `json:"decision"`
	Policy   string `json:"policy"`
	Rule     string `json:"rule"`
	Agent    string `json:"agent"`
	Tool     string `json:"tool"`
	User     string `json:"user"`
	Session  string `json:"session"`
	Message  string `json:"message,omitempty"`
	Mode     string `json:"mode"`
}

// AuditLogger emits structured log entries for policy decisions.
type AuditLogger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new AuditLogger backed by the given slog.Logger.
func NewAuditLogger(logger *slog.Logger) *AuditLogger {
	return &AuditLogger{logger: logger}
}

// Log emits a structured audit log entry.
func (a *AuditLogger) Log(entry AuditEntry) {
	a.logger.Info("policy.audit",
		"decision", entry.Decision,
		"policy", entry.Policy,
		"rule", entry.Rule,
		"agent", entry.Agent,
		"tool", entry.Tool,
		"user", entry.User,
		"session", entry.Session,
		"message", entry.Message,
		"mode", entry.Mode,
	)
}

// RedactFields replaces values of specified field names in the body map with
// the redacted placeholder. It returns a shallow copy of the body with
// redacted values, leaving the original unchanged.
func RedactFields(body map[string]interface{}, fields []string) map[string]interface{} {
	if len(fields) == 0 || len(body) == 0 {
		return body
	}
	redactSet := buildRedactSet(fields)
	redacted := make(map[string]interface{}, len(body))
	for k, v := range body {
		if _, ok := redactSet[k]; ok {
			redacted[k] = redactedValue
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// buildRedactSet converts a slice of field names to a set for O(1) lookups.
func buildRedactSet(fields []string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}

// IsAuditEnabled checks whether auditing is enabled for a given audit config.
// If the config is nil or Enabled is nil, auditing defaults to true.
func IsAuditEnabled(enabled *bool) bool {
	if enabled == nil {
		return true
	}
	return *enabled
}
