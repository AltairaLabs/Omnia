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

func TestSelectIngestionConfig(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantName     string
		wantStrategy string
	}{
		{"chunk explicit", ingestStrategyChunk, ingestStrategyChunk, ingestion.StrategyChunk},
		{"empty defaults to chunk", "", ingestStrategyChunk, ingestion.StrategyChunk},
		{"unknown defaults to chunk", "bogus", ingestStrategyChunk, ingestion.StrategyChunk},
		{"summary selects summary strategy", ingestStrategySummary, ingestStrategySummary, ingestion.StrategySummary},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, name := selectIngestionConfig(tc.input, 100, 20)
			if name != tc.wantName {
				t.Errorf("resolved name = %q, want %q", name, tc.wantName)
			}
			if cfg.Strategy != tc.wantStrategy {
				t.Errorf("strategy = %q, want %q", cfg.Strategy, tc.wantStrategy)
			}
			if cfg.ChunkSize != 100 || cfg.ChunkOverlap != 20 {
				t.Errorf("chunk geometry = (%d,%d), want (100,20)", cfg.ChunkSize, cfg.ChunkOverlap)
			}
		})
	}
}
