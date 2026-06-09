/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import "strings"

// parseKeyValueLabels parses a comma-separated "k1=v1,k2=v2" string into a
// label map. Blank entries and entries without "=" are skipped; keys and
// values are trimmed. Returns nil for an empty input so callers can leave the
// pod-label set unset rather than empty.
func parseKeyValueLabels(s string) map[string]string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	labels := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		labels[key] = strings.TrimSpace(value)
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}
