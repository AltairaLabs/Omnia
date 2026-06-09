import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  ToolRegistryDialog,
  OpenAPIFields,
  McpFields,
  buildToolRegistrySpec,
  validateToolRegistryForm,
  emptyHandler,
  type HandlerForm,
  type ToolRegistryFormState,
} from "./tool-registry-dialog";

const mockCreateToolRegistry = vi.fn();

vi.mock("@/hooks/use-tool-registry-mutations", () => ({
  useToolRegistryMutations: () => ({
    createToolRegistry: mockCreateToolRegistry,
    loading: false,
    error: null,
  }),
}));

function handler(overrides: Partial<HandlerForm>): HandlerForm {
  return { ...emptyHandler(), name: "h1", ...overrides };
}

function form(overrides: Partial<ToolRegistryFormState>): ToolRegistryFormState {
  return { name: "my-tools", handlers: [handler({})], ...overrides };
}

describe("buildToolRegistrySpec", () => {
  it("builds an http handler with endpoint, method and tool", () => {
    const spec = buildToolRegistrySpec(
      form({
        handlers: [
          handler({
            name: "weather",
            type: "http",
            httpEndpoint: "https://svc/weather",
            httpMethod: "POST",
            httpToolName: "get_weather",
            httpToolDescription: "Get weather",
          }),
        ],
      })
    );
    expect(spec).toEqual({
      handlers: [
        {
          name: "weather",
          type: "http",
          httpConfig: { endpoint: "https://svc/weather", method: "POST" },
          tool: { name: "get_weather", description: "Get weather" },
        },
      ],
    });
  });

  it("builds an openapi handler with optional baseURL", () => {
    const spec = buildToolRegistrySpec(
      form({
        handlers: [
          handler({
            name: "petstore",
            type: "openapi",
            openapiSpecURL: "https://api/openapi.json",
            openapiBaseURL: "https://api",
          }),
        ],
      })
    );
    expect(spec.handlers[0]).toEqual({
      name: "petstore",
      type: "openapi",
      openAPIConfig: { specURL: "https://api/openapi.json", baseURL: "https://api" },
    });
  });

  it("builds an mcp sse handler with endpoint", () => {
    const spec = buildToolRegistrySpec(
      form({
        handlers: [
          handler({ name: "mcp1", type: "mcp", mcpTransport: "sse", mcpEndpoint: "https://mcp/sse" }),
        ],
      })
    );
    expect(spec.handlers[0]).toEqual({
      name: "mcp1",
      type: "mcp",
      mcpConfig: { transport: "sse", endpoint: "https://mcp/sse" },
    });
  });

  it("builds an mcp stdio handler with command and split args", () => {
    const spec = buildToolRegistrySpec(
      form({
        handlers: [
          handler({
            name: "mcp2",
            type: "mcp",
            mcpTransport: "stdio",
            mcpCommand: "npx",
            mcpArgs: "-y  @scope/server",
          }),
        ],
      })
    );
    expect(spec.handlers[0]).toEqual({
      name: "mcp2",
      type: "mcp",
      mcpConfig: { transport: "stdio", command: "npx", args: ["-y", "@scope/server"] },
    });
  });
});

describe("validateToolRegistryForm", () => {
  it("rejects empty name", () => {
    expect(validateToolRegistryForm(form({ name: "" }))).toMatch(/name is required/i);
  });
  it("rejects invalid DNS name", () => {
    expect(validateToolRegistryForm(form({ name: "Bad Name" }))).toMatch(/DNS/i);
  });
  it("rejects zero handlers", () => {
    expect(validateToolRegistryForm(form({ handlers: [] }))).toMatch(/at least one handler/i);
  });
  it("rejects http handler without endpoint", () => {
    expect(
      validateToolRegistryForm(form({ handlers: [handler({ type: "http", httpToolName: "t" })] }))
    ).toMatch(/endpoint is required/i);
  });
  it("rejects http handler without tool name", () => {
    expect(
      validateToolRegistryForm(
        form({ handlers: [handler({ type: "http", httpEndpoint: "https://x" })] })
      )
    ).toMatch(/tool name is required/i);
  });
  it("rejects openapi handler without spec URL", () => {
    expect(
      validateToolRegistryForm(form({ handlers: [handler({ type: "openapi" })] }))
    ).toMatch(/spec URL is required/i);
  });
  it("rejects mcp sse handler without endpoint", () => {
    expect(
      validateToolRegistryForm(
        form({ handlers: [handler({ type: "mcp", mcpTransport: "sse" })] })
      )
    ).toMatch(/endpoint is required/i);
  });
  it("rejects mcp stdio handler without command", () => {
    expect(
      validateToolRegistryForm(
        form({ handlers: [handler({ type: "mcp", mcpTransport: "stdio" })] })
      )
    ).toMatch(/command is required/i);
  });
  it("accepts a valid http handler", () => {
    expect(
      validateToolRegistryForm(
        form({
          handlers: [handler({ type: "http", httpEndpoint: "https://x", httpToolName: "t" })],
        })
      )
    ).toBeNull();
  });
});

describe("handler field components", () => {
  afterEach(() => cleanup());

  it("OpenAPIFields renders spec URL + base URL and reports changes", () => {
    const update = vi.fn();
    render(<OpenAPIFields h={handler({ type: "openapi" })} update={update} />);
    fireEvent.change(screen.getByLabelText("Spec URL"), {
      target: { value: "https://a/o.json" },
    });
    expect(update).toHaveBeenCalledWith(expect.any(String), {
      openapiSpecURL: "https://a/o.json",
    });
    expect(screen.getByLabelText("Base URL (optional)")).toBeInTheDocument();
  });

  it("McpFields renders an endpoint for sse transport", () => {
    render(<McpFields h={handler({ type: "mcp", mcpTransport: "sse" })} update={vi.fn()} />);
    expect(screen.getByLabelText("Endpoint")).toBeInTheDocument();
  });

  it("McpFields renders command + args for stdio transport", () => {
    render(<McpFields h={handler({ type: "mcp", mcpTransport: "stdio" })} update={vi.fn()} />);
    expect(screen.getByLabelText("Command")).toBeInTheDocument();
    expect(screen.getByLabelText("Args (space-separated)")).toBeInTheDocument();
  });
});

describe("ToolRegistryDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateToolRegistry.mockResolvedValue({});
  });

  afterEach(() => {
    cleanup();
  });

  it("renders the create dialog and a default handler", () => {
    render(<ToolRegistryDialog open onOpenChange={vi.fn()} />);
    expect(
      screen.getByRole("button", { name: "Create ToolRegistry" })
    ).toBeInTheDocument();
    expect(screen.getByText("Handler 1")).toBeInTheDocument();
  });

  it("adds and removes handlers", async () => {
    const user = userEvent.setup();
    render(<ToolRegistryDialog open onOpenChange={vi.fn()} />);
    expect(screen.getByText("Handler 1")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /add handler/i }));
    expect(screen.getByText("Handler 2")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Remove handler 2" }));
    expect(screen.queryByText("Handler 2")).not.toBeInTheDocument();
  });

  it("shows a validation error and does not create when name is empty", async () => {
    render(<ToolRegistryDialog open onOpenChange={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Create ToolRegistry" }));
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument();
    expect(mockCreateToolRegistry).not.toHaveBeenCalled();
  });

  it("creates a registry with an http handler on submit", async () => {
    const user = userEvent.setup();
    const onSuccess = vi.fn();
    const onOpenChange = vi.fn();
    render(<ToolRegistryDialog open onOpenChange={onOpenChange} onSuccess={onSuccess} />);

    await user.type(screen.getByLabelText("Name"), "my-tools");
    await user.type(screen.getByLabelText("Handler name"), "weather");
    await user.type(screen.getByLabelText("Endpoint"), "https://svc/weather");
    await user.type(screen.getByLabelText("Tool name"), "get_weather");
    await user.type(screen.getByLabelText("Method (optional)"), "POST");
    await user.type(screen.getByLabelText("Tool description (optional)"), "Get weather");

    await user.click(screen.getByRole("button", { name: "Create ToolRegistry" }));

    await waitFor(() => expect(mockCreateToolRegistry).toHaveBeenCalledTimes(1));
    expect(mockCreateToolRegistry).toHaveBeenCalledWith("my-tools", {
      handlers: [
        {
          name: "weather",
          type: "http",
          httpConfig: { endpoint: "https://svc/weather", method: "POST" },
          tool: { name: "get_weather", description: "Get weather" },
        },
      ],
    });
    expect(onSuccess).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
