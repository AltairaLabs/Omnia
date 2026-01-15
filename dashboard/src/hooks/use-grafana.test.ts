import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  useGrafana,
  buildPanelUrl,
  buildDashboardUrl,
  buildLokiExploreUrl,
  buildTempoExploreUrl,
  GRAFANA_DASHBOARDS,
  OVERVIEW_PANELS,
  type GrafanaConfig,
} from "./use-grafana";

// Mock the useGrafanaUrl hook
vi.mock("./use-runtime-config", () => ({
  useGrafanaUrl: vi.fn(() => ({ grafanaUrl: "", loading: false })),
}));

import { useGrafanaUrl } from "./use-runtime-config";

describe("useGrafana", () => {
  const originalEnv = process.env;
  const mockUseGrafanaUrl = vi.mocked(useGrafanaUrl);

  beforeEach(() => {
    vi.resetModules();
    process.env = { ...originalEnv };
    mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "", loading: false });
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  describe("when grafanaUrl is not set", () => {
    it("should return disabled config", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "", loading: false });
      const { result } = renderHook(() => useGrafana());

      expect(result.current.enabled).toBe(false);
      expect(result.current.baseUrl).toBeNull();
    });

    it("should have default remote path", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "", loading: false });
      const { result } = renderHook(() => useGrafana());

      expect(result.current.remotePath).toBe("/grafana/");
    });

    it("should have default org ID", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "", loading: false });
      const { result } = renderHook(() => useGrafana());

      expect(result.current.orgId).toBe(1);
    });
  });

  describe("when grafanaUrl is set via runtime config", () => {
    it("should return enabled config", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "https://grafana.example.com", loading: false }); // NOSONAR - test URL
      const { result } = renderHook(() => useGrafana());

      expect(result.current.enabled).toBe(true);
      expect(result.current.baseUrl).toBe("https://grafana.example.com");
    });

    it("should use custom remote path from env", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "https://grafana.example.com", loading: false }); // NOSONAR - test URL
      process.env.NEXT_PUBLIC_GRAFANA_PATH = "/monitoring";
      const { result } = renderHook(() => useGrafana());

      expect(result.current.remotePath).toBe("/monitoring/");
    });

    it("should normalize path without leading slash", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "https://grafana.example.com", loading: false }); // NOSONAR - test URL
      process.env.NEXT_PUBLIC_GRAFANA_PATH = "metrics";
      const { result } = renderHook(() => useGrafana());

      expect(result.current.remotePath).toBe("/metrics/");
    });

    it("should use custom org ID from env", () => {
      mockUseGrafanaUrl.mockReturnValue({ grafanaUrl: "https://grafana.example.com", loading: false }); // NOSONAR - test URL
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

describe("buildLokiExploreUrl", () => {
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
    const url = buildLokiExploreUrl(disabledConfig, "default", "my-agent");
    expect(url).toBeNull();
  });

  it("should return null when baseUrl is not set", () => {
    const configNoUrl: GrafanaConfig = {
      enabled: true,
      baseUrl: null,
      remotePath: "/grafana/",
      orgId: 1,
    };
    const url = buildLokiExploreUrl(configNoUrl, "default", "my-agent");
    expect(url).toBeNull();
  });

  it("should build correct Loki explore URL", () => {
    const url = buildLokiExploreUrl(enabledConfig, "default", "my-agent");

    expect(url).not.toBeNull();
    expect(url).toContain("https://grafana.example.com/grafana/explore");
    expect(url).toContain("orgId=1");
    expect(url).toContain("datasource");
    expect(url).toContain("loki");
  });

  it("should include namespace and agent in query", () => {
    const url = buildLokiExploreUrl(enabledConfig, "production", "test-agent");

    expect(url).not.toBeNull();
    expect(url).toContain("production");
    expect(url).toContain("test-agent");
  });

  it("should use custom time range options", () => {
    const url = buildLokiExploreUrl(enabledConfig, "default", "my-agent", {
      from: "now-24h",
      to: "now-1h",
    });

    expect(url).not.toBeNull();
    expect(url).toContain("now-24h");
    expect(url).toContain("now-1h");
  });

  it("should handle baseUrl with trailing slash", () => {
    const configWithSlash: GrafanaConfig = {
      enabled: true,
      baseUrl: "https://grafana.example.com/", // NOSONAR - test URL
      remotePath: "/grafana/",
      orgId: 1,
    };

    const url = buildLokiExploreUrl(configWithSlash, "default", "my-agent");

    expect(url).not.toBeNull();
    expect(url).toContain("https://grafana.example.com/grafana/explore");
    expect(url).not.toContain("//grafana/");
  });
});

describe("buildTempoExploreUrl", () => {
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
    const url = buildTempoExploreUrl(disabledConfig, "default", "my-agent");
    expect(url).toBeNull();
  });

  it("should return null when baseUrl is not set", () => {
    const configNoUrl: GrafanaConfig = {
      enabled: true,
      baseUrl: null,
      remotePath: "/grafana/",
      orgId: 1,
    };
    const url = buildTempoExploreUrl(configNoUrl, "default", "my-agent");
    expect(url).toBeNull();
  });

  it("should build correct Tempo explore URL", () => {
    const url = buildTempoExploreUrl(enabledConfig, "default", "my-agent");

    expect(url).not.toBeNull();
    expect(url).toContain("https://grafana.example.com/grafana/explore");
    expect(url).toContain("orgId=2");
    expect(url).toContain("datasource");
    expect(url).toContain("tempo");
  });

  it("should include service name with namespace and agent", () => {
    const url = buildTempoExploreUrl(enabledConfig, "production", "test-agent");

    expect(url).not.toBeNull();
    // TraceQL query includes service name as agent.namespace
    expect(url).toContain("test-agent.production");
  });

  it("should use custom time range options", () => {
    const url = buildTempoExploreUrl(enabledConfig, "default", "my-agent", {
      from: "now-6h",
      to: "now",
    });

    expect(url).not.toBeNull();
    expect(url).toContain("now-6h");
  });

  it("should handle baseUrl with trailing slash", () => {
    const configWithSlash: GrafanaConfig = {
      enabled: true,
      baseUrl: "https://grafana.example.com/", // NOSONAR - test URL
      remotePath: "/grafana/",
      orgId: 1,
    };

    const url = buildTempoExploreUrl(configWithSlash, "default", "my-agent");

    expect(url).not.toBeNull();
    expect(url).toContain("https://grafana.example.com/grafana/explore");
    expect(url).not.toContain("//grafana/");
  });
});
