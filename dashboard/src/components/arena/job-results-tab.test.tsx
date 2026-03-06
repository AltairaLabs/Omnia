import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { JobResultsTab } from "./job-results-tab";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-ws" },
  }),
}));

// Mock results panel store
let mockJobName: string | null = "test-job";
vi.mock("@/stores/results-panel-store", () => ({
  useResultsPanelStore: vi.fn((selector) => {
    const state = {
      currentJobName: mockJobName,
    };
    return selector(state);
  }),
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

function makeResults(count: number) {
  return Array.from({ length: count }, (_, i) => ({
    scenario: `scenario-${i}`,
    status: i % 2 === 0 ? "pass" : "fail",
    score: i % 2 === 0 ? 0.9 : 0.3,
    durationMs: 100 + i,
  }));
}

describe("JobResultsTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockJobName = "test-job";
  });

  it("shows 'No job selected' when no job is active", () => {
    mockJobName = null;
    render(<JobResultsTab />);
    expect(screen.getByText("No job selected")).toBeInTheDocument();
  });

  it("renders results table when data is available", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          type: "evaluation",
          results: makeResults(5),
        }),
    });

    render(<JobResultsTab />);

    await waitFor(() => {
      expect(screen.getByText("scenario-0")).toBeInTheDocument();
    });

    // All 5 should be visible (under INITIAL_RESULTS_WINDOW of 100)
    expect(screen.getByText("scenario-4")).toBeInTheDocument();
    // No "Show more" button since count < 100
    expect(screen.queryByText(/Show more results/)).not.toBeInTheDocument();
  });

  it("shows 'Show more results' button when results exceed window", async () => {
    const results = makeResults(150);
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          type: "evaluation",
          results,
        }),
    });

    render(<JobResultsTab />);

    await waitFor(() => {
      expect(screen.getByText("scenario-0")).toBeInTheDocument();
    });

    // First 100 visible, remaining 50
    expect(screen.getByText(/Show more results \(50 remaining\)/)).toBeInTheDocument();
    // scenario-99 should be visible (index 99, within first 100)
    expect(screen.getByText("scenario-99")).toBeInTheDocument();
    // scenario-100 should NOT be visible (beyond window)
    expect(screen.queryByText("scenario-100")).not.toBeInTheDocument();
  });

  it("loads more results when 'Show more' is clicked", async () => {
    const user = userEvent.setup();
    const results = makeResults(150);
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          type: "evaluation",
          results,
        }),
    });

    render(<JobResultsTab />);

    await waitFor(() => {
      expect(screen.getByText(/Show more results/)).toBeInTheDocument();
    });

    await user.click(screen.getByText(/Show more results/));

    // Now all 150 should be visible
    expect(screen.getByText("scenario-149")).toBeInTheDocument();
    // No more "Show more" button
    expect(screen.queryByText(/Show more results/)).not.toBeInTheDocument();
  });

  it("displays loading spinner while fetching", () => {
    mockFetch.mockReturnValueOnce(new Promise(() => {})); // never resolves
    render(<JobResultsTab />);
    // The spinner has animate-spin class
    expect(document.querySelector(".animate-spin")).toBeInTheDocument();
  });

  it("displays error message on fetch failure", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });

    render(<JobResultsTab />);

    await waitFor(() => {
      expect(
        screen.getByText("Failed to fetch results: Internal Server Error")
      ).toBeInTheDocument();
    });
  });
});
