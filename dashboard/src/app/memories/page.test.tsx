import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import MemoriesPage from "./page";

// Mock layout components that pull in complex infrastructure (WorkspaceSwitcher, UserMenu)
vi.mock("@/components/layout", () => ({
  Header: ({
    title,
    description,
  }: {
    title: string;
    description?: string;
  }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  ),
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { id: "test-user" }, isAuthenticated: true }),
}));
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    workspaces: [],
    currentWorkspace: { name: "test-ws" },
    setCurrentWorkspace: vi.fn(),
    isLoading: false,
    error: null,
  }),
}));
vi.mock("@/hooks/use-memories", () => ({
  useMemories: () => ({ data: { memories: [], total: 0 }, isLoading: false }),
}));
vi.mock("@/hooks/use-memory-mutations", () => ({
  useDeleteMemory: () => ({ mutate: vi.fn() }),
  useDeleteAllMemories: () => ({ mutate: vi.fn() }),
  useExportMemories: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("@/hooks/use-consent", () => ({
  useConsent: () => ({
    data: { grants: [], defaults: [], denied: [] },
    isLoading: false,
  }),
  useUpdateConsent: () => ({ mutate: vi.fn(), isPending: false }),
}));

describe("MemoriesPage", () => {
  it("renders empty state when no memories", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("empty-state")).toBeTruthy();
  });

  it("renders toolbar", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("memories-toolbar")).toBeTruthy();
  });

  it("renders consent banner", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("consent-banner")).toBeTruthy();
  });
});
