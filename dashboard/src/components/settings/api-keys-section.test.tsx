/**
 * Tests for ApiKeysSection component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ApiKeysSection } from "./api-keys-section";
import { usePermissions, Permission } from "@/hooks";
import type { PermissionType } from "@/lib/auth/permissions";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@/hooks", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/hooks")>();
  return {
    ...actual,
    usePermissions: vi.fn(),
  };
});

const mockUsePermissions = vi.mocked(usePermissions);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return function QueryWrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

const mockApiKeysResponse = {
  keys: [
    {
      id: "key-1",
      name: "Test Key 1",
      keyPrefix: "omnia_***abc",
      role: "admin",
      expiresAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
      createdAt: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
      lastUsedAt: new Date(Date.now() - 1 * 60 * 60 * 1000).toISOString(),
      isExpired: false,
    },
    {
      id: "key-2",
      name: "Expired Key",
      keyPrefix: "omnia_***xyz",
      role: "viewer",
      expiresAt: new Date(Date.now() - 1 * 24 * 60 * 60 * 1000).toISOString(),
      createdAt: new Date(Date.now() - 90 * 24 * 60 * 60 * 1000).toISOString(),
      lastUsedAt: null,
      isExpired: true,
    },
  ],
  config: {
    storeType: "memory" as const,
    allowCreate: true,
    maxKeysPerUser: 5,
    defaultExpirationDays: 90,
  },
};

function createMockPermissions(canFn: (p: PermissionType) => boolean, perms: PermissionType[]) {
  return {
    can: canFn,
    canAll: (ps: PermissionType[]) => ps.every(canFn),
    canAny: (ps: PermissionType[]) => ps.some(canFn),
    permissions: new Set(perms),
  };
}

describe("ApiKeysSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it("should not render when user lacks view permission", () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        (p) => p !== Permission.API_KEYS_VIEW_OWN,
        []
      )
    );

    const { container } = render(<ApiKeysSection />, { wrapper: createWrapper() });
    expect(container).toBeEmptyDOMElement();
  });

  it("should render loading state", () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise(() => {}) // Never resolves - keeps loading
    );

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    expect(screen.getByText("API Keys")).toBeInTheDocument();
  });

  it("should render empty state when no keys", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ ...mockApiKeysResponse, keys: [] }),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("No API keys yet")).toBeInTheDocument();
    });
  });

  it("should render API keys table", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockApiKeysResponse),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Test Key 1")).toBeInTheDocument();
      expect(screen.getByText("Expired Key")).toBeInTheDocument();
    });

    expect(screen.getByText("omnia_***abc")).toBeInTheDocument();
    expect(screen.getByText("Expired")).toBeInTheDocument();
  });

  it("should show create button when user can manage keys", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        (p) =>
          p === Permission.API_KEYS_VIEW_OWN ||
          p === Permission.API_KEYS_MANAGE_OWN,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockApiKeysResponse),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument();
    });
  });

  it("should open create dialog when create button clicked", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockApiKeysResponse),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /Create Key/i }));

    expect(screen.getByText("Create API Key")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
  });

  it("should show file mode notice when keys are file-based", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          ...mockApiKeysResponse,
          config: { ...mockApiKeysResponse.config, storeType: "file", allowCreate: false },
        }),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Keys managed via Kubernetes Secret")).toBeInTheDocument();
    });
  });

  it("should show error state when fetch fails", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      json: () => Promise.resolve({ error: "Failed to fetch" }),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Failed to load API keys")).toBeInTheDocument();
    });
  });

  it("should open delete confirmation dialog when delete clicked", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockApiKeysResponse),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Test Key 1")).toBeInTheDocument();
    });

    // Find and click delete button
    const deleteButtons = screen.getAllByRole("button");
    const trashButton = deleteButtons.find((btn) =>
      btn.querySelector('svg.lucide-trash-2')
    );
    if (trashButton) {
      fireEvent.click(trashButton);
    }

    await waitFor(() => {
      expect(screen.getByText("Revoke API Key?")).toBeInTheDocument();
    });
  });

  it("should display key count", async () => {
    mockUsePermissions.mockReturnValue(
      createMockPermissions(
        () => true,
        [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN]
      )
    );

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockApiKeysResponse),
    });

    render(<ApiKeysSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("2 of 5 keys used")).toBeInTheDocument();
    });
  });
});
