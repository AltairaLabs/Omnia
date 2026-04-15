/**
 * Tests for SkillSourceExplorer wrapper. The shared ContentExplorer is
 * exercised by content-explorer.test.tsx; here we just verify the wrapper
 * passes the right URL/builders and finds SKILL.md anywhere in the tree.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { SkillSourceExplorer } from "./skill-source-explorer";
import type { ArenaSourceContentNode } from "@/types/arena";

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({ currentWorkspace: { name: "test-ws" } })),
}));

const mockContent: {
  tree: ArenaSourceContentNode[];
  fileCount: number;
  directoryCount: number;
  loading: boolean;
  error: Error | null;
  refetch: ReturnType<typeof vi.fn>;
} = {
  tree: [],
  fileCount: 0,
  directoryCount: 0,
  loading: false,
  error: null,
  refetch: vi.fn(),
};
vi.mock("@/hooks/use-skill-source-content", () => ({
  useSkillSourceContent: vi.fn(() => mockContent),
}));

vi.mock("@/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizablePanel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizableHandle: () => <div />,
}));

vi.mock("@/components/arena", () => ({
  YamlEditor: ({ value }: { value: string }) => <div data-testid="yaml-editor">{value}</div>,
  YamlEditorEmptyState: () => <div data-testid="yaml-editor-empty" />,
}));

vi.mock("@/components/ui/markdown", () => ({
  Markdown: ({ content }: { content: string }) => <div data-testid="markdown">{content}</div>,
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("SkillSourceExplorer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockContent.tree = [];
    mockContent.fileCount = 0;
    mockContent.loading = false;
    mockContent.error = null;
  });

  afterEach(() => vi.resetAllMocks());

  it("renders the empty state when no content has been synced", () => {
    render(<SkillSourceExplorer sourceName="skills-git" />);
    expect(screen.getByText("No content available")).toBeInTheDocument();
  });

  it("auto-loads the first SKILL.md found anywhere in the tree", async () => {
    mockContent.tree = [
      {
        name: "skills",
        path: "skills",
        isDirectory: true,
        children: [
          {
            name: "first",
            path: "skills/first",
            isDirectory: true,
            children: [
              {
                name: "SKILL.md",
                path: "skills/first/SKILL.md",
                isDirectory: false,
                size: 10,
              },
            ],
          },
        ],
      },
    ];
    mockContent.fileCount = 1;

    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        path: "skills/first/SKILL.md",
        content: "# First skill",
        size: 13,
      }),
    });

    render(<SkillSourceExplorer sourceName="skills-git" />);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/skills/skills-git/file?path=skills%2Ffirst%2FSKILL.md"
      );
    });
    await waitFor(() => {
      expect(screen.getByTestId("markdown")).toHaveTextContent("# First skill");
    });
  });

  it("surfaces fetch errors from loadFile in the file viewer", async () => {
    mockContent.tree = [
      { name: "SKILL.md", path: "SKILL.md", isDirectory: false, size: 1 },
    ];
    mockContent.fileCount = 1;
    mockFetch.mockResolvedValue({
      ok: false,
      statusText: "Forbidden",
      json: async () => ({ error: "Nope" }),
    });

    render(<SkillSourceExplorer sourceName="skills-git" />);

    await waitFor(() => {
      expect(screen.getByText("Nope")).toBeInTheDocument();
    });
  });
});
