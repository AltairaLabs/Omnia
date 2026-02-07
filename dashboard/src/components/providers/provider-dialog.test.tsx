import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ProviderDialog } from "./provider-dialog";
import type { Provider } from "@/types/generated/provider";

// Mock workspace context
const mockCurrentWorkspace = {
  name: "test-workspace",
  namespace: "test-namespace",
  role: "editor",
};

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
    workspaces: [mockCurrentWorkspace],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

// Mock provider mutations
const mockCreateProvider = vi.fn();
const mockUpdateProvider = vi.fn();

vi.mock("@/hooks/use-provider-mutations", () => ({
  useProviderMutations: () => ({
    createProvider: mockCreateProvider,
    updateProvider: mockUpdateProvider,
    loading: false,
    error: null,
  }),
}));

// Helper to create a mock Provider
function createMockProvider(overrides?: Partial<Provider>): Provider {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Provider",
    metadata: {
      name: "test-provider",
      namespace: "test-namespace",
      uid: "test-uid",
      creationTimestamp: "2025-01-01T00:00:00Z",
    },
    spec: {
      type: "claude",
      model: "claude-sonnet-4-20250514",
    },
    status: {
      phase: "Ready",
    },
    ...overrides,
  };
}

function createHyperscalerProvider(): Provider {
  return createMockProvider({
    metadata: {
      name: "bedrock-provider",
      namespace: "test-namespace",
      uid: "bedrock-uid",
      creationTimestamp: "2025-01-01T00:00:00Z",
    },
    spec: {
      type: "bedrock",
      model: "anthropic.claude-v2",
      platform: {
        type: "aws",
        region: "us-east-1",
      },
      auth: {
        type: "workloadIdentity",
      },
    },
  });
}

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("ProviderDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateProvider.mockResolvedValue(createMockProvider());
    mockUpdateProvider.mockResolvedValue(createMockProvider());
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("dialog rendering", () => {
    it("renders 'Create Provider' title when no provider prop", () => {
      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      expect(
        screen.getByRole("heading", { name: "Create Provider" })
      ).toBeInTheDocument();
    });

    it("renders 'Edit Provider' title when provider prop passed", () => {
      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={createMockProvider()}
          />
        </TestWrapper>
      );

      expect(
        screen.getByRole("heading", { name: "Edit Provider" })
      ).toBeInTheDocument();
    });

    it("does not render dialog when closed", () => {
      render(
        <TestWrapper>
          <ProviderDialog open={false} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      expect(screen.queryByText("Create Provider")).not.toBeInTheDocument();
    });
  });

  describe("edit mode", () => {
    it("disables name field in edit mode", () => {
      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={createMockProvider()}
          />
        </TestWrapper>
      );

      const nameInput = screen.getByLabelText("Name");
      expect(nameInput).toBeDisabled();
    });

    it("pre-fills form in edit mode", () => {
      const provider = createMockProvider({
        spec: {
          type: "claude",
          model: "claude-sonnet-4-20250514",
          baseURL: "https://api.custom.com",
        },
      });

      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={provider}
          />
        </TestWrapper>
      );

      const nameInput = screen.getByLabelText("Name") as HTMLInputElement;
      expect(nameInput.value).toBe("test-provider");

      const modelInput = screen.getByLabelText("Model") as HTMLInputElement;
      expect(modelInput.value).toBe("claude-sonnet-4-20250514");

      const baseURLInput = screen.getByLabelText("Base URL (optional)") as HTMLInputElement;
      expect(baseURLInput.value).toBe("https://api.custom.com");
    });
  });

  describe("validation", () => {
    it("validates required name field", async () => {
      vi.useRealTimers();
      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      // Click Create without filling name
      const submitButton = screen.getByRole("button", { name: /create provider/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText("Name is required")).toBeInTheDocument();
      });
      expect(mockCreateProvider).not.toHaveBeenCalled();
    });

    it("validates DNS name format", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      const nameInput = screen.getByLabelText("Name");
      await user.type(nameInput, "Invalid Name!");

      const submitButton = screen.getByRole("button", { name: /create provider/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(
          screen.getByText(/must be a valid DNS subdomain/i)
        ).toBeInTheDocument();
      });
    });
  });

  describe("conditional sections", () => {
    it("shows credential section for standard type (claude)", () => {
      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      // Default type is "claude" - should show credential section
      expect(screen.getByText("Credentials")).toBeInTheDocument();
      expect(screen.getByText("Credential Source")).toBeInTheDocument();
    });

    it("shows platform/auth sections for hyperscaler type", () => {
      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={createHyperscalerProvider()}
          />
        </TestWrapper>
      );

      expect(screen.getByText("Platform")).toBeInTheDocument();
      expect(screen.getByText("Authentication")).toBeInTheDocument();
    });

    it("hides credential section for hyperscaler type", () => {
      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={createHyperscalerProvider()}
          />
        </TestWrapper>
      );

      expect(screen.queryByText("Credential Source")).not.toBeInTheDocument();
    });

    it("hides credential section for local type (ollama)", () => {
      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={createMockProvider({
              spec: { type: "ollama" },
            })}
          />
        </TestWrapper>
      );

      expect(screen.queryByText("Credential Source")).not.toBeInTheDocument();
      expect(screen.queryByText("Platform")).not.toBeInTheDocument();
    });
  });

  describe("form submission", () => {
    it("creates provider with correct spec on submit", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      const onSuccess = vi.fn();
      const onOpenChange = vi.fn();

      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={onOpenChange}
            onSuccess={onSuccess}
          />
        </TestWrapper>
      );

      // Fill name
      const nameInput = screen.getByLabelText("Name");
      await user.type(nameInput, "my-new-provider");

      // Fill model
      const modelInput = screen.getByLabelText("Model");
      await user.type(modelInput, "claude-sonnet-4-20250514");

      // Fill credential secret name
      const secretInput = screen.getByLabelText("Secret Name");
      await user.type(secretInput, "my-api-key");

      // Submit
      const submitButton = screen.getByRole("button", { name: /create provider/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "my-new-provider",
          expect.objectContaining({
            type: "claude",
            model: "claude-sonnet-4-20250514",
            credential: {
              secretRef: { name: "my-api-key" },
            },
          })
        );
      });

      expect(onSuccess).toHaveBeenCalled();
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    it("creates hyperscaler provider with platform + auth fields", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      const onSuccess = vi.fn();

      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            onSuccess={onSuccess}
          />
        </TestWrapper>
      );

      // Fill name
      const nameInput = screen.getByLabelText("Name");
      await user.type(nameInput, "my-bedrock");

      // Change type to bedrock
      const typeSelect = screen.getByLabelText("Provider Type");
      fireEvent.click(typeSelect);
      const bedrockOption = await screen.findByRole("option", {
        name: "Amazon Bedrock",
      });
      fireEvent.click(bedrockOption);

      // Fill region
      const regionInput = screen.getByLabelText("Region");
      await user.type(regionInput, "us-west-2");

      // Submit
      const submitButton = screen.getByRole("button", { name: /create provider/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "my-bedrock",
          expect.objectContaining({
            type: "bedrock",
            platform: expect.objectContaining({
              type: "aws",
              region: "us-west-2",
            }),
            auth: expect.objectContaining({
              type: "workloadIdentity",
            }),
          })
        );
      });
    });

    it("updates provider in edit mode", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      const onSuccess = vi.fn();
      const provider = createMockProvider({
        spec: {
          type: "claude",
          model: "claude-sonnet-4-20250514",
          credential: {
            secretRef: { name: "my-secret" },
          },
        },
      });

      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={provider}
            onSuccess={onSuccess}
          />
        </TestWrapper>
      );

      // Change model
      const modelInput = screen.getByLabelText("Model") as HTMLInputElement;
      await user.clear(modelInput);
      await user.type(modelInput, "claude-opus-4-20250514");

      // Submit
      const submitButton = screen.getByRole("button", { name: /save changes/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockUpdateProvider).toHaveBeenCalledWith(
          "test-provider",
          expect.objectContaining({
            type: "claude",
            model: "claude-opus-4-20250514",
          })
        );
      });

      expect(onSuccess).toHaveBeenCalled();
    });

    it("shows error on mutation failure", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      mockCreateProvider.mockRejectedValue(new Error("API error: conflict"));

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      const nameInput = screen.getByLabelText("Name");
      await user.type(nameInput, "my-provider");

      // Fill credential secret name
      const secretInput = screen.getByLabelText("Secret Name");
      await user.type(secretInput, "my-api-key");

      const submitButton = screen.getByRole("button", { name: /create provider/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText("API error: conflict")).toBeInTheDocument();
      });
    });
  });

  describe("credential source switching", () => {
    it("switches to envVar credential source", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      const onSuccess = vi.fn();

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} onSuccess={onSuccess} />
        </TestWrapper>
      );

      // Fill name
      await user.type(screen.getByLabelText("Name"), "env-provider");

      // Select env var radio
      fireEvent.click(screen.getByLabelText("Env Variable"));

      // Fill env var
      await user.type(screen.getByLabelText("Environment Variable"), "MY_API_KEY");

      // Submit
      fireEvent.click(screen.getByRole("button", { name: /create provider/i }));

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "env-provider",
          expect.objectContaining({
            credential: { envVar: "MY_API_KEY" },
          })
        );
      });
    });

    it("switches to filePath credential source", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      await user.type(screen.getByLabelText("Name"), "file-provider");

      // Select file path radio
      fireEvent.click(screen.getByLabelText("File Path"));

      await user.type(screen.getByPlaceholderText("/var/run/secrets/api-key"), "/var/run/secrets/key");

      fireEvent.click(screen.getByRole("button", { name: /create provider/i }));

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "file-provider",
          expect.objectContaining({
            credential: { filePath: "/var/run/secrets/key" },
          })
        );
      });
    });
  });

  describe("capabilities toggling", () => {
    it("toggles capabilities on and off", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      await user.type(screen.getByLabelText("Name"), "cap-provider");
      await user.type(screen.getByLabelText("Secret Name"), "my-secret");

      // Expand the capabilities collapsible
      fireEvent.click(screen.getByRole("button", { name: /capabilities/i }));

      // Toggle text capability on
      fireEvent.click(screen.getByLabelText("text"));
      // Toggle streaming on
      fireEvent.click(screen.getByLabelText("streaming"));

      fireEvent.click(screen.getByRole("button", { name: /create provider/i }));

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "cap-provider",
          expect.objectContaining({
            capabilities: ["text", "streaming"],
          })
        );
      });
    });

    it("removes capability when toggled off", () => {
      render(
        <TestWrapper>
          <ProviderDialog
            open={true}
            onOpenChange={vi.fn()}
            provider={createMockProvider({
              spec: {
                type: "claude",
                capabilities: ["text", "streaming"],
                credential: { secretRef: { name: "s" } },
              },
            })}
          />
        </TestWrapper>
      );

      // text should be checked
      const textCheckbox = screen.getByLabelText("text");
      expect(textCheckbox).toBeChecked();

      // Toggle it off
      fireEvent.click(textCheckbox);
      expect(textCheckbox).not.toBeChecked();
    });
  });

  describe("defaults and pricing collapsibles", () => {
    it("shows defaults section and fills fields", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      await user.type(screen.getByLabelText("Name"), "defaults-provider");
      await user.type(screen.getByLabelText("Secret Name"), "my-secret");

      // Open defaults collapsible
      fireEvent.click(screen.getByRole("button", { name: /defaults/i }));

      // Fill defaults
      await user.type(screen.getByLabelText("Temperature"), "0.7");
      await user.type(screen.getByLabelText("Top P"), "0.9");
      await user.type(screen.getByLabelText("Max Tokens"), "4096");
      await user.type(screen.getByLabelText("Context Window"), "128000");

      fireEvent.click(screen.getByRole("button", { name: /create provider/i }));

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "defaults-provider",
          expect.objectContaining({
            defaults: {
              temperature: "0.7",
              topP: "0.9",
              maxTokens: 4096,
              contextWindow: 128_000,
            },
          })
        );
      });
    });

    it("shows pricing section and fills fields", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      await user.type(screen.getByLabelText("Name"), "pricing-provider");
      await user.type(screen.getByLabelText("Secret Name"), "my-secret");

      // Open pricing collapsible
      fireEvent.click(screen.getByRole("button", { name: /pricing/i }));

      await user.type(screen.getByLabelText("Input / 1K tokens"), "0.003");
      await user.type(screen.getByLabelText("Output / 1K tokens"), "0.015");
      await user.type(screen.getByLabelText("Cached / 1K tokens"), "0.0003");

      fireEvent.click(screen.getByRole("button", { name: /create provider/i }));

      await waitFor(() => {
        expect(mockCreateProvider).toHaveBeenCalledWith(
          "pricing-provider",
          expect.objectContaining({
            pricing: {
              inputCostPer1K: "0.003",
              outputCostPer1K: "0.015",
              cachedCostPer1K: "0.0003",
            },
          })
        );
      });
    });
  });

  describe("provider type change resets fields", () => {
    it("resets credential fields when switching to hyperscaler", async () => {
      vi.useRealTimers();

      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={vi.fn()} />
        </TestWrapper>
      );

      // Default type is "claude" - should show credential section
      expect(screen.getByText("Credentials")).toBeInTheDocument();

      // Switch to bedrock
      const typeSelect = screen.getByLabelText("Provider Type");
      fireEvent.click(typeSelect);
      const bedrockOption = await screen.findByRole("option", { name: "Amazon Bedrock" });
      fireEvent.click(bedrockOption);

      // Credential section should be hidden, platform/auth should show
      expect(screen.queryByText("Credential Source")).not.toBeInTheDocument();
      expect(screen.getByText("Platform")).toBeInTheDocument();
      expect(screen.getByText("Authentication")).toBeInTheDocument();
    });
  });

  describe("cancel behavior", () => {
    it("closes dialog on cancel", () => {
      const onOpenChange = vi.fn();
      render(
        <TestWrapper>
          <ProviderDialog open={true} onOpenChange={onOpenChange} />
        </TestWrapper>
      );

      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      fireEvent.click(cancelButton);

      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});
