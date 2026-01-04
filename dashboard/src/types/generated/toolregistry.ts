// Auto-generated from omnia.altairalabs.ai_toolregistries.yaml
// Do not edit manually - run 'make generate-dashboard-types' to regenerate

import type { ObjectMeta, Condition } from "../common";

export interface ToolRegistrySpec {
  /** handlers defines the list of tool handlers in this registry.
   * Each handler can expose one or more tools. */
  handlers: {
    /** grpcConfig contains gRPC-specific configuration.
     * Required when type is "grpc". */
    grpcConfig?: {
      /** endpoint is the gRPC server address (host:port). */
      endpoint: string;
      /** tls enables TLS for the connection. */
      tls?: boolean;
      /** tlsCAPath is the path to the CA certificate. */
      tlsCAPath?: string;
      /** tlsCertPath is the path to the TLS certificate. */
      tlsCertPath?: string;
      /** tlsInsecureSkipVerify skips TLS verification (not recommended for production). */
      tlsInsecureSkipVerify?: boolean;
      /** tlsKeyPath is the path to the TLS key. */
      tlsKeyPath?: string;
    };
    /** httpConfig contains HTTP-specific configuration.
     * Required when type is "http". */
    httpConfig?: {
      /** authSecretRef references a secret containing auth credentials. */
      authSecretRef?: {
        /** key is the key in the secret to select. */
        key: string;
        /** name is the name of the secret. */
        name: string;
      };
      /** authType specifies the authentication type (none, bearer, basic). */
      authType?: string;
      /** contentType is the Content-Type header value.
       * Defaults to "application/json". */
      contentType?: string;
      /** endpoint is the HTTP endpoint URL. */
      endpoint: string;
      /** headers are additional HTTP headers to include in requests. */
      headers?: Record<string, string>;
      /** method is the HTTP method to use (GET, POST, PUT, DELETE).
       * Defaults to POST. */
      method?: string;
    };
    /** mcpConfig contains MCP-specific configuration.
     * Required when type is "mcp". */
    mcpConfig?: {
      /** args are the command arguments for stdio transport. */
      args?: string[];
      /** command is the command to run for stdio transport. */
      command?: string;
      /** endpoint is the SSE server URL (required for SSE transport). */
      endpoint?: string;
      /** env are additional environment variables for stdio transport. */
      env?: Record<string, string>;
      /** transport specifies the MCP transport type. */
      transport: "sse" | "stdio";
      /** workDir is the working directory for stdio transport. */
      workDir?: string;
    };
    /** name is a unique identifier for this handler within the registry. */
    name: string;
    /** openAPIConfig contains OpenAPI-specific configuration.
     * Required when type is "openapi". */
    openAPIConfig?: {
      /** baseURL overrides the base URL from the OpenAPI spec.
       * If not specified, uses the first server URL from the spec. */
      baseURL?: string;
      /** operationFilter limits which operations are exposed as tools.
       * If empty, all operations are exposed. */
      operationFilter?: string[];
      /** specURL is the URL to the OpenAPI specification (JSON or YAML). */
      specURL: string;
    };
    /** retries specifies the number of retry attempts on failure. */
    retries?: number;
    /** selector discovers the handler endpoint from Kubernetes Services.
     * Mutually exclusive with inline endpoint configuration. */
    selector?: {
      /** matchLabels specifies labels that must match on the Service. */
      matchLabels?: Record<string, string>;
      /** namespace specifies the namespace to search for Services.
       * If empty, searches in the same namespace as the ToolRegistry. */
      namespace?: string;
      /** port specifies the port name or number on the Service.
       * If empty, uses the first port. */
      port?: string;
    };
    /** timeout specifies the maximum duration for tool invocation.
     * Defaults to "30s". */
    timeout?: string;
    /** tool defines the tool interface (required for http and grpc types).
     * Self-describing handlers (mcp, openapi) discover tools automatically. */
    tool?: {
      /** description explains what the tool does (shown to LLM). */
      description: string;
      /** inputSchema is the JSON Schema for the tool's input parameters. */
      inputSchema: unknown;
      /** name is the tool name that will be exposed to the LLM. */
      name: string;
      /** outputSchema is the JSON Schema for the tool's output. */
      outputSchema?: unknown;
    };
    /** type specifies the handler protocol. */
    type: "http" | "openapi" | "grpc" | "mcp";
  }[];
}

export interface ToolRegistryStatus {
  /** conditions represent the current state of the ToolRegistry resource. */
  conditions?: {
    /** lastTransitionTime is the last time the condition transitioned from one status to another.
     * This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable. */
    lastTransitionTime: string;
    /** message is a human readable message indicating details about the transition.
     * This may be an empty string. */
    message: string;
    /** observedGeneration represents the .metadata.generation that the condition was set based upon.
     * For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
     * with respect to the current state of the instance. */
    observedGeneration?: number;
    /** reason contains a programmatic identifier indicating the reason for the condition's last transition.
     * Producers of specific condition types may define expected values and meanings for this field,
     * and whether the values are considered a guaranteed API.
     * The value should be a CamelCase string.
     * This field may not be empty. */
    reason: string;
    /** status of the condition, one of True, False, Unknown. */
    status: "True" | "False" | "Unknown";
    /** type of condition in CamelCase or in foo.example.com/CamelCase. */
    type: string;
  }[];
  /** discoveredTools contains details of each discovered tool. */
  discoveredTools?: {
    /** description is the tool description (for LLM) */
    description: string;
    /** endpoint is the resolved endpoint URL/address */
    endpoint: string;
    /** error contains the error message if status is Unavailable */
    error?: string;
    /** handlerName is the handler that provides this tool */
    handlerName: string;
    /** inputSchema is the JSON Schema for input parameters */
    inputSchema?: unknown;
    /** lastChecked is the timestamp of the last availability check */
    lastChecked?: string;
    /** name is the tool name (used by LLM) */
    name: string;
    /** outputSchema is the JSON Schema for output */
    outputSchema?: unknown;
    /** status indicates whether the tool is available */
    status: "Available" | "Unavailable" | "Unknown";
  }[];
  /** discoveredToolsCount is the number of tools successfully discovered. */
  discoveredToolsCount?: number;
  /** lastDiscoveryTime is the timestamp of the last successful discovery. */
  lastDiscoveryTime?: string;
  /** phase represents the current lifecycle phase of the ToolRegistry. */
  phase?: "Pending" | "Ready" | "Degraded" | "Failed";
}

export interface ToolRegistry {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "ToolRegistry";
  metadata: ObjectMeta;
  spec: ToolRegistrySpec;
  status?: ToolRegistryStatus;
}
