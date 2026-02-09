/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package redaction

import (
	"fmt"
	"regexp"
	"strings"
)

// patternDef defines a built-in PII pattern with its compiled regex and replacement token.
type patternDef struct {
	Name  string
	Regex *regexp.Regexp
	Token string
}

// builtinPatterns is the registry of compiled built-in PII patterns, keyed by name.
var builtinPatterns map[string]patternDef

func init() {
	defs := []struct {
		name  string
		regex string
		token string
	}{
		{"ssn", `\b\d{3}-\d{2}-\d{4}\b`, "[REDACTED_SSN]"},
		{"credit_card", `\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`, "[REDACTED_CC]"},
		{"phone_number", `\b\d{3}[-.)\\s]?\d{3}[-.)\\s]?\d{4}\b`, "[REDACTED_PHONE]"},
		{"email", `(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`, "[REDACTED_EMAIL]"},
		{"ip_address", `\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`, "[REDACTED_IP]"},
	}

	builtinPatterns = make(map[string]patternDef, len(defs))
	for _, d := range defs {
		builtinPatterns[d.name] = patternDef{
			Name:  d.name,
			Regex: regexp.MustCompile(d.regex),
			Token: d.token,
		}
	}
}

// resolvePatterns resolves pattern names to compiled patternDefs.
// Built-in names (e.g. "ssn") are looked up from the registry.
// Names with the "custom:" prefix are compiled as user-provided regex.
// Unknown built-in names return an error.
func resolvePatterns(names []string) ([]patternDef, error) {
	resolved := make([]patternDef, 0, len(names))
	for _, name := range names {
		if expr, ok := strings.CutPrefix(name, "custom:"); ok {
			re, err := regexp.Compile(expr)
			if err != nil {
				return nil, fmt.Errorf("invalid custom pattern %q: %w", expr, err)
			}
			resolved = append(resolved, patternDef{
				Name:  name,
				Regex: re,
				Token: "[REDACTED_CUSTOM]",
			})
			continue
		}
		p, ok := builtinPatterns[name]
		if !ok {
			return nil, fmt.Errorf("unknown PII pattern: %q", name)
		}
		resolved = append(resolved, p)
	}
	return resolved, nil
}
