/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import "fmt"

// Token pricing constants (per 1M tokens).
const (
	// Claude claude-sonnet-4-20250514 pricing.
	claudeSonnetInputPer1M  = 3.0
	claudeSonnetOutputPer1M = 15.0

	// GPT-4o pricing.
	gpt4oInputPer1M  = 2.5
	gpt4oOutputPer1M = 10.0

	// GPT-4o-mini pricing.
	gpt4oMiniInputPer1M  = 0.15
	gpt4oMiniOutputPer1M = 0.60

	// pricingKeyFormat is the format string for pricing map keys.
	pricingKeyFormat = "%s:%s"
)

// ModelPricing holds per-1K-token pricing for a model.
type ModelPricing struct {
	InputPer1KTokens  float64
	OutputPer1KTokens float64
}

// CostCalculator computes LLM usage costs based on token counts and model pricing.
type CostCalculator struct {
	pricing map[string]ModelPricing // keyed by "provider:model"
}

// NewCostCalculator creates a CostCalculator pre-loaded with default pricing
// for common models.
func NewCostCalculator() *CostCalculator {
	c := &CostCalculator{
		pricing: make(map[string]ModelPricing),
	}
	c.registerDefaults()
	return c
}

// RegisterPricing adds or overrides pricing for a provider:model combination.
func (c *CostCalculator) RegisterPricing(provider, model string, pricing ModelPricing) {
	key := pricingKey(provider, model)
	c.pricing[key] = pricing
}

// Calculate returns the estimated cost in USD for the given token usage.
// Returns 0 if no pricing is registered for the provider:model combination.
func (c *CostCalculator) Calculate(provider, model string, inputTokens, outputTokens int) float64 {
	key := pricingKey(provider, model)
	p, ok := c.pricing[key]
	if !ok {
		return 0
	}
	inputCost := float64(inputTokens) * p.InputPer1KTokens / 1000.0
	outputCost := float64(outputTokens) * p.OutputPer1KTokens / 1000.0
	return inputCost + outputCost
}

// registerDefaults loads default pricing for well-known models.
func (c *CostCalculator) registerDefaults() {
	c.RegisterPricing("claude", "claude-sonnet-4-20250514", ModelPricing{
		InputPer1KTokens:  claudeSonnetInputPer1M / 1000.0,
		OutputPer1KTokens: claudeSonnetOutputPer1M / 1000.0,
	})
	c.RegisterPricing("openai", "gpt-4o", ModelPricing{
		InputPer1KTokens:  gpt4oInputPer1M / 1000.0,
		OutputPer1KTokens: gpt4oOutputPer1M / 1000.0,
	})
	c.RegisterPricing("openai", "gpt-4o-mini", ModelPricing{
		InputPer1KTokens:  gpt4oMiniInputPer1M / 1000.0,
		OutputPer1KTokens: gpt4oMiniOutputPer1M / 1000.0,
	})
}

// pricingKey returns the map key for a provider:model combination.
func pricingKey(provider, model string) string {
	return fmt.Sprintf(pricingKeyFormat, provider, model)
}
