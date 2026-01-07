import { describe, it, expect } from "vitest";
import {
  MODEL_PRICING,
  getModelPricing,
  calculateCost,
  formatCost,
  formatTokens,
} from "./pricing";

describe("pricing utilities", () => {
  describe("getModelPricing", () => {
    it("should return pricing for exact model match", () => {
      const pricing = getModelPricing("gpt-4o");
      expect(pricing).not.toBeNull();
      expect(pricing?.provider).toBe("openai");
      expect(pricing?.inputPer1M).toBe(2.5);
      expect(pricing?.outputPer1M).toBe(10.0);
    });

    it("should return pricing for partial model match", () => {
      const pricing = getModelPricing("claude-sonnet-4-20250514-v2");
      expect(pricing).not.toBeNull();
      expect(pricing?.provider).toBe("anthropic");
      expect(pricing?.displayName).toBe("Claude Sonnet 4");
    });

    it("should return null for unknown model", () => {
      const pricing = getModelPricing("unknown-model-xyz");
      expect(pricing).toBeNull();
    });

    it("should find all Anthropic models", () => {
      const claudeOpus = getModelPricing("claude-opus-4-20250514");
      const claudeSonnet = getModelPricing("claude-sonnet-4-20250514");
      const claudeHaiku = getModelPricing("claude-haiku-3-20250514");

      expect(claudeOpus?.provider).toBe("anthropic");
      expect(claudeSonnet?.provider).toBe("anthropic");
      expect(claudeHaiku?.provider).toBe("anthropic");
    });

    it("should find OpenAI models", () => {
      const gpt4Turbo = getModelPricing("gpt-4-turbo");
      const gpt4oMini = getModelPricing("gpt-4o-mini");
      const gpt35 = getModelPricing("gpt-3.5-turbo");

      expect(gpt4Turbo?.provider).toBe("openai");
      expect(gpt4oMini?.provider).toBe("openai");
      expect(gpt35?.provider).toBe("openai");
    });

    it("should find Bedrock models", () => {
      const bedrockSonnet = getModelPricing("anthropic.claude-3-sonnet");
      const bedrockHaiku = getModelPricing("anthropic.claude-3-haiku");

      expect(bedrockSonnet?.provider).toBe("bedrock");
      expect(bedrockHaiku?.provider).toBe("bedrock");
    });

    it("should find self-hosted models with zero cost", () => {
      const llama = getModelPricing("llama3");
      const mistral = getModelPricing("mistral");

      expect(llama?.provider).toBe("ollama");
      expect(llama?.inputPer1M).toBe(0);
      expect(llama?.outputPer1M).toBe(0);
      expect(mistral?.provider).toBe("ollama");
    });
  });

  describe("calculateCost", () => {
    it("should calculate cost for known model", () => {
      // gpt-4o: $2.50/1M input, $10.00/1M output
      const cost = calculateCost("gpt-4o", 1_000_000, 500_000);
      // Expected: (1M / 1M * 2.50) + (0.5M / 1M * 10.00) = 2.50 + 5.00 = 7.50
      expect(cost).toBe(7.5);
    });

    it("should return 0 for unknown model", () => {
      const cost = calculateCost("unknown-model", 1_000_000, 1_000_000);
      expect(cost).toBe(0);
    });

    it("should calculate cost with cache savings", () => {
      // claude-sonnet-4: $3.00/1M input, $15.00/1M output, $0.30/1M cache
      // Cache savings = (cacheHits / 1M) * (inputPer1M - cachePer1M)
      const cost = calculateCost(
        "claude-sonnet-4-20250514",
        1_000_000, // 1M input
        500_000, // 0.5M output
        200_000 // 0.2M cache hits
      );
      // Input: 1M / 1M * 3.00 = 3.00
      // Output: 0.5M / 1M * 15.00 = 7.50
      // Cache savings: 0.2M / 1M * (3.00 - 0.30) = 0.2 * 2.70 = 0.54
      // Total: 3.00 + 7.50 - 0.54 = 9.96
      expect(cost).toBeCloseTo(9.96, 2);
    });

    it("should handle models without cache pricing", () => {
      // gpt-4o doesn't have cache pricing
      const cost = calculateCost(
        "gpt-4o",
        1_000_000,
        500_000,
        200_000 // cache hits ignored since no cache pricing
      );
      expect(cost).toBe(7.5);
    });

    it("should handle zero tokens", () => {
      const cost = calculateCost("gpt-4o", 0, 0);
      expect(cost).toBe(0);
    });

    it("should handle self-hosted models (zero cost)", () => {
      const cost = calculateCost("llama3", 10_000_000, 5_000_000);
      expect(cost).toBe(0);
    });
  });

  describe("formatCost", () => {
    it("should format very small costs with 4 decimal places", () => {
      expect(formatCost(0.0001)).toBe("$0.0001");
      expect(formatCost(0.0099)).toBe("$0.0099");
    });

    it("should format small costs with 3 decimal places", () => {
      expect(formatCost(0.01)).toBe("$0.010");
      expect(formatCost(0.123)).toBe("$0.123");
      expect(formatCost(0.999)).toBe("$0.999");
    });

    it("should format regular costs with 2 decimal places", () => {
      expect(formatCost(1.0)).toBe("$1.00");
      expect(formatCost(10.5)).toBe("$10.50");
      expect(formatCost(100.99)).toBe("$100.99");
    });

    it("should handle zero", () => {
      expect(formatCost(0)).toBe("$0.0000");
    });
  });

  describe("formatTokens", () => {
    it("should format tokens in millions", () => {
      expect(formatTokens(1_000_000)).toBe("1.0M");
      expect(formatTokens(2_500_000)).toBe("2.5M");
      expect(formatTokens(10_000_000)).toBe("10.0M");
    });

    it("should format tokens in thousands", () => {
      expect(formatTokens(1_000)).toBe("1.0K");
      expect(formatTokens(50_000)).toBe("50.0K");
      expect(formatTokens(999_999)).toBe("1000.0K"); // Edge case
    });

    it("should format small token counts as-is", () => {
      expect(formatTokens(0)).toBe("0");
      expect(formatTokens(100)).toBe("100");
      expect(formatTokens(999)).toBe("999");
    });
  });

  describe("MODEL_PRICING constant", () => {
    it("should have all required fields for each model", () => {
      for (const pricing of MODEL_PRICING) {
        expect(pricing.provider).toBeDefined();
        expect(pricing.model).toBeDefined();
        expect(pricing.displayName).toBeDefined();
        expect(typeof pricing.inputPer1M).toBe("number");
        expect(typeof pricing.outputPer1M).toBe("number");
        expect(pricing.inputPer1M).toBeGreaterThanOrEqual(0);
        expect(pricing.outputPer1M).toBeGreaterThanOrEqual(0);
      }
    });

    it("should have unique models", () => {
      const models = MODEL_PRICING.map((p) => p.model);
      const uniqueModels = new Set(models);
      expect(uniqueModels.size).toBe(models.length);
    });
  });
});
