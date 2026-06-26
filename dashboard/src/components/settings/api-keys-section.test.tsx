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

vi.mock("@/hooks/use-workspaces", () => ({
  useWorkspaces: vi.fn(() => ({
    data: [
      { name: "demo", displayName: "Demo", environment: "development", namespace: "demo", role: "editor", permissions: {} },
      { name: "prod", displayName: "Production", environment: "production", namespace: "prod", role: "viewer", permissions: {} },
    ],
    isLoading: false,
    error: null,
  })),
}));

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

describe("ApiKeysSection workspace scope (#1561 P2)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUsePermissions.mockReturnValue(
      createMockPermissions(() => true, [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN])
    );
  });

  function fetchMock() {
    return vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ key: { id: "k1", name: "ci", key: "omnia_sk_x", keyPrefix: "omnia_sk_x...", role: "editor", expiresAt: null, createdAt: new Date().toISOString(), lastUsedAt: null, isExpired: false } }) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(mockApiKeysResponse) });
    });
  }

  it("renders a workspace checkbox per accessible workspace in the create dialog", async () => {
    global.fetch = fetchMock() as never;
    render(<ApiKeysSection />, { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Create Key/i }));
    expect(screen.getByLabelText("Demo")).toBeInTheDocument();
    expect(screen.getByLabelText("Production")).toBeInTheDocument();
  });

  it("POSTs selected workspace names; omits workspaces when none selected", async () => {
    const fetchFn = fetchMock();
    global.fetch = fetchFn as never;
    render(<ApiKeysSection />, { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Create Key/i }));

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "ci" } });
    fireEvent.click(screen.getByLabelText("Demo")); // select one workspace
    // Click the dialog submit button (second "Create Key" button — card button + dialog submit)
    const createKeyButtons = screen.getAllByRole("button", { name: /Create Key/i });
    fireEvent.click(createKeyButtons[createKeyButtons.length - 1]);

    await waitFor(() => {
      const postCall = fetchFn.mock.calls.find((c) => (c[1] as RequestInit)?.method === "POST");
      expect(postCall).toBeTruthy();
      const body = JSON.parse((postCall![1] as RequestInit).body as string);
      expect(body.workspaces).toEqual(["demo"]);
    });
  });

  it("omits workspaces from the POST body when none are selected", async () => {
    const fetchFn = fetchMock();
    global.fetch = fetchFn as never;
    render(<ApiKeysSection />, { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Create Key/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "global" } });
    // Click the dialog submit button (last "Create Key" button)
    const createKeyButtons = screen.getAllByRole("button", { name: /Create Key/i });
    fireEvent.click(createKeyButtons[createKeyButtons.length - 1]);
    await waitFor(() => {
      const postCall = fetchFn.mock.calls.find((c) => (c[1] as RequestInit)?.method === "POST");
      const body = JSON.parse((postCall![1] as RequestInit).body as string);
      expect(body.workspaces).toBeUndefined();
    });
  });
});

describe("ApiKeysSection dialog actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUsePermissions.mockReturnValue(
      createMockPermissions(() => true, [Permission.API_KEYS_VIEW_OWN, Permission.API_KEYS_MANAGE_OWN])
    );
  });

  it("issues a DELETE when the user confirms revocation", async () => {
    const fetchFn = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(mockApiKeysResponse) });
    });
    global.fetch = fetchFn as never;

    render(<ApiKeysSection />, { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText("Test Key 1")).toBeInTheDocument());

    const trash = screen
      .getAllByRole("button")
      .find((b) => b.querySelector("svg.lucide-trash-2"));
    fireEvent.click(trash!);

    await waitFor(() => expect(screen.getByText("Revoke API Key?")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Revoke Key/i }));

    await waitFor(() => {
      const del = fetchFn.mock.calls.find((c) => (c[1] as RequestInit)?.method === "DELETE");
      expect(del).toBeTruthy();
      expect(del![0]).toContain("/api/settings/api-keys/key-1");
    });
  });

  it("copies the new key to the clipboard from the success dialog", async () => {
    const writeText = vi.fn();
    Object.assign(navigator, { clipboard: { writeText } });

    const newKey = {
      id: "k1",
      name: "ci",
      key: "omnia_sk_secret_value",
      keyPrefix: "omnia_sk_...",
      role: "editor",
      expiresAt: null,
      createdAt: new Date().toISOString(),
      lastUsedAt: null,
      isExpired: false,
    };
    const fetchFn = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ key: newKey }) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(mockApiKeysResponse) });
    });
    global.fetch = fetchFn as never;

    render(<ApiKeysSection />, { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Create Key/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "ci" } });
    const createButtons = screen.getAllByRole("button", { name: /Create Key/i });
    fireEvent.click(createButtons[createButtons.length - 1]);

    await waitFor(() => expect(screen.getByText("API Key Created")).toBeInTheDocument());

    const copyBtn = screen
      .getAllByRole("button")
      .find((b) => b.querySelector("svg.lucide-copy"));
    fireEvent.click(copyBtn!);

    expect(writeText).toHaveBeenCalledWith("omnia_sk_secret_value");
    await waitFor(() => expect(screen.getByText("Done")).toBeInTheDocument());

    // Closing the success dialog resets its state (onOpenChange close path).
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() => expect(screen.queryByText("API Key Created")).not.toBeInTheDocument());
  });

  it("surfaces an error message when key creation fails", async () => {
    const fetchFn = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return Promise.resolve({ ok: false, json: () => Promise.resolve({ error: "max keys reached" }) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(mockApiKeysResponse) });
    });
    global.fetch = fetchFn as never;

    render(<ApiKeysSection />, { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByRole("button", { name: /Create Key/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Create Key/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "overflow" } });
    const createButtons = screen.getAllByRole("button", { name: /Create Key/i });
    fireEvent.click(createButtons[createButtons.length - 1]);

    await waitFor(() => expect(screen.getByText("max keys reached")).toBeInTheDocument());
  });
});
