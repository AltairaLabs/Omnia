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
	"testing"

	"github.com/go-logr/logr"
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
