import { describe, it, expect } from "vitest";
import {
  AGENT_METRICS,
  FACADE_METRICS,
  LLM_METRICS,
  LABELS,
  buildFilter,
  buildEvalSelector,
  AgentQueries,
  LLMQueries,
  SystemQueries,
  EvalQueries,
  EVAL_METRIC_PATTERN,
} from "./prometheus-queries";

describe("prometheus-queries", () => {
  describe("metric constants", () => {
    it("exports AGENT_METRICS with correct prefixes", () => {
      expect(AGENT_METRICS.CONNECTIONS_ACTIVE).toBe(
        "omnia_agent_connections_active"
      );
      expect(AGENT_METRICS.REQUESTS_TOTAL).toBe("omnia_agent_requests_total");
      expect(AGENT_METRICS.REQUEST_DURATION).toBe(
        "omnia_agent_request_duration_seconds"
      );
    });

    it("exports FACADE_METRICS with correct prefixes", () => {
      expect(FACADE_METRICS.DOWNLOAD_BYTES).toBe(
        "omnia_facade_download_bytes_total"
      );
      expect(FACADE_METRICS.UPLOAD_BYTES).toBe(
        "omnia_facade_upload_bytes_total"
      );
    });

    it("exports LLM_METRICS with correct prefixes", () => {
      expect(LLM_METRICS.COST_USD).toBe("omnia_llm_cost_usd_total");
      expect(LLM_METRICS.INPUT_TOKENS).toBe("omnia_llm_input_tokens_total");
      expect(LLM_METRICS.OUTPUT_TOKENS).toBe("omnia_llm_output_tokens_total");
    });

    it("exports LABELS with correct names", () => {
      expect(LABELS.AGENT).toBe("agent");
      expect(LABELS.NAMESPACE).toBe("namespace");
      expect(LABELS.PROVIDER).toBe("provider");
      expect(LABELS.MODEL).toBe("model");
    });
  });

  describe("buildFilter", () => {
    it("returns empty string for empty filter", () => {
      expect(buildFilter({})).toBe("");
    });

    it("builds filter with single label", () => {
      expect(buildFilter({ agent: "my-agent" })).toBe('agent="my-agent"');
    });

    it("builds filter with multiple labels", () => {
      const filter = buildFilter({
        agent: "my-agent",
        namespace: "prod",
      });
      expect(filter).toBe('agent="my-agent",namespace="prod"');
    });

    it("builds filter with all labels", () => {
      const filter = buildFilter({
        agent: "my-agent",
        namespace: "prod",
        provider: "openai",
        model: "gpt-4",
        status: "success",
      });
      expect(filter).toContain('agent="my-agent"');
      expect(filter).toContain('namespace="prod"');
      expect(filter).toContain('provider="openai"');
      expect(filter).toContain('model="gpt-4"');
      expect(filter).toContain('status="success"');
    });

    it("ignores undefined values", () => {
      const filter = buildFilter({
        agent: "my-agent",
        namespace: undefined,
      });
      expect(filter).toBe('agent="my-agent"');
    });
  });

  describe("AgentQueries", () => {
    it("connectionsActive returns correct query", () => {
      expect(AgentQueries.connectionsActive()).toBe(
        "sum(omnia_agent_connections_active)"
      );
    });

    it("connectionsActive with filter returns correct query", () => {
      expect(AgentQueries.connectionsActive({ agent: "test" })).toBe(
        'sum(omnia_agent_connections_active{agent="test"})'
      );
    });

    it("requestRate returns correct query with default window", () => {
      const query = AgentQueries.requestRate();
      expect(query).toContain("rate(");
      expect(query).toContain("[5m]");
      expect(query).toContain(AGENT_METRICS.REQUESTS_TOTAL);
    });

    it("requestRate respects custom window", () => {
      const query = AgentQueries.requestRate(undefined, "1h");
      expect(query).toContain("[1h]");
    });

    it("p95Latency returns histogram_quantile query", () => {
      const query = AgentQueries.p95Latency();
      expect(query).toContain("histogram_quantile(0.95");
      expect(query).toContain("_bucket");
      expect(query).toContain("* 1000"); // Convert to ms
    });

    it("p99Latency returns histogram_quantile query", () => {
      const query = AgentQueries.p99Latency();
      expect(query).toContain("histogram_quantile(0.99");
    });

    it("avgLatency returns sum/count ratio", () => {
      const query = AgentQueries.avgLatency();
      expect(query).toContain("_sum");
      expect(query).toContain("_count");
      expect(query).toContain("* 1000"); // Convert to ms
    });

    it("inflightRequests returns correct query", () => {
      expect(AgentQueries.inflightRequests()).toContain(
        AGENT_METRICS.REQUESTS_INFLIGHT
      );
    });

    it("activeSessions returns correct query", () => {
      expect(AgentQueries.activeSessions()).toContain(
        AGENT_METRICS.SESSIONS_ACTIVE
      );
    });

    it("totalRequests returns correct query", () => {
      expect(AgentQueries.totalRequests()).toContain(
        AGENT_METRICS.REQUESTS_TOTAL
      );
    });
  });

  describe("LLMQueries", () => {
    it("requestRate returns correct query", () => {
      const query = LLMQueries.requestRate();
      expect(query).toContain("rate(");
      expect(query).toContain(LLM_METRICS.REQUESTS_TOTAL);
    });

    it("errorRate returns ratio of error to total", () => {
      const query = LLMQueries.errorRate();
      expect(query).toContain('status="error"');
      expect(query).toContain("/"); // Division for ratio
    });

    it("inputTokens returns correct query", () => {
      expect(LLMQueries.inputTokens()).toContain(LLM_METRICS.INPUT_TOKENS);
    });

    it("outputTokens returns correct query", () => {
      expect(LLMQueries.outputTokens()).toContain(LLM_METRICS.OUTPUT_TOKENS);
    });

    it("inputTokenRate returns rate query", () => {
      const query = LLMQueries.inputTokenRate();
      expect(query).toContain("rate(");
      expect(query).toContain(LLM_METRICS.INPUT_TOKENS);
    });

    it("outputTokenRate returns rate query", () => {
      const query = LLMQueries.outputTokenRate();
      expect(query).toContain("rate(");
      expect(query).toContain(LLM_METRICS.OUTPUT_TOKENS);
    });

    it("inputTokenIncrease returns increase query", () => {
      const query = LLMQueries.inputTokenIncrease();
      expect(query).toContain("increase(");
      expect(query).toContain(LLM_METRICS.INPUT_TOKENS);
    });

    it("outputTokenIncrease returns increase query", () => {
      const query = LLMQueries.outputTokenIncrease();
      expect(query).toContain("increase(");
      expect(query).toContain(LLM_METRICS.OUTPUT_TOKENS);
    });

    it("totalCost returns correct query", () => {
      expect(LLMQueries.totalCost()).toContain(LLM_METRICS.COST_USD);
    });

    it("costIncrease returns increase query with 24h default", () => {
      const query = LLMQueries.costIncrease();
      expect(query).toContain("increase(");
      expect(query).toContain("[24h]");
    });

    it("cacheHits returns correct query", () => {
      expect(LLMQueries.cacheHits()).toContain(LLM_METRICS.CACHE_HITS);
    });

    it("p95Latency returns histogram_quantile query", () => {
      const query = LLMQueries.p95Latency();
      expect(query).toContain("histogram_quantile(0.95");
      expect(query).toContain("_bucket");
    });

    it("byProviderModel aggregates by labels", () => {
      const query = LLMQueries.byProviderModel(LLM_METRICS.COST_USD);
      expect(query).toContain("sum by");
      expect(query).toContain(LABELS.AGENT);
      expect(query).toContain(LABELS.PROVIDER);
      expect(query).toContain(LABELS.MODEL);
    });
  });

  describe("SystemQueries", () => {
    it("totalConnections delegates to AgentQueries", () => {
      expect(SystemQueries.totalConnections()).toBe(
        AgentQueries.connectionsActive()
      );
    });

    it("totalRequestRate returns LLM request rate", () => {
      const query = SystemQueries.totalRequestRate();
      expect(query).toContain(LLM_METRICS.REQUESTS_TOTAL);
    });

    it("totalRequestRate respects custom window", () => {
      const query = SystemQueries.totalRequestRate("1h");
      expect(query).toContain("[1h]");
    });

    it("systemP95Latency returns agent p95 latency", () => {
      const query = SystemQueries.systemP95Latency();
      expect(query).toContain("histogram_quantile(0.95");
    });

    it("cost24h returns 24h cost increase", () => {
      const query = SystemQueries.cost24h();
      expect(query).toContain("increase(");
      expect(query).toContain("[24h]");
      expect(query).toContain(LLM_METRICS.COST_USD);
    });

    it("tokensPerMinute returns combined token rate", () => {
      const query = SystemQueries.tokensPerMinute();
      expect(query).toContain(LLM_METRICS.INPUT_TOKENS);
      expect(query).toContain(LLM_METRICS.OUTPUT_TOKENS);
      expect(query).toContain("/ 5"); // Divide by 5 for per-minute
    });

    it("costByProvider aggregates by provider", () => {
      const query = SystemQueries.costByProvider();
      expect(query).toContain(`sum by (${LABELS.PROVIDER})`);
      expect(query).toContain(LLM_METRICS.COST_USD);
    });

    it("costByProvider respects custom window", () => {
      const query = SystemQueries.costByProvider("6h");
      expect(query).toContain("[6h]");
    });
  });

  describe("buildEvalSelector", () => {
    it("returns empty string for undefined filter", () => {
      expect(buildEvalSelector()).toBe("");
    });

    it("returns empty string for empty filter", () => {
      expect(buildEvalSelector({})).toBe("");
    });

    it("builds selector with agent", () => {
      expect(buildEvalSelector({ agent: "my-agent" })).toBe(
        'agent="my-agent"'
      );
    });

    it("builds selector with promptpackName", () => {
      expect(buildEvalSelector({ promptpackName: "my-pack" })).toBe(
        'promptpack_name="my-pack"'
      );
    });

    it("builds selector with both labels", () => {
      const sel = buildEvalSelector({
        agent: "a",
        promptpackName: "p",
      });
      expect(sel).toContain('agent="a"');
      expect(sel).toContain('promptpack_name="p"');
    });
  });

  describe("EvalQueries", () => {
    it("discoverMetrics returns regex selector", () => {
      const query = EvalQueries.discoverMetrics();
      expect(query).toContain(EVAL_METRIC_PATTERN);
      expect(query).toContain("__name__=~");
    });

    it("discoverMetrics with filter includes labels", () => {
      const query = EvalQueries.discoverMetrics({ agent: "test" });
      expect(query).toContain('agent="test"');
    });

    it("metricValue returns metric name", () => {
      expect(EvalQueries.metricValue("omnia_eval_foo")).toBe("omnia_eval_foo");
    });

    it("metricValue with filter adds labels", () => {
      const query = EvalQueries.metricValue("omnia_eval_foo", {
        agent: "a",
      });
      expect(query).toBe('omnia_eval_foo{agent="a"}');
    });

    it("metricSum wraps in sum()", () => {
      const query = EvalQueries.metricSum("omnia_eval_foo");
      expect(query).toBe("sum(omnia_eval_foo)");
    });

    it("metricAvgOverTime uses default window", () => {
      const query = EvalQueries.metricAvgOverTime("omnia_eval_foo");
      expect(query).toContain("avg_over_time(");
      expect(query).toContain("[1h]");
    });

    it("metricRate uses custom window", () => {
      const query = EvalQueries.metricRate("omnia_eval_foo", "10m");
      expect(query).toContain("rate(");
      expect(query).toContain("[10m]");
    });

    it("discoverAgents groups by agent", () => {
      const query = EvalQueries.discoverAgents();
      expect(query).toContain("group(");
      expect(query).toContain(`by (${LABELS.AGENT})`);
    });

    it("discoverPromptPacks groups by promptpack_name", () => {
      const query = EvalQueries.discoverPromptPacks();
      expect(query).toContain("group(");
      expect(query).toContain(`by (${LABELS.PROMPTPACK_NAME})`);
    });
  });
});
