/**
 * LLM model pricing table for cost estimation.
 * Prices are in USD per 1 million tokens.
 *
 * Note: These are approximate prices and may vary.
 * For accurate billing, use actual provider invoices.
 */

export interface ModelPricing {
  provider: string;
  model: string;
  displayName: string;
  inputPer1M: number;  // USD per 1M input tokens
  outputPer1M: number; // USD per 1M output tokens
  cachePer1M?: number; // USD per 1M cached tokens (if applicable)
}

export const MODEL_PRICING: ModelPricing[] = [
  // Anthropic
  {
    provider: "anthropic",
    model: "claude-opus-4-20250514",
    displayName: "Claude Opus 4",
    inputPer1M: 15,
    outputPer1M: 75,
    cachePer1M: 1.5,
  },
  {
    provider: "anthropic",
    model: "claude-sonnet-4-20250514",
    displayName: "Claude Sonnet 4",
    inputPer1M: 3,
    outputPer1M: 15,
    cachePer1M: 0.3,
  },
  {
    provider: "anthropic",
    model: "claude-haiku-3-20250514",
    displayName: "Claude Haiku 3",
    inputPer1M: 0.25,
    outputPer1M: 1.25,
    cachePer1M: 0.025,
  },
  // OpenAI
  {
    provider: "openai",
    model: "gpt-4-turbo",
    displayName: "GPT-4 Turbo",
    inputPer1M: 10,
    outputPer1M: 30,
  },
  {
    provider: "openai",
    model: "gpt-4o",
    displayName: "GPT-4o",
    inputPer1M: 2.5,
    outputPer1M: 10,
  },
  {
    provider: "openai",
    model: "gpt-4o-mini",
    displayName: "GPT-4o Mini",
    inputPer1M: 0.15,
    outputPer1M: 0.6,
  },
  {
    provider: "openai",
    model: "gpt-3.5-turbo",
    displayName: "GPT-3.5 Turbo",
    inputPer1M: 0.5,
    outputPer1M: 1.5,
  },
  // AWS Bedrock (approximate)
  {
    provider: "bedrock",
    model: "anthropic.claude-3-sonnet",
    displayName: "Claude 3 Sonnet (Bedrock)",
    inputPer1M: 3,
    outputPer1M: 15,
  },
  {
    provider: "bedrock",
    model: "anthropic.claude-3-haiku",
    displayName: "Claude 3 Haiku (Bedrock)",
    inputPer1M: 0.25,
    outputPer1M: 1.25,
  },
  // Ollama / Self-hosted (no cost)
  {
    provider: "ollama",
    model: "llama3",
    displayName: "Llama 3 (Self-hosted)",
    inputPer1M: 0,
    outputPer1M: 0,
  },
  {
    provider: "ollama",
    model: "mistral",
    displayName: "Mistral (Self-hosted)",
    inputPer1M: 0,
    outputPer1M: 0,
  },
];

/**
 * Get pricing for a specific model.
 * Returns null if model not found.
 */
export function getModelPricing(model: string): ModelPricing | null {
  // Try exact match first
  const exactMatch = MODEL_PRICING.find((p) => p.model === model);
  if (exactMatch) return exactMatch;

  // Try partial match (for versioned model names)
  const partialMatch = MODEL_PRICING.find((p) =>
    model.includes(p.model) || p.model.includes(model)
  );
  return partialMatch || null;
}

/**
 * Calculate estimated cost for token usage.
 */
export function calculateCost(
  model: string,
  inputTokens: number,
  outputTokens: number,
  cacheHits: number = 0
): number {
  const pricing = getModelPricing(model);
  if (!pricing) return 0;

  const inputCost = (inputTokens / 1_000_000) * pricing.inputPer1M;
  const outputCost = (outputTokens / 1_000_000) * pricing.outputPer1M;
  const cacheSavings = pricing.cachePer1M
    ? (cacheHits / 1_000_000) * (pricing.inputPer1M - pricing.cachePer1M)
    : 0;

  return inputCost + outputCost - cacheSavings;
}

/**
 * Format cost as currency string.
 */
export function formatCost(cost: number): string {
  if (cost < 0.01) {
    return `$${cost.toFixed(4)}`;
  }
  if (cost < 1) {
    return `$${cost.toFixed(3)}`;
  }
  return `$${cost.toFixed(2)}`;
}

/**
 * Format token count with K/M suffix.
 */
export function formatTokens(tokens: number): string {
  if (tokens >= 1_000_000) {
    return `${(tokens / 1_000_000).toFixed(1)}M`;
  }
  if (tokens >= 1_000) {
    return `${(tokens / 1_000).toFixed(1)}K`;
  }
  return tokens.toString();
}
