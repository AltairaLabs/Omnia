import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

import type { User } from "@/lib/auth/types";
import type { ContentNode } from "./content-api-service";

vi.mock("./content-api-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./content-api-service")>();
  return { ...actual, getContent: vi.fn() };
});

const user = { id: "u", username: "u", groups: [], role: "viewer", provider: "oauth" } as unknown as User;

// A small fake tree:  root/  a.txt  dir/  dir/b.txt  dir/.hidden
const TREE: Record<string, ContentNode> = {
  "": {
    path: "",
    entries: [
      { name: "a.txt", type: "file", size: 3, modifiedAt: "t" },
      { name: "dir", type: "directory", size: 0, modifiedAt: "t" },
    ],
  },
  dir: {
    path: "dir",
    entries: [
      { name: "b.txt", type: "file", size: 5, modifiedAt: "t" },
      { name: ".hidden", type: "directory", size: 0, modifiedAt: "t" },
    ],
  },
  "dir/.hidden": { path: "dir/.hidden", entries: [] },
};

describe("listContentTree", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("builds a recursive tree from flat listings", async () => {
    const svc = await import("./content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => TREE[relpath]);

    const { listContentTree } = await import("./content-tree");
    const tree = await listContentTree("ws", user, "");

    expect(tree.map((n) => n.name)).toEqual(["a.txt", "dir"]);
    const file = tree.find((n) => n.name === "a.txt");
    expect(file).toMatchObject({ isDirectory: false, size: 3, path: "a.txt" });
    const dir = tree.find((n) => n.name === "dir");
    expect(dir?.isDirectory).toBe(true);
    expect(dir?.children?.map((c) => c.name)).toEqual(["b.txt", ".hidden"]);
    expect(dir?.children?.find((c) => c.name === "b.txt")?.path).toBe("dir/b.txt");
  });

  it("skips hidden entries when skipHidden is set", async () => {
    const svc = await import("./content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => TREE[relpath]);

    const { listContentTree } = await import("./content-tree");
    const tree = await listContentTree("ws", user, "", { skipHidden: true });

    const dir = tree.find((n) => n.name === "dir");
    expect(dir?.children?.map((c) => c.name)).toEqual(["b.txt"]);
  });

  it("returns an empty array when the path is a file, not a directory", async () => {
    const svc = await import("./content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "a.txt",
      content: "abc",
      encoding: "utf-8",
      size: 3,
      modifiedAt: "t",
    });

    const { listContentTree } = await import("./content-tree");
    expect(await listContentTree("ws", user, "a.txt")).toEqual([]);
  });
});
