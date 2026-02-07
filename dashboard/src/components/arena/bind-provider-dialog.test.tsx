import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BindProviderDialog } from "./bind-provider-dialog";

const mockProviders = [
  {
    metadata: { name: "provider-a", namespace: "default" },
    spec: { type: "openai", model: "gpt-4" },
  },
  {
    metadata: { name: "provider-b", namespace: "production" },
    spec: { type: "anthropic" },
  },
];

vi.mock("@/hooks/use-providers", () => ({
  useProviders: vi.fn(() => ({
    data: mockProviders,
    isLoading: false,
    error: null,
  })),
}));

describe("BindProviderDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    filePath: "test.provider.yaml",
    getContent: vi.fn(() => Promise.resolve("apiVersion: v1\nkind: Provider\nmetadata:\n  name: test\nspec:\n  type: openai\n")),
    saveContent: vi.fn(() => Promise.resolve()),
    onBound: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render dialog with title", () => {
    render(<BindProviderDialog {...defaultProps} />);
    expect(screen.getByText("Bind to Provider")).toBeInTheDocument();
  });

  it("should show file name in description", () => {
    render(<BindProviderDialog {...defaultProps} />);
    expect(screen.getByText(/test\.provider\.yaml/)).toBeInTheDocument();
  });

  it("should list available providers", () => {
    render(<BindProviderDialog {...defaultProps} />);
    expect(screen.getByText("provider-a")).toBeInTheDocument();
    expect(screen.getByText("provider-b")).toBeInTheDocument();
  });

  it("should show provider type info", () => {
    render(<BindProviderDialog {...defaultProps} />);
    expect(screen.getByText(/openai · gpt-4/)).toBeInTheDocument();
    expect(screen.getByText("anthropic")).toBeInTheDocument();
  });

  it("should have Bind button disabled when nothing selected", () => {
    render(<BindProviderDialog {...defaultProps} />);
    const bindButton = screen.getByRole("button", { name: "Bind" });
    expect(bindButton).toBeDisabled();
  });

  it("should enable Bind button when a provider is selected", () => {
    render(<BindProviderDialog {...defaultProps} />);
    fireEvent.click(screen.getByText("provider-a"));
    const bindButton = screen.getByRole("button", { name: "Bind" });
    expect(bindButton).toBeEnabled();
  });

  it("should call getContent and saveContent on bind", async () => {
    render(<BindProviderDialog {...defaultProps} />);
    fireEvent.click(screen.getByText("provider-a"));
    fireEvent.click(screen.getByRole("button", { name: "Bind" }));

    await waitFor(() => {
      expect(defaultProps.getContent).toHaveBeenCalled();
      expect(defaultProps.saveContent).toHaveBeenCalled();
    });
  });

  it("should call onBound after successful binding", async () => {
    render(<BindProviderDialog {...defaultProps} />);
    fireEvent.click(screen.getByText("provider-a"));
    fireEvent.click(screen.getByRole("button", { name: "Bind" }));

    await waitFor(() => {
      expect(defaultProps.onBound).toHaveBeenCalled();
    });
  });

  it("should show error on save failure", async () => {
    const props = {
      ...defaultProps,
      saveContent: vi.fn(() => Promise.reject(new Error("Save failed"))),
    };
    render(<BindProviderDialog {...props} />);
    fireEvent.click(screen.getByText("provider-a"));
    fireEvent.click(screen.getByRole("button", { name: "Bind" }));

    await waitFor(() => {
      expect(screen.getByText("Save failed")).toBeInTheDocument();
    });
  });

  it("should not render when open is false", () => {
    render(<BindProviderDialog {...defaultProps} open={false} />);
    expect(screen.queryByText("Bind to Provider")).not.toBeInTheDocument();
  });

  it("should have Cancel button", () => {
    render(<BindProviderDialog {...defaultProps} />);
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });
});
