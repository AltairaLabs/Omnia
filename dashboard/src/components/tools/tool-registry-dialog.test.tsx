import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  ToolRegistryDialog,
  OpenAPIFields,
  McpFields,
  buildToolRegistrySpec,
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

describe("inline validation via ToolRegistryDialog", () => {
  afterEach(() => cleanup());

  it("shows an inline error for an invalid resource name and blocks submit", async () => {
    const user = userEvent.setup();
    render(<ToolRegistryDialog open onOpenChange={() => {}} />);

    const nameInput = screen.getByLabelText("Name");
    await user.type(nameInput, "Bad Name");

    expect(
      await screen.findByText(/lowercase letters, numbers, hyphens/i)
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /create/i })).toBeDisabled();
  });

  it("shows required errors on submit when name is empty", async () => {
    render(<ToolRegistryDialog open onOpenChange={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Create ToolRegistry" }));
    const errors = await screen.findAllByText(/this field is required/i);
    expect(errors.length).toBeGreaterThan(0);
    expect(mockCreateToolRegistry).not.toHaveBeenCalled();
  });

  it("shows an inline error for an invalid handler name pattern", async () => {
    const user = userEvent.setup();
    render(<ToolRegistryDialog open onOpenChange={() => {}} />);

    const handlerName = screen.getByLabelText(/handler name/i);
    await user.type(handlerName, "Api");

    expect(
      await screen.findByText(/lowercase letters, numbers, and hyphens/i)
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /create/i })).toBeDisabled();
  });

  it("clears the handler name error when input becomes valid", async () => {
    const user = userEvent.setup();
    render(<ToolRegistryDialog open onOpenChange={() => {}} />);

    const handlerName = screen.getByLabelText(/handler name/i);
    await user.type(handlerName, "Api");
    expect(
      await screen.findByText(/lowercase letters, numbers, and hyphens/i)
    ).toBeInTheDocument();

    await user.clear(handlerName);
    await user.type(handlerName, "valid-name");
    await waitFor(() => {
      expect(
        screen.queryByText(/lowercase letters, numbers, and hyphens/i)
      ).not.toBeInTheDocument();
    });
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

  it("handler-name input aria-describedby points at the rendered FieldError id", async () => {
    const user = userEvent.setup();
    render(<ToolRegistryDialog open onOpenChange={() => {}} />);

    const handlerName = screen.getByLabelText(/handler name/i);
    await user.type(handlerName, "Api");

    const errorText = await screen.findByText(/lowercase letters, numbers, and hyphens/i);
    const describedById = handlerName.getAttribute("aria-describedby");
    expect(describedById).toBeTruthy();
    const errorElement = document.getElementById(describedById!);
    expect(errorElement).not.toBeNull();
    expect(errorElement).toContainElement(errorText as HTMLElement);
  });

  it("shows inline required errors and does not create when name is empty", async () => {
    render(<ToolRegistryDialog open onOpenChange={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Create ToolRegistry" }));
    const errs = await screen.findAllByText(/this field is required/i);
    expect(errs.length).toBeGreaterThan(0);
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
