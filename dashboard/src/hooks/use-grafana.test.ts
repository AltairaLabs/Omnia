import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  useGrafana,
  buildPanelUrl,
  buildDashboardUrl,
  GRAFANA_DASHBOARDS,
  OVERVIEW_PANELS,
  type GrafanaConfig,
} from "./use-grafana";

describe("useGrafana", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    vi.resetModules();
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  describe("when NEXT_PUBLIC_GRAFANA_URL is not set", () => {
    it("should return disabled config", () => {
      delete process.env.NEXT_PUBLIC_GRAFANA_URL;
      const { result } = renderHook(() => useGrafana());

      expect(result.current.enabled).toBe(false);
      expect(result.current.baseUrl).toBeNull();
    });

    it("should have default remote path", () => {
      delete process.env.NEXT_PUBLIC_GRAFANA_URL;
      const { result } = renderHook(() => useGrafana());

      expect(result.current.remotePath).toBe("/grafana/");
    });

    it("should have default org ID", () => {
      delete process.env.NEXT_PUBLIC_GRAFANA_URL;
      const { result } = renderHook(() => useGrafana());

      expect(result.current.orgId).toBe(1);
    });
  });

  describe("when NEXT_PUBLIC_GRAFANA_URL is set", () => {
    it("should return enabled config", () => {
      process.env.NEXT_PUBLIC_GRAFANA_URL = "https://grafana.example.com"; // NOSONAR - test URL
      const { result } = renderHook(() => useGrafana());

      expect(result.current.enabled).toBe(true);
      expect(result.current.baseUrl).toBe("https://grafana.example.com");
    });

    it("should use custom remote path", () => {
      process.env.NEXT_PUBLIC_GRAFANA_URL = "https://grafana.example.com"; // NOSONAR - test URL
      process.env.NEXT_PUBLIC_GRAFANA_PATH = "/monitoring";
      const { result } = renderHook(() => useGrafana());

      expect(result.current.remotePath).toBe("/monitoring/");
    });

    it("should normalize path without leading slash", () => {
      process.env.NEXT_PUBLIC_GRAFANA_URL = "https://grafana.example.com"; // NOSONAR - test URL
      process.env.NEXT_PUBLIC_GRAFANA_PATH = "metrics";
      const { result } = renderHook(() => useGrafana());

      expect(result.current.remotePath).toBe("/metrics/");
    });

    it("should use custom org ID", () => {
      process.env.NEXT_PUBLIC_GRAFANA_URL = "https://grafana.example.com"; // NOSONAR - test URL
      process.env.NEXT_PUBLIC_GRAFANA_ORG_ID = "5";
      const { result } = renderHook(() => useGrafana());

      expect(result.current.orgId).toBe(5);
    });
  });
});

describe("buildPanelUrl", () => {
  const enabledConfig: GrafanaConfig = {
    enabled: true,
    baseUrl: "https://grafana.example.com", // NOSONAR - test URL
    remotePath: "/grafana/",
    orgId: 1,
  };

  const disabledConfig: GrafanaConfig = {
    enabled: false,
    baseUrl: null,
    remotePath: "/grafana/",
    orgId: 1,
  };

  it("should return null when Grafana is disabled", () => {
    const url = buildPanelUrl(disabledConfig, {
      dashboardUid: GRAFANA_DASHBOARDS.OVERVIEW,
      panelId: OVERVIEW_PANELS.REQUESTS_PER_SEC,
    });

    expect(url).toBeNull();
  });

  it("should build correct panel URL", () => {
    const url = buildPanelUrl(enabledConfig, {
      dashboardUid: GRAFANA_DASHBOARDS.OVERVIEW,
      panelId: OVERVIEW_PANELS.REQUESTS_PER_SEC,
    });

    expect(url).toContain("https://grafana.example.com/grafana/d-solo/omnia-overview");
    expect(url).toContain("panelId=1");
    expect(url).toContain("orgId=1");
  });

  it("should include default parameters", () => {
    const url = buildPanelUrl(enabledConfig, {
      dashboardUid: "test-dash",
      panelId: 5,
    });

    expect(url).toContain("from=now-1h");
    expect(url).toContain("to=now");
    expect(url).toContain("refresh=30s");
    expect(url).toContain("theme=dark");
  });

  it("should accept custom parameters", () => {
    const url = buildPanelUrl(enabledConfig, {
      dashboardUid: "test-dash",
      panelId: 5,
      from: "now-24h",
      to: "now-1h",
      refresh: "1m",
      theme: "light",
    });

    expect(url).toContain("from=now-24h");
    expect(url).toContain("to=now-1h");
    expect(url).toContain("refresh=1m");
    expect(url).toContain("theme=light");
  });

  it("should include template variables with var- prefix", () => {
    const url = buildPanelUrl(enabledConfig, {
      dashboardUid: "agent-detail",
      panelId: 1,
      vars: { agent: "my-agent", namespace: "production" },
    });

    expect(url).toContain("var-agent=my-agent");
    expect(url).toContain("var-namespace=production");
  });

  it("should handle baseUrl with trailing slash", () => {
    const configWithSlash: GrafanaConfig = {
      enabled: true,
      baseUrl: "https://grafana.example.com/", // NOSONAR - test URL
      remotePath: "/grafana/",
      orgId: 1,
    };

    const url = buildPanelUrl(configWithSlash, {
      dashboardUid: "test",
      panelId: 1,
    });

    expect(url).toContain("https://grafana.example.com/grafana/d-solo/test");
    expect(url).not.toContain("//grafana/");
  });
});

describe("buildDashboardUrl", () => {
  const enabledConfig: GrafanaConfig = {
    enabled: true,
    baseUrl: "https://grafana.example.com", // NOSONAR - test URL
    remotePath: "/grafana/",
    orgId: 2,
  };

  const disabledConfig: GrafanaConfig = {
    enabled: false,
    baseUrl: null,
    remotePath: "/grafana/",
    orgId: 1,
  };

  it("should return null when Grafana is disabled", () => {
    const url = buildDashboardUrl(disabledConfig, GRAFANA_DASHBOARDS.OVERVIEW);

    expect(url).toBeNull();
  });

  it("should build correct dashboard URL", () => {
    const url = buildDashboardUrl(enabledConfig, GRAFANA_DASHBOARDS.OVERVIEW);

    expect(url).toContain("https://grafana.example.com/grafana/d/omnia-overview/_");
    expect(url).toContain("orgId=2");
  });

  it("should include template variables", () => {
    const url = buildDashboardUrl(enabledConfig, GRAFANA_DASHBOARDS.AGENT_DETAIL, {
      agent: "test-agent",
      namespace: "staging",
    });

    expect(url).toContain("var-agent=test-agent");
    expect(url).toContain("var-namespace=staging");
  });

  it("should handle empty vars", () => {
    const url = buildDashboardUrl(enabledConfig, GRAFANA_DASHBOARDS.COSTS, {});

    expect(url).not.toContain("var-");
    expect(url).toContain("orgId=2");
  });
});

describe("GRAFANA_DASHBOARDS", () => {
  it("should have correct dashboard UIDs", () => {
    expect(GRAFANA_DASHBOARDS.OVERVIEW).toBe("omnia-overview");
    expect(GRAFANA_DASHBOARDS.COSTS).toBe("omnia-costs");
    expect(GRAFANA_DASHBOARDS.AGENT_DETAIL).toBe("omnia-agent-detail");
    expect(GRAFANA_DASHBOARDS.LOGS).toBe("omnia-logs");
  });
});

describe("OVERVIEW_PANELS", () => {
  it("should have correct panel IDs", () => {
    expect(OVERVIEW_PANELS.REQUESTS_PER_SEC).toBe(1);
    expect(OVERVIEW_PANELS.P95_LATENCY).toBe(2);
    expect(OVERVIEW_PANELS.COST_24H).toBe(3);
    expect(OVERVIEW_PANELS.TOKENS_PER_MIN).toBe(4);
  });
});
