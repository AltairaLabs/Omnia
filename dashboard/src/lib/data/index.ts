/**
 * Data service module.
 *
 * Provides a unified data access layer that abstracts away
 * whether we're using mock data (demo mode) or real API data.
 *
 * Architecture:
 *
 *   DataService (interface)
 *   ├── MockDataService       - Mock data for demo mode
 *   └── LiveDataService       - Production data (composes:)
 *       ├── WorkspaceApiService - Workspace-scoped K8s data via ServiceAccount tokens
 *       └── PrometheusService   - Metrics/cost data from Prometheus
 *
 * The dashboard uses workspace-scoped ServiceAccount tokens to access K8s data
 * directly, with RBAC enforced at the K8s level.
 *
 * Usage:
 *   import { useDataService } from "@/lib/data";
 *   const service = useDataService();
 *   const agents = await service.getAgents("my-workspace");
 */

export * from "./types";
export { MockDataService, MockAgentConnection } from "./mock-service";
export { WorkspaceApiService } from "./workspace-api-service";
export { PrometheusService } from "./prometheus-service";
export { LiveDataService, LiveAgentConnection } from "./live-service";
export { SessionApiService } from "./session-api-service";
export { DataServiceProvider, useDataService, createDataService } from "./provider";
