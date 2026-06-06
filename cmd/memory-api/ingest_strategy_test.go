/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

func TestSelectIngestionStrategy(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantName  string
		wantChunk bool // true → *ChunkStrategy, false → *SummaryStrategy
	}{
		{"chunk explicit", ingestStrategyChunk, ingestStrategyChunk, true},
		{"empty defaults to chunk", "", ingestStrategyChunk, true},
		{"unknown defaults to chunk", "bogus", ingestStrategyChunk, true},
		{"summary selects SummaryStrategy", ingestStrategySummary, ingestStrategySummary, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, name := selectIngestionStrategy(tc.input, 100, 20)
			if name != tc.wantName {
				t.Errorf("resolved name = %q, want %q", name, tc.wantName)
			}
			switch got.(type) {
			case *ingestion.ChunkStrategy:
				if !tc.wantChunk {
					t.Errorf("got *ChunkStrategy, want *SummaryStrategy")
				}
			case *ingestion.SummaryStrategy:
				if tc.wantChunk {
					t.Errorf("got *SummaryStrategy, want *ChunkStrategy")
				}
			default:
				t.Errorf("unexpected strategy type %T", got)
			}
		})
	}
}
