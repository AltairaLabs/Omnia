/**
 * Tests for import converters.
 */

import { describe, it, expect } from "vitest";
import {
  convertProviderToArena,
  generateProviderFilename,
  convertToolToArena,
  generateToolFilename,
} from "./import-converters";
import type { Provider } from "@/types/generated/provider";
import type { DiscoveredTool } from "@/types/tool-registry";

describe("convertProviderToArena", () => {
  it("should convert a basic provider", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "my-provider",
        namespace: "default",
      },
      spec: {
        type: "openai",
        model: "gpt-4",
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
    expect(yaml).toContain("kind: Provider");
    expect(yaml).toContain("name: default-my-provider");
    expect(yaml).toContain("omnia.altairalabs.ai/provider-name: my-provider");
    expect(yaml).toContain("omnia.altairalabs.ai/provider-namespace: default");
    expect(yaml).toContain("id: default-my-provider");
    expect(yaml).toContain("type: openai");
    expect(yaml).toContain("model: gpt-4");
  });

  it("should handle provider without namespace", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "my-provider",
      },
      spec: {
        type: "claude",
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("name: default-my-provider");
    expect(yaml).toContain("omnia.altairalabs.ai/provider-namespace: default");
  });

  it("should convert temperature default", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          temperature: "0.7",
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("defaults:");
    expect(yaml).toContain("temperature: 0.7");
  });

  it("should convert maxTokens default", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          maxTokens: 1000,
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("max_tokens: 1000");
  });

  it("should convert topP default", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          topP: "0.9",
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("top_p: 0.9");
  });

  it("should convert contextWindow default", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          contextWindow: 4096,
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("context_window: 4096");
  });

  it("should convert truncationStrategy default", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          truncationStrategy: "sliding",
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("truncation_strategy: sliding");
  });

  it("should include baseURL when present", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "ollama",
        baseURL: "https://api.example.com",
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("base_url: https://api.example.com");
  });

  it("should include capabilities when present", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "claude",
        capabilities: ["text", "streaming", "vision", "tools"],
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).toContain("  capabilities:");
    expect(yaml).toContain("    - text");
    expect(yaml).toContain("    - streaming");
    expect(yaml).toContain("    - vision");
    expect(yaml).toContain("    - tools");
  });

  it("should omit capabilities when empty", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        capabilities: [],
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).not.toContain("capabilities:");
  });

  it("should omit capabilities when not specified", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).not.toContain("capabilities:");
  });

  it("should skip invalid temperature", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          temperature: "invalid",
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).not.toContain("temperature:");
  });

  it("should skip invalid topP", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "test",
        namespace: "ns",
      },
      spec: {
        type: "openai",
        defaults: {
          topP: "not-a-number",
        },
      },
    };

    const yaml = convertProviderToArena(provider);

    expect(yaml).not.toContain("top_p:");
  });
});

describe("generateProviderFilename", () => {
  it("should generate filename with namespace", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "my-provider",
        namespace: "test-ns",
      },
      spec: {
        type: "openai",
      },
    };

    const filename = generateProviderFilename(provider);

    expect(filename).toBe("test-ns-my-provider.provider.yaml");
  });

  it("should use default namespace when not specified", () => {
    const provider: Provider = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Provider",
      metadata: {
        name: "my-provider",
      },
      spec: {
        type: "openai",
      },
    };

    const filename = generateProviderFilename(provider);

    expect(filename).toBe("default-my-provider.provider.yaml");
  });
});

// Helper to create a minimal valid DiscoveredTool for tests
function createTool(overrides: Partial<DiscoveredTool> & { name: string }): DiscoveredTool {
  return {
    handlerName: "test-handler",
    description: "",
    endpoint: "http://localhost",
    status: "Available",
    ...overrides,
  };
}

describe("convertToolToArena", () => {
  it("should convert a basic tool", () => {
    const tool = createTool({
      name: "get-weather",
      description: "Get weather data",
    });

    const yaml = convertToolToArena(tool, {
      registryName: "my-registry",
      registryNamespace: "default",
    });

    expect(yaml).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
    expect(yaml).toContain("kind: Tool");
    expect(yaml).toContain("name: my-registry-get-weather");
    expect(yaml).toContain("omnia.altairalabs.ai/registry-name: my-registry");
    expect(yaml).toContain("omnia.altairalabs.ai/registry-namespace: default");
    expect(yaml).toContain("omnia.altairalabs.ai/tool-name: get-weather");
    expect(yaml).toContain("description: Get weather data");
  });

  it("should handle tool with input schema", () => {
    const tool = createTool({
      name: "search",
      description: "Search for items",
      inputSchema: {
        type: "object",
        properties: {
          query: {
            type: "string",
            description: "Search query",
          },
        },
        required: ["query"],
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "registry",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("parameters:");
    expect(yaml).toContain("type: object");
    expect(yaml).toContain("properties:");
    expect(yaml).toContain("query:");
    expect(yaml).toContain("type: string");
    expect(yaml).toContain("required: [query]");
  });

  it("should handle tool without description", () => {
    const tool = createTool({
      name: "my-tool",
      description: "",
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    // Empty description outputs as empty string (no quotes needed)
    expect(yaml).toContain("description:");
  });

  it("should handle complex input schema with enums", () => {
    const tool = createTool({
      name: "api-call",
      description: "Make API call",
      inputSchema: {
        type: "object",
        properties: {
          method: {
            type: "string",
            description: "HTTP method",
            enum: ["GET", "POST", "PUT"],
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "http",
      registryNamespace: "default",
    });

    expect(yaml).toContain("enum: [GET, POST, PUT]");
  });

  it("should handle schema property with default value", () => {
    const tool = createTool({
      name: "test",
      inputSchema: {
        type: "object",
        properties: {
          limit: {
            type: "number",
            default: 10,
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("default: 10");
  });

  it("should handle empty input schema", () => {
    const tool = createTool({
      name: "no-params",
      inputSchema: null,
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("parameters:");
    expect(yaml).toContain("type: object");
    expect(yaml).toContain("properties: {}");
    expect(yaml).toContain("required: []");
  });

  it("should handle descriptions with special characters", () => {
    const tool = createTool({
      name: "special",
      description: 'Tool with "quotes" and: colons',
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain('description: "Tool with \\"quotes\\" and: colons"');
  });

  it("should handle property descriptions with special characters", () => {
    const tool = createTool({
      name: "special",
      inputSchema: {
        type: "object",
        properties: {
          name: {
            type: "string",
            description: "Name with #hashtag and: colon",
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain('description: "Name with #hashtag and: colon"');
  });

  it("should handle string default values", () => {
    const tool = createTool({
      name: "test",
      inputSchema: {
        type: "object",
        properties: {
          format: {
            type: "string",
            default: "json",
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("default: json");
  });

  it("should handle boolean default values", () => {
    const tool = createTool({
      name: "test",
      inputSchema: {
        type: "object",
        properties: {
          enabled: {
            type: "boolean",
            default: true,
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("default: true");
  });

  it("should handle null default values", () => {
    const tool = createTool({
      name: "test",
      inputSchema: {
        type: "object",
        properties: {
          optional: {
            type: "string",
            default: null,
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("default: null");
  });

  it("should handle array default values", () => {
    const tool = createTool({
      name: "test",
      inputSchema: {
        type: "object",
        properties: {
          tags: {
            type: "array",
            default: ["a", "b"],
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain("default: [a, b]");
  });

  it("should handle object default values", () => {
    const tool = createTool({
      name: "test",
      inputSchema: {
        type: "object",
        properties: {
          config: {
            type: "object",
            default: { key: "value" },
          },
        },
      },
    });

    const yaml = convertToolToArena(tool, {
      registryName: "reg",
      registryNamespace: "ns",
    });

    expect(yaml).toContain('default: {"key":"value"}');
  });
});

describe("generateToolFilename", () => {
  it("should generate filename from registry and tool name", () => {
    const tool = createTool({
      name: "get-weather",
    });

    const filename = generateToolFilename(tool, "my-registry");

    expect(filename).toBe("my-registry-get-weather.tool.yaml");
  });

  it("should sanitize tool name with special characters", () => {
    const tool = createTool({
      name: "search/with.dots",
    });

    const filename = generateToolFilename(tool, "registry");

    expect(filename).toBe("registry-search-with-dots.tool.yaml");
  });
});
