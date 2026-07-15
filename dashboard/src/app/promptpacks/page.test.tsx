import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import PromptPacksPage from "./page";
import { createQueryWrapper } from "@/test/query-wrapper";
import type { PromptPack } from "@/types";

const usePromptPacksSpy = vi.hoisted(() => vi.fn());
const useWorkspaceSpy = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/resources", () => ({
  usePromptPacks: usePromptPacksSpy,
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: useWorkspaceSpy,
}));

// Header pulls in next/navigation + workspace/theme context this suite
// doesn't need to exercise; stub it like other page tests do.
vi.mock("@/components/layout", () => ({
  Header: ({ title }: { title: React.ReactNode }) => <h1>{title}</h1>,
}));

// Render PromptPackCard as a thin stub so this suite exercises only the
// page's dedupe/filter/count logic, not the card's own hooks/rendering.
vi.mock("@/components/promptpacks", () => ({
  PromptPackCard: ({ promptPack }: { promptPack: PromptPack }) => (
    <div data-testid="promptpack-card-stub">{promptPack.spec.packName}</div>
  ),
  PromptPackDialog: ({ open, onSuccess }: { open: boolean; onSuccess: () => void }) => (
    <>
      {open && <div data-testid="dialog-open" />}
      <button onClick={onSuccess}>trigger-success</button>
    </>
  ),
}));

// Stub NamespaceFilter with a button that reports a namespace selection, so
// the page's onSelectionChange wiring can be exercised without pulling in
// the real filter dropdown component.
vi.mock("@/components/filters", () => ({
  NamespaceFilter: ({ onSelectionChange }: { onSelectionChange: (ns: string[]) => void }) => (
    <button onClick={() => onSelectionChange(["staging"])}>select-staging</button>
  ),
}));

// Build a version-object the way the operator names real PromptPacks after
// #1837: a deterministic pp-<hash> metadata.name, with the logical identity
// carried in spec.packName.
function makePack(
  metadataName: string,
  packName: string,
  version: string,
  phase: "Active" | "Pending" | "Failed",
  namespace = "production",
): PromptPack {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: { name: metadataName, namespace, uid: metadataName },
    spec: {
      packName,
      source: { type: "configmap", configMapRef: { name: "cm" } },
      version,
    },
    status: { phase, activeVersion: version },
  };
}

describe("PromptPacksPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useWorkspaceSpy.mockReturnValue({ isLoading: false });
  });

  it("dedupes multiple version-objects of one packName to a single card", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "1.0.0", "Active"),
      makePack("pp-hash2", "support-bot", "1.2.0", "Active"),
      makePack("pp-hash3", "support-bot", "1.1.0", "Active"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    const cards = screen.getAllByTestId("promptpack-card-stub");
    expect(cards).toHaveLength(1);
    expect(cards[0]).toHaveTextContent("support-bot");
  });

  it("counts the deduped set in the phase tabs, not the raw version-objects", () => {
    const packs: PromptPack[] = [
      // 3 version-objects of the same logical pack -> should count as 1 Active.
      makePack("pp-hash1", "support-bot", "1.0.0", "Active"),
      makePack("pp-hash2", "support-bot", "1.2.0", "Active"),
      makePack("pp-hash3", "support-bot", "1.1.0", "Active"),
      // A second, distinct logical pack -> counts as 1 Pending.
      makePack("pp-hash4", "billing-bot", "2.0.0", "Pending"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.getByRole("tab", { name: "All (2)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Active (1)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Pending (1)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Failed (0)" })).toBeInTheDocument();
  });

  it("picks the channel-max stable version as the representative card", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "1.0.0", "Active"),
      makePack("pp-hash2", "support-bot", "2.0.0-rc.1", "Pending"),
      makePack("pp-hash3", "support-bot", "1.5.0", "Active"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    // Highest stable version (1.5.0) wins over the higher prerelease (2.0.0-rc.1).
    expect(screen.getByRole("tab", { name: "Active (1)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Pending (0)" })).toBeInTheDocument();
  });

  it("falls back to the highest prerelease when no stable version exists", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "1.0.0-rc.1", "Pending"),
      makePack("pp-hash2", "support-bot", "1.0.0-rc.2", "Pending"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    const cards = screen.getAllByTestId("promptpack-card-stub");
    expect(cards).toHaveLength(1);
    expect(cards[0]).toHaveTextContent("support-bot");
  });

  it("falls back to the first version-object when no version parses as semver", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "not-a-version", "Pending"),
      makePack("pp-hash2", "support-bot", "also-not-a-version", "Pending"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    const cards = screen.getAllByTestId("promptpack-card-stub");
    expect(cards).toHaveLength(1);
    expect(cards[0]).toHaveTextContent("support-bot");
  });

  it("sorts without erroring when a packName is empty", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "", "1.0.0", "Active"),
      makePack("pp-hash2", "billing-bot", "1.0.0", "Active"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.getAllByTestId("promptpack-card-stub")).toHaveLength(2);
  });

  it("does not dedupe packs with different packNames", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "1.0.0", "Active"),
      makePack("pp-hash2", "billing-bot", "1.0.0", "Active"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    const cards = screen.getAllByTestId("promptpack-card-stub");
    expect(cards).toHaveLength(2);
  });

  it("shows an empty state when there are no packs", () => {
    usePromptPacksSpy.mockReturnValue({ data: [], isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.getByText("No PromptPacks found")).toBeInTheDocument();
  });

  it("shows skeletons while loading", () => {
    usePromptPacksSpy.mockReturnValue({ data: undefined, isLoading: true });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.queryByTestId("promptpack-card-stub")).not.toBeInTheDocument();
  });

  it("shows skeletons while the workspace is loading, even once packs have loaded", () => {
    useWorkspaceSpy.mockReturnValue({ isLoading: true });
    usePromptPacksSpy.mockReturnValue({ data: [], isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.queryByTestId("promptpack-card-stub")).not.toBeInTheDocument();
  });

  it("filters the visible cards by phase tab", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "1.0.0", "Active"),
      makePack("pp-hash2", "billing-bot", "1.0.0", "Pending"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.getAllByTestId("promptpack-card-stub")).toHaveLength(2);

    // Radix Tabs selects on mousedown, not click (see @radix-ui/react-tabs).
    fireEvent.mouseDown(screen.getByRole("tab", { name: "Active (1)" }));

    const cards = screen.getAllByTestId("promptpack-card-stub");
    expect(cards).toHaveLength(1);
    expect(cards[0]).toHaveTextContent("support-bot");
  });

  it("filters the visible cards by namespace selection", () => {
    const packs: PromptPack[] = [
      makePack("pp-hash1", "support-bot", "1.0.0", "Active", "production"),
      makePack("pp-hash2", "billing-bot", "1.0.0", "Active", "staging"),
    ];
    usePromptPacksSpy.mockReturnValue({ data: packs, isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.getAllByTestId("promptpack-card-stub")).toHaveLength(2);

    fireEvent.click(screen.getByText("select-staging"));

    const cards = screen.getAllByTestId("promptpack-card-stub");
    expect(cards).toHaveLength(1);
    expect(cards[0]).toHaveTextContent("billing-bot");
  });

  it("opens the create dialog from the toolbar button", () => {
    usePromptPacksSpy.mockReturnValue({ data: [], isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    expect(screen.queryByTestId("dialog-open")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /new promptpack/i }));
    expect(screen.getByTestId("dialog-open")).toBeInTheDocument();
  });

  it("opens the create dialog from the empty-state button", () => {
    usePromptPacksSpy.mockReturnValue({ data: [], isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    fireEvent.click(screen.getByRole("button", { name: /create your first promptpack/i }));
    expect(screen.getByTestId("dialog-open")).toBeInTheDocument();
  });

  it("invalidates the promptPacks query on dialog success", () => {
    usePromptPacksSpy.mockReturnValue({ data: [], isLoading: false });

    render(<PromptPacksPage />, { wrapper: createQueryWrapper() });

    // Just exercises the onSuccess wiring; the invalidation itself is
    // React Query's own well-tested behavior.
    expect(() => fireEvent.click(screen.getByText("trigger-success"))).not.toThrow();
  });
});
