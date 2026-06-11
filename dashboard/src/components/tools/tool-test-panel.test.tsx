/**
 * Tests for ToolTestPanel component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ToolTestPanel } from "./tool-test-panel";
import type { ToolRegistry } from "@/types";

// Mock the OpenAPI preview hook so tests are deterministic.
vi.mock("@/hooks/use-openapi-tool-preview", () => ({ useOpenAPIToolPreview: vi.fn() }));
import { useOpenAPIToolPreview } from "@/hooks/use-openapi-tool-preview";
const mockPreview = useOpenAPIToolPreview as ReturnType<typeof vi.fn>;

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockRegistry: ToolRegistry = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ToolRegistry",
  metadata: {
    name: "test-registry",
    namespace: "test-namespace",
    uid: "test-uid",
  },
  spec: {
    handlers: [
      {
        name: "http-handler",
        type: "http",
        tool: {
          name: "search",
          description: "Search things",
          inputSchema: {
            type: "object",
            properties: {
              query: { type: "string" },
              limit: { type: "number" },
            },
          },
        },
      },
      {
        name: "mcp-handler",
        type: "mcp",
      },
    ],
  },
  status: {
    phase: "Ready",
    discoveredToolsCount: 3,
    discoveredTools: [
      {
        name: "search",
        handlerName: "http-handler",
        description: "Search things",
        endpoint: "https://localhost:8080/search",
        status: "Available",
      },
      {
        name: "read_file",
        handlerName: "mcp-handler",
        description: "Read a file",
        endpoint: "https://mcp:8080",
        status: "Available",
        inputSchema: {
          type: "object",
          properties: {
            path: { type: "string" },
          },
        },
      },
      {
        name: "write_file",
        handlerName: "mcp-handler",
        description: "Write a file",
        endpoint: "https://mcp:8080",
        status: "Available",
      },
    ],
  },
};

describe("ToolTestPanel", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    mockPreview.mockReturnValue({ tools: [], specURL: null, error: null, loading: false });
  });

  it("renders handler select with all handlers", () => {
    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    expect(screen.getByText("Test Tool Call")).toBeInTheDocument();
    expect(screen.getByText("Run Test")).toBeInTheDocument();
    // Handler select should be present
    expect(screen.getByLabelText("Handler")).toBeInTheDocument();
  });

  it("renders arguments textarea with sample JSON", () => {
    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    const textarea = screen.getByLabelText("Arguments (JSON)");
    expect(textarea).toBeInTheDocument();
    // Should have sample args from the first handler's tool schema
    const value = (textarea as HTMLTextAreaElement).value;
    expect(value).toContain("query");
    expect(value).toContain("limit");
  });

  it("shows empty state when no handlers", () => {
    const emptyRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: { handlers: [] },
    };

    render(<ToolTestPanel registry={emptyRegistry} workspaceName="ws1" />);

    expect(screen.getByText("No handlers configured in this ToolRegistry")).toBeInTheDocument();
  });

  it("shows success result after test", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          success: true,
          result: { data: "hello" },
          durationMs: 42,
          handlerType: "http",
        }),
    });

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("Success")).toBeInTheDocument();
    });

    expect(screen.getByText("42ms")).toBeInTheDocument();
  });

  it("shows error result after failed test", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          success: false,
          error: "connection refused",
          durationMs: 5,
          handlerType: "http",
        }),
    });

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("Failed")).toBeInTheDocument();
    });

    expect(screen.getByText("connection refused")).toBeInTheDocument();
  });

  it("validates JSON before submitting", async () => {
    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    const textarea = screen.getByLabelText("Arguments (JSON)");
    fireEvent.change(textarea, { target: { value: "{invalid json" } });
    fireEvent.click(screen.getByText("Run Test"));

    expect(screen.getByText(/Invalid JSON/)).toBeInTheDocument();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("handles fetch errors gracefully", async () => {
    mockFetch.mockRejectedValueOnce(new Error("Network error"));

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("Failed")).toBeInTheDocument();
    });

    expect(screen.getByText("Network error")).toBeInTheDocument();
  });

  it("shows validation results when present", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          success: true,
          result: { data: "ok" },
          durationMs: 15,
          handlerType: "http",
          validation: {
            request: { valid: true },
            response: {
              valid: false,
              errors: ["status is required"],
            },
          },
        }),
    });

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("Schema Validation")).toBeInTheDocument();
    });

    expect(screen.getByText(/Request:/)).toBeInTheDocument();
    expect(screen.getByText("Valid")).toBeInTheDocument();
    expect(screen.getByText(/Response:/)).toBeInTheDocument();
    expect(screen.getByText("Invalid")).toBeInTheDocument();
    expect(screen.getByText("status is required")).toBeInTheDocument();
  });

  it("sends correct request body", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          success: true,
          result: null,
          durationMs: 10,
          handlerType: "http",
        }),
    });

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/workspaces/ws1/toolregistries/test-registry/test");
    expect(options.method).toBe("POST");

    const body = JSON.parse(options.body);
    expect(body.handlerName).toBe("http-handler");
  });

  it("updates args and clears result when handler changes", () => {
    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    // The handler select trigger shows the current value
    const handlerTrigger = screen.getByLabelText("Handler");
    fireEvent.click(handlerTrigger);

    // Select mcp-handler from the dropdown
    const mcpOption = screen.getByText("mcp-handler", { selector: "[role=option] *" });
    fireEvent.click(mcpOption);

    // The args textarea should now reflect the mcp handler's first discovered tool (read_file has path property)
    const textarea = screen.getByLabelText("Arguments (JSON)");
    const value = (textarea as HTMLTextAreaElement).value;
    expect(value).toContain("path");
  });

  it("updates args when tool changes within a handler", () => {
    // Start with mcp-handler selected (which has multiple discovered tools)
    const mcpFirstRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [
          {
            name: "mcp-handler",
            type: "mcp",
          },
        ],
      },
    };

    render(<ToolTestPanel registry={mcpFirstRegistry} workspaceName="ws1" />);

    // The tool select should be visible since mcp-handler has multiple tools and no inline tool
    const toolTrigger = screen.getByLabelText("Tool");
    fireEvent.click(toolTrigger);

    // Select write_file from dropdown options
    const options = screen.getAllByRole("option");
    const writeOption = options.find((opt) => opt.textContent === "write_file");
    expect(writeOption).toBeDefined();
    fireEvent.click(writeOption!);

    // Args should be reset (write_file has no inputSchema, so defaults to {})
    const textarea = screen.getByLabelText("Arguments (JSON)");
    const value = (textarea as HTMLTextAreaElement).value;
    expect(value).toBe("{}");
  });

  it("shows tool select dropdown when handler has multiple discovered tools and no inline tool", () => {
    const mcpOnlyRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [
          {
            name: "mcp-handler",
            type: "mcp",
          },
        ],
      },
    };

    render(<ToolTestPanel registry={mcpOnlyRegistry} workspaceName="ws1" />);

    // Tool select should be visible for mcp-handler (2 discovered tools, no inline tool)
    expect(screen.getByLabelText("Tool")).toBeInTheDocument();
  });

  it("does not show tool select for handler with only an inline tool", () => {
    const singleInlineRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [
          {
            name: "http-handler",
            type: "http",
            tool: {
              name: "search",
              description: "Search things",
              inputSchema: {
                type: "object",
                properties: { query: { type: "string" } },
              },
            },
          },
        ],
      },
      status: {
        phase: "Ready",
        discoveredToolsCount: 1,
        discoveredTools: [
          {
            name: "search",
            handlerName: "http-handler",
            description: "Search things",
            endpoint: "https://localhost:8080/search",
            status: "Available",
          },
        ],
      },
    };

    render(<ToolTestPanel registry={singleInlineRegistry} workspaceName="ws1" />);

    // Tool select should NOT be visible (1 discovered tool + has inline tool)
    expect(screen.queryByLabelText("Tool")).not.toBeInTheDocument();
  });

  it("handles non-Error thrown by fetch", async () => {
    mockFetch.mockRejectedValueOnce("string error");

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("Failed")).toBeInTheDocument();
    });

    expect(screen.getByText("Request failed")).toBeInTheDocument();
  });

  it("renders result as string when result is a string", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          success: true,
          result: "plain text response",
          durationMs: 20,
          handlerType: "http",
        }),
    });

    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("plain text response")).toBeInTheDocument();
    });
  });

  it("clears json error when textarea changes", async () => {
    render(<ToolTestPanel registry={mockRegistry} workspaceName="ws1" />);

    const textarea = screen.getByLabelText("Arguments (JSON)");
    fireEvent.change(textarea, { target: { value: "{invalid" } });
    fireEvent.click(screen.getByText("Run Test"));

    expect(screen.getByText(/Invalid JSON/)).toBeInTheDocument();

    // Change textarea to clear the error
    fireEvent.change(textarea, { target: { value: "{}" } });
    expect(screen.queryByText(/Invalid JSON/)).not.toBeInTheDocument();
  });

  it("handles registry with no status or discoveredTools", () => {
    const noStatusRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [
          {
            name: "grpc-handler",
            type: "grpc",
            tool: {
              name: "my-tool",
              description: "A tool",
              inputSchema: {
                type: "object",
                properties: {
                  enabled: { type: "boolean" },
                  count: { type: "integer" },
                  items: { type: "array" },
                  meta: { type: "object" },
                  other: { type: "unknown" },
                },
              },
            },
          },
        ],
      },
      status: undefined as unknown as ToolRegistry["status"],
    };

    render(<ToolTestPanel registry={noStatusRegistry} workspaceName="ws1" />);

    const textarea = screen.getByLabelText("Arguments (JSON)");
    const value = (textarea as HTMLTextAreaElement).value;
    const parsed = JSON.parse(value);
    expect(parsed.enabled).toBe(false);
    expect(parsed.count).toBe(0);
    expect(parsed.items).toEqual([]);
    expect(parsed.meta).toEqual({});
    expect(parsed.other).toBe(null);
  });

  it("generates empty sample args for tool with no schema properties", () => {
    const noSchemaRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [
          {
            name: "simple-handler",
            type: "http",
            tool: {
              name: "simple-tool",
              description: "No schema",
            },
          },
        ],
      },
      status: {
        phase: "Ready",
        discoveredToolsCount: 0,
        discoveredTools: [],
      },
    };

    render(<ToolTestPanel registry={noSchemaRegistry} workspaceName="ws1" />);

    const textarea = screen.getByLabelText("Arguments (JSON)");
    expect((textarea as HTMLTextAreaElement).value).toBe("{}");
  });

  it("generates empty sample args when inputSchema is not an object", () => {
    const badSchemaRegistry: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [
          {
            name: "bad-handler",
            type: "http",
            tool: {
              name: "bad-tool",
              description: "Bad schema",
              inputSchema: "not-an-object",
            },
          },
        ],
      },
      status: {
        phase: "Ready",
        discoveredToolsCount: 0,
        discoveredTools: [],
      },
    };

    render(<ToolTestPanel registry={badSchemaRegistry} workspaceName="ws1" />);

    const textarea = screen.getByLabelText("Arguments (JSON)");
    expect((textarea as HTMLTextAreaElement).value).toBe("{}");
  });
});

describe("ToolTestPanel — OpenAPI handler", () => {
  const openAPIRegistry: ToolRegistry = {
    ...mockRegistry,
    spec: {
      handlers: [
        {
          name: "petstore",
          type: "openapi",
          openAPIConfig: { specURL: "u" },
        },
      ],
    },
    status: {
      phase: "Ready",
      discoveredToolsCount: 0,
      discoveredTools: [],
    },
  };

  beforeEach(() => {
    mockFetch.mockReset();
    mockPreview.mockReturnValue({ tools: [], specURL: null, error: null, loading: false });
  });

  it("lists live operations and selects the first by default", async () => {
    mockPreview.mockReturnValue({
      tools: [
        {
          name: "getPet",
          description: "Fetch a pet",
          inputSchema: { type: "object", properties: { id: { type: "string" } } },
        },
        { name: "listPets", description: "List pets" },
      ],
      specURL: "https://api/openapi.json",
      error: null,
      loading: false,
    });

    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws1" />);

    // The live operation getPet should appear (tool select + inspector).
    expect((await screen.findAllByText("getPet")).length).toBeGreaterThan(0);
    // getPet has a renderable schema → Form mode by default; switch to JSON to
    // inspect the seeded sample args.
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const textarea = screen.getByLabelText("Arguments (JSON)");
    expect((textarea as HTMLTextAreaElement).value).toContain("id");
  });

  it("shows a discovery-error card instead of an empty select", () => {
    mockPreview.mockReturnValue({
      tools: [],
      specURL: "https://api/openapi.json",
      error: "failed to fetch OpenAPI spec: connection refused",
      loading: false,
    });

    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws1" />);

    expect(screen.getByText(/couldn't load tools/i)).toBeInTheDocument();
    expect(
      screen.getByText("failed to fetch OpenAPI spec: connection refused")
    ).toBeInTheDocument();
    expect(screen.getByText("https://api/openapi.json")).toBeInTheDocument();
    // No empty Tool select rendered.
    expect(screen.queryByLabelText("Tool")).not.toBeInTheDocument();
  });

  it("shows a loading state while tools are being discovered", () => {
    mockPreview.mockReturnValue({ tools: [], specURL: null, error: null, loading: true });

    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws1" />);

    expect(screen.getByText(/loading tools/i)).toBeInTheDocument();
  });

  it("runs a live OpenAPI tool and shows the result", async () => {
    mockPreview.mockReturnValue({
      tools: [
        {
          name: "getPet",
          description: "Fetch a pet",
          inputSchema: { type: "object", properties: { id: { type: "string" } } },
        },
      ],
      specURL: "u",
      error: null,
      loading: false,
    });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          success: true,
          result: { ok: true },
          durationMs: 7,
          handlerType: "openapi",
        }),
    });

    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws1" />);

    fireEvent.click(screen.getByText("Run Test"));

    await waitFor(() => {
      expect(screen.getByText("Success")).toBeInTheDocument();
    });

    const [url, options] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/workspaces/ws1/toolregistries/test-registry/test");
    const body = JSON.parse(options.body);
    expect(body.handlerName).toBe("petstore");
    expect(body.toolName).toBe("getPet");
  });

  it("validates JSON and reseeds args when the live tool changes", () => {
    mockPreview.mockReturnValue({
      tools: [
        {
          name: "getPet",
          description: "Fetch a pet",
          inputSchema: { type: "object", properties: { id: { type: "string" } } },
        },
        { name: "listPets", description: "List pets" },
      ],
      specURL: "u",
      error: null,
      loading: false,
    });

    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws1" />);

    // getPet has a renderable schema → Form mode by default; switch to JSON to
    // exercise the raw-JSON validation path.
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));

    // Invalid JSON blocks the run.
    const textarea = screen.getByLabelText("Arguments (JSON)");
    fireEvent.change(textarea, { target: { value: "{bad" } });
    fireEvent.click(screen.getByText("Run Test"));
    expect(screen.getByText(/Invalid JSON/)).toBeInTheDocument();
    expect(mockFetch).not.toHaveBeenCalled();

    // Switching to listPets (no schema) reseeds args to "{}".
    fireEvent.click(screen.getByLabelText("Tool"));
    const listOption = screen
      .getAllByRole("option")
      .find((o) => o.textContent === "listPets");
    fireEvent.click(listOption!);
    expect((screen.getByLabelText("Arguments (JSON)") as HTMLTextAreaElement).value).toBe("{}");
  });

  it("shows the selected tool's description and a weak-description lint", async () => {
    mockPreview.mockReturnValue({
      tools: [
        {
          name: "getPet",
          description: "GET /pets/{id}",
          inputSchema: {
            type: "object",
            properties: { id: { type: "string" } },
            required: ["id"],
          },
        },
      ],
      specURL: "u",
      error: null,
      loading: false,
    });
    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws" />);
    expect(await screen.findByText("GET /pets/{id}")).toBeInTheDocument();
    expect(
      screen.getByText(/method and path as this tool's description/i)
    ).toBeInTheDocument();
  });

  it("renders a schema-driven form for the selected tool by default", async () => {
    mockPreview.mockReturnValue({
      tools: [
        {
          name: "getPet",
          description: "Fetch a pet",
          inputSchema: {
            type: "object",
            properties: { id: { type: "string", description: "pet id" } },
            required: ["id"],
          },
        },
      ],
      specURL: "u",
      error: null,
      loading: false,
    });
    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws" />);
    expect(await screen.findByLabelText(/id/i)).toBeInTheDocument();
  });

  it("filters operations via search when there are many", async () => {
    const tools = Array.from({ length: 10 }, (_, i) => ({
      name: `op${i}`,
      description: `desc ${i}`,
      inputSchema: { type: "object", properties: {} },
    }));
    mockPreview.mockReturnValue({ tools, specURL: "u", error: null, loading: false });
    render(<ToolTestPanel registry={openAPIRegistry} workspaceName="ws" />);
    const search = await screen.findByPlaceholderText(/search operations/i);
    fireEvent.change(search, { target: { value: "op7" } });
    expect(screen.getByText("op7")).toBeInTheDocument();
  });
});
