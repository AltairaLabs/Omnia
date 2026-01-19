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
 *       ├── OperatorApiService  - CRD + K8s data (agents, logs, events, etc.)
 *       └── PrometheusService   - Metrics/cost data from Prometheus
 *
 * The operator acts as the gateway to K8s data - the dashboard never
 * talks directly to K8s. All CRD operations, logs, events, and namespaces
 * go through the operator API.
 *
 * Usage:
 *   import { useDataService } from "@/lib/data";
 *   const service = useDataService();
 *   const agents = await service.getAgents();
 */

export * from "./types";
export { MockDataService, MockAgentConnection } from "./mock-service";
export { OperatorApiService } from "./operator-service";
export { WorkspaceApiService } from "./workspace-api-service";
export { PrometheusService } from "./prometheus-service";
export { LiveDataService, LiveAgentConnection } from "./live-service";
export { DataServiceProvider, useDataService, createDataService } from "./provider";
