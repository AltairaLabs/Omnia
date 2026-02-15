/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"math"
	"testing"
)

const floatTolerance = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

func TestNewCostCalculator_DefaultPricing(t *testing.T) {
	c := NewCostCalculator()

	tests := []struct {
		name         string
		provider     string
		model        string
		inputTokens  int
		outputTokens int
		wantCost     float64
	}{
		{
			name:         "claude-sonnet-4-20250514 1K input 1K output",
			provider:     "claude",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  1000,
			outputTokens: 1000,
			// input: 1000 * (3.0/1000) / 1000 = 0.003
			// output: 1000 * (15.0/1000) / 1000 = 0.015
			wantCost: 0.018,
		},
		{
			name:         "gpt-4o 1K input 1K output",
			provider:     "openai",
			model:        "gpt-4o",
			inputTokens:  1000,
			outputTokens: 1000,
			// input: 1000 * (2.5/1000) / 1000 = 0.0025
			// output: 1000 * (10.0/1000) / 1000 = 0.01
			wantCost: 0.0125,
		},
		{
			name:         "gpt-4o-mini 1K input 1K output",
			provider:     "openai",
			model:        "gpt-4o-mini",
			inputTokens:  1000,
			outputTokens: 1000,
			// input: 1000 * (0.15/1000) / 1000 = 0.00015
			// output: 1000 * (0.60/1000) / 1000 = 0.0006
			wantCost: 0.00075,
		},
		{
			name:         "claude-sonnet-4-20250514 1M input 0 output",
			provider:     "claude",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  1000000,
			outputTokens: 0,
			wantCost:     3.0,
		},
		{
			name:         "claude-sonnet-4-20250514 0 input 1M output",
			provider:     "claude",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  0,
			outputTokens: 1000000,
			wantCost:     15.0,
		},
		{
			name:         "zero tokens",
			provider:     "claude",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  0,
			outputTokens: 0,
			wantCost:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Calculate(tt.provider, tt.model, tt.inputTokens, tt.outputTokens)
			if !almostEqual(got, tt.wantCost) {
				t.Errorf("Calculate() = %v, want %v", got, tt.wantCost)
			}
		})
	}
}

func TestCostCalculator_UnknownModel(t *testing.T) {
	c := NewCostCalculator()

	cost := c.Calculate("unknown-provider", "unknown-model", 1000, 1000)
	if cost != 0 {
		t.Errorf("expected 0 for unknown model, got %v", cost)
	}
}

func TestCostCalculator_RegisterCustomPricing(t *testing.T) {
	c := NewCostCalculator()

	c.RegisterPricing("custom", "my-model", ModelPricing{
		InputPer1KTokens:  0.5,
		OutputPer1KTokens: 1.0,
	})

	cost := c.Calculate("custom", "my-model", 2000, 3000)
	// input: 2000 * 0.5 / 1000 = 1.0
	// output: 3000 * 1.0 / 1000 = 3.0
	expected := 4.0
	if !almostEqual(cost, expected) {
		t.Errorf("Calculate() = %v, want %v", cost, expected)
	}
}

func TestCostCalculator_OverrideDefaultPricing(t *testing.T) {
	c := NewCostCalculator()

	// Override gpt-4o pricing.
	c.RegisterPricing("openai", "gpt-4o", ModelPricing{
		InputPer1KTokens:  0.001,
		OutputPer1KTokens: 0.002,
	})

	cost := c.Calculate("openai", "gpt-4o", 1000, 1000)
	// input: 1000 * 0.001 / 1000 = 0.001
	// output: 1000 * 0.002 / 1000 = 0.002
	expected := 0.003
	if !almostEqual(cost, expected) {
		t.Errorf("Calculate() = %v, want %v", cost, expected)
	}
}

func TestCostCalculator_PricingKey(t *testing.T) {
	key := pricingKey("openai", "gpt-4o")
	if key != "openai:gpt-4o" {
		t.Errorf("pricingKey() = %q, want %q", key, "openai:gpt-4o")
	}
}

func TestCostCalculator_EmptyProviderModel(t *testing.T) {
	c := NewCostCalculator()

	// Empty provider/model should return 0 (not registered).
	cost := c.Calculate("", "", 1000, 1000)
	if cost != 0 {
		t.Errorf("expected 0 for empty provider/model, got %v", cost)
	}
}

func TestCostCalculator_LargeTokenCounts(t *testing.T) {
	c := NewCostCalculator()

	// 10M input tokens, 5M output tokens for gpt-4o.
	cost := c.Calculate("openai", "gpt-4o", 10_000_000, 5_000_000)
	// input: 10M * (2.5/1000) / 1000 = 25.0
	// output: 5M * (10.0/1000) / 1000 = 50.0
	expected := 75.0
	if !almostEqual(cost, expected) {
		t.Errorf("Calculate() = %v, want %v", cost, expected)
	}
}
