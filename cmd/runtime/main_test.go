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

package main

import (
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
)

func TestWarnIfCustomTruncation(t *testing.T) {
	cases := []struct {
		name     string
		strategy string
		want     bool
	}{
		{"custom warns", "custom", true},
		{"sliding does not warn", "sliding", false},
		{"summarize does not warn", "summarize", false},
		{"empty does not warn", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := warnIfCustomTruncation(logr.Discard(), tc.strategy); got != tc.want {
				t.Fatalf("warnIfCustomTruncation(%q) = %v, want %v", tc.strategy, got, tc.want)
			}
		})
	}
}

// TestWarnIfCustomTruncation_EmitsWarning guards the actual feature: the
// operator-visible log line, not just the bool return. It would fail if the
// log.Info call were deleted or gutted, unlike TestWarnIfCustomTruncation
// above which only checks the return value.
func TestWarnIfCustomTruncation_EmitsWarning(t *testing.T) {
	t.Run("custom strategy emits the warning with structured fields", func(t *testing.T) {
		var captured []string
		log := funcr.NewJSON(func(obj string) {
			captured = append(captured, obj)
		}, funcr.Options{})

		if got := warnIfCustomTruncation(log, "custom"); !got {
			t.Fatalf("warnIfCustomTruncation(custom) = %v, want true", got)
		}
		if len(captured) != 1 {
			t.Fatalf("expected exactly one log record, got %d: %v", len(captured), captured)
		}

		record := captured[0]
		wantFields := []string{
			`"msg":"truncation disabled"`,
			`"reason":"customStrategyOnPromptKitRuntime"`,
			`"truncationStrategy":"custom"`,
			`"impact":"no truncation applied; context may exceed the provider limit"`,
			`"remedy":"use sliding or summarize, or run a custom runtime that implements truncation"`,
		}
		for _, want := range wantFields {
			if !strings.Contains(record, want) {
				t.Fatalf("log record missing %q; got: %s", want, record)
			}
		}
	})

	t.Run("non-custom strategy emits no log record", func(t *testing.T) {
		var captured []string
		log := funcr.NewJSON(func(obj string) {
			captured = append(captured, obj)
		}, funcr.Options{})

		if got := warnIfCustomTruncation(log, "sliding"); got {
			t.Fatalf("warnIfCustomTruncation(sliding) = %v, want false", got)
		}
		if len(captured) != 0 {
			t.Fatalf("expected no log records for non-custom strategy, got %d: %v", len(captured), captured)
		}
	})
}
