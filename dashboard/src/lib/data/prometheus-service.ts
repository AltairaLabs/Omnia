/**
 * Prometheus-based cost data service.
 *
 * Queries Prometheus for LLM cost and usage metrics emitted by the runtime.
 */

import {
  queryPrometheus,
  queryPrometheusRange,
  isPrometheusAvailable,
  matrixToTimeSeries,
} from "../prometheus";
import { getModelPricing } from "../pricing";
import type {
  CostData,
  CostOptions,
  CostAllocationItem,
  CostSummary,
  ProviderCost,
  ModelCost,
  CostTimeSeriesPoint,
} from "./types";

// Default empty cost summary
const EMPTY_SUMMARY: CostSummary = {
  totalCost: 0,
  totalInputCost: 0,
  totalOutputCost: 0,
  totalCacheSavings: 0,
  totalRequests: 0,
  totalTokens: 0,
  anthropicCost: 0,
  openaiCost: 0,
  projectedMonthlyCost: 0,
  anthropicPercent: 0,
  openaiPercent: 0,
  inputPercent: 0,
  outputPercent: 0,
};

// Grafana dashboard URL (configured via environment)
const GRAFANA_URL = process.env.NEXT_PUBLIC_GRAFANA_URL;

export class PrometheusService {
  private available: boolean | null = null;

  /**
   * Check if Prometheus is available.
   */
  async checkAvailability(): Promise<boolean> {
    this.available ??= await isPrometheusAvailable();
    return this.available;
  }

  /**
   * Fetch cost data from Prometheus.
   */
  async getCosts(options?: CostOptions): Promise<CostData> {
    const available = await this.checkAvailability();

    if (!available) {
      return {
        available: false,
        reason: "Prometheus not configured",
        summary: EMPTY_SUMMARY,
        byAgent: [],
        byProvider: [],
        byModel: [],
        timeSeries: [],
      };
    }

    try {
      const namespace = options?.namespace;

      // Build namespace filter for PromQL queries
      const nsFilter = namespace ? `,namespace="${namespace}"` : "";

      // Calculate time range (last 24 hours)
      const now = new Date();
      const start = new Date(now.getTime() - 24 * 60 * 60 * 1000);

      // Query current totals (instant queries)
      const [inputTokensResult, outputTokensResult, cacheHitsResult, requestsResult, costResult] =
        await Promise.all([
          queryPrometheus(`sum by (agent, namespace, provider, model) (omnia_llm_input_tokens_total{${nsFilter.slice(1)}})`),
          queryPrometheus(`sum by (agent, namespace, provider, model) (omnia_llm_output_tokens_total{${nsFilter.slice(1)}})`),
          queryPrometheus(`sum by (agent, namespace, provider, model) (omnia_llm_cache_hits_total{${nsFilter.slice(1)}})`),
          queryPrometheus(`sum by (agent, namespace, provider, model) (omnia_llm_requests_total{${nsFilter.slice(1)}})`),
          queryPrometheus(`sum by (agent, namespace, provider, model) (omnia_llm_cost_usd_total{${nsFilter.slice(1)}})`),
        ]);

      // Query time series for charts (last 24h, hourly resolution)
      const costTimeSeriesResult = await queryPrometheusRange(
        `sum by (provider) (increase(omnia_llm_cost_usd_total{${nsFilter.slice(1)}}[1h]))`,
        start,
        now,
        "1h"
      );

      // Build per-agent cost allocation
      const byAgent = this.buildAgentCosts(
        inputTokensResult,
        outputTokensResult,
        cacheHitsResult,
        requestsResult,
        costResult
      );

      // Build summary
      const summary = this.buildSummary(byAgent);

      // Build provider breakdown
      const byProvider = this.buildProviderCosts(byAgent);

      // Build model breakdown
      const byModel = this.buildModelCosts(byAgent);

      // Build time series
      const timeSeries = this.buildTimeSeries(costTimeSeriesResult);

      return {
        available: true,
        summary,
        byAgent,
        byProvider,
        byModel,
        timeSeries,
        grafanaUrl: GRAFANA_URL ? `${GRAFANA_URL}/d/omnia-costs/llm-costs` : undefined,
      };
    } catch (error) {
      console.error("Failed to fetch cost data from Prometheus:", error);
      return {
        available: false,
        reason: error instanceof Error ? error.message : "Unknown error",
        summary: EMPTY_SUMMARY,
        byAgent: [],
        byProvider: [],
        byModel: [],
        timeSeries: [],
      };
    }
  }

  /**
   * Process a single Prometheus result and update agent entries.
   */
  private processAgentResult(
    result: Awaited<ReturnType<typeof queryPrometheus>>,
    agentMap: Map<string, CostAllocationItem>,
    field: keyof Pick<CostAllocationItem, "inputTokens" | "outputTokens" | "cacheHits" | "requests" | "totalCost">
  ): void {
    if (result.status !== "success" || !result.data?.result) return;
    for (const item of result.data.result) {
      const agent = this.getOrCreateAgent(agentMap, item.metric);
      agent[field] = Number.parseFloat(item.value[1]) || 0;
    }
  }

  /**
   * Get or create an agent entry in the map.
   */
  private getOrCreateAgent(
    agentMap: Map<string, CostAllocationItem>,
    metric: Record<string, string>
  ): CostAllocationItem {
    const key = `${metric.namespace}/${metric.agent}/${metric.model}`;
    if (!agentMap.has(key)) {
      agentMap.set(key, {
        agent: metric.agent || "unknown",
        namespace: metric.namespace || "default",
        provider: metric.provider || "unknown",
        model: metric.model || "unknown",
        inputTokens: 0,
        outputTokens: 0,
        cacheHits: 0,
        requests: 0,
        inputCost: 0,
        outputCost: 0,
        cacheSavings: 0,
        totalCost: 0,
      });
    }
    return agentMap.get(key)!;
  }

  private buildAgentCosts(
    inputTokensResult: Awaited<ReturnType<typeof queryPrometheus>>,
    outputTokensResult: Awaited<ReturnType<typeof queryPrometheus>>,
    cacheHitsResult: Awaited<ReturnType<typeof queryPrometheus>>,
    requestsResult: Awaited<ReturnType<typeof queryPrometheus>>,
    costResult: Awaited<ReturnType<typeof queryPrometheus>>
  ): CostAllocationItem[] {
    const agentMap = new Map<string, CostAllocationItem>();

    this.processAgentResult(inputTokensResult, agentMap, "inputTokens");
    this.processAgentResult(outputTokensResult, agentMap, "outputTokens");
    this.processAgentResult(cacheHitsResult, agentMap, "cacheHits");
    this.processAgentResult(requestsResult, agentMap, "requests");
    this.processAgentResult(costResult, agentMap, "totalCost");

    // Calculate input/output costs using pricing
    for (const agent of agentMap.values()) {
      const pricing = getModelPricing(agent.model);
      if (pricing) {
        agent.inputCost = (agent.inputTokens / 1_000_000) * pricing.inputPer1M;
        agent.outputCost = (agent.outputTokens / 1_000_000) * pricing.outputPer1M;
        if (pricing.cachePer1M) {
          agent.cacheSavings =
            (agent.cacheHits / 1_000_000) * (pricing.inputPer1M - pricing.cachePer1M);
        }
      }
    }

    return Array.from(agentMap.values()).sort((a, b) => b.totalCost - a.totalCost);
  }

  private buildSummary(byAgent: CostAllocationItem[]): CostSummary {
    const totalCost = byAgent.reduce((sum, item) => sum + item.totalCost, 0);
    const totalInputCost = byAgent.reduce((sum, item) => sum + item.inputCost, 0);
    const totalOutputCost = byAgent.reduce((sum, item) => sum + item.outputCost, 0);
    const totalCacheSavings = byAgent.reduce((sum, item) => sum + item.cacheSavings, 0);
    const totalRequests = byAgent.reduce((sum, item) => sum + item.requests, 0);
    const totalTokens = byAgent.reduce(
      (sum, item) => sum + item.inputTokens + item.outputTokens,
      0
    );

    const anthropicCost = byAgent
      .filter((item) => item.provider === "anthropic" || item.provider === "claude")
      .reduce((sum, item) => sum + item.totalCost, 0);
    const openaiCost = byAgent
      .filter((item) => item.provider === "openai")
      .reduce((sum, item) => sum + item.totalCost, 0);

    return {
      totalCost,
      totalInputCost,
      totalOutputCost,
      totalCacheSavings,
      totalRequests,
      totalTokens,
      anthropicCost,
      openaiCost,
      projectedMonthlyCost: totalCost * 30,
      anthropicPercent: totalCost > 0 ? (anthropicCost / totalCost) * 100 : 0,
      openaiPercent: totalCost > 0 ? (openaiCost / totalCost) * 100 : 0,
      inputPercent:
        totalInputCost + totalOutputCost > 0
          ? (totalInputCost / (totalInputCost + totalOutputCost)) * 100
          : 0,
      outputPercent:
        totalInputCost + totalOutputCost > 0
          ? (totalOutputCost / (totalInputCost + totalOutputCost)) * 100
          : 0,
    };
  }

  private buildProviderCosts(byAgent: CostAllocationItem[]): ProviderCost[] {
    const providerMap = new Map<string, ProviderCost>();

    const getProviderDisplayName = (p: string): string => {
      if (p === "anthropic") return "Anthropic";
      if (p === "openai") return "OpenAI";
      return p;
    };

    for (const item of byAgent) {
      const provider = item.provider === "claude" ? "anthropic" : item.provider;
      if (!providerMap.has(provider)) {
        providerMap.set(provider, {
          name: getProviderDisplayName(provider),
          provider,
          cost: 0,
          requests: 0,
          tokens: 0,
        });
      }
      const p = providerMap.get(provider)!;
      p.cost += item.totalCost;
      p.requests += item.requests;
      p.tokens += item.inputTokens + item.outputTokens;
    }

    return Array.from(providerMap.values()).sort((a, b) => b.cost - a.cost);
  }

  private buildModelCosts(byAgent: CostAllocationItem[]): ModelCost[] {
    const modelMap = new Map<string, ModelCost>();

    for (const item of byAgent) {
      if (!modelMap.has(item.model)) {
        modelMap.set(item.model, {
          model: item.model,
          displayName: this.getModelDisplayName(item.model),
          provider: item.provider,
          cost: 0,
          requests: 0,
          tokens: 0,
        });
      }
      const m = modelMap.get(item.model)!;
      m.cost += item.totalCost;
      m.requests += item.requests;
      m.tokens += item.inputTokens + item.outputTokens;
    }

    return Array.from(modelMap.values()).sort((a, b) => b.cost - a.cost);
  }

  private buildTimeSeries(
    result: Awaited<ReturnType<typeof queryPrometheusRange>>
  ): CostTimeSeriesPoint[] {
    const series = matrixToTimeSeries(result);

    return series.map(({ timestamp, values }) => ({
      timestamp: timestamp.toISOString(),
      anthropic: values.anthropic || values.claude || 0,
      openai: values.openai || 0,
      total: Object.values(values).reduce((sum, v) => sum + v, 0),
    }));
  }

  private getModelDisplayName(model: string): string {
    const displayNames: Record<string, string> = {
      "claude-3-opus": "Claude 3 Opus",
      "claude-3-sonnet": "Claude 3 Sonnet",
      "claude-3-haiku": "Claude 3 Haiku",
      "claude-sonnet-4": "Claude Sonnet 4",
      "claude-opus-4": "Claude Opus 4",
      "gpt-4": "GPT-4",
      "gpt-4-turbo": "GPT-4 Turbo",
      "gpt-4o": "GPT-4o",
      "gpt-4o-mini": "GPT-4o Mini",
      "gpt-3.5-turbo": "GPT-3.5 Turbo",
    };
    return displayNames[model] || model;
  }
}
