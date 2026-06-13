import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useArenaWorkloadModel } from "./use-arena-workload-model";

const CONFIG = `
kind: Arena
spec:
  prompt_configs:
    - id: assistant
      file: prompts/assistant.yaml
  providers:
    - file: providers/gpt.provider.yaml
  scenarios: []
`;
const PROMPT = `
kind: PromptConfig
metadata: { name: assistant }
spec: { system_template: "Hi", variables: [] }
`;
const PROVIDER = `kind: Provider
metadata: { name: gpt }
spec: { id: gpt, type: openai, model: gpt-4o }`;

let openFiles: Array<{ path: string; content: string }> = [];
const fileTree = [
  { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false },
  { name: "prompts", path: "prompts", isDirectory: true, children: [
    { name: "assistant.yaml", path: "prompts/assistant.yaml", isDirectory: false },
  ]},
  { name: "providers", path: "providers", isDirectory: true, children: [
    { name: "gpt.provider.yaml", path: "providers/gpt.provider.yaml", isDirectory: false },
  ]},
];

vi.mock("@/stores", () => ({
  useProjectEditorStore: (selector: (s: unknown) => unknown) => selector({ openFiles, fileTree }),
}));

const getFileContent = vi.fn();
vi.mock("@/hooks/arena", () => ({
  useArenaProjectFiles: () => ({ getFileContent }),
}));

beforeEach(() => {
  openFiles = [];
  getFileContent.mockReset();
  getFileContent.mockImplementation((_id: string, path: string) => {
    const map: Record<string, string> = {
      "config.arena.yaml": CONFIG,
      "prompts/assistant.yaml": PROMPT,
      "providers/gpt.provider.yaml": PROVIDER,
    };
    if (map[path]) return Promise.resolve({ content: map[path], path, size: 0, modifiedAt: "", encoding: "utf-8" });
    return Promise.reject(new Error("File not found"));
  });
});

describe("useArenaWorkloadModel", () => {
  it("fetches saved files and builds the model when nothing is open", async () => {
    const { result } = renderHook(() => useArenaWorkloadModel("proj-1"));
    await waitFor(() => expect(result.current.model).not.toBeNull());
    expect(result.current.model!.altitude).toBe("test");
    expect(result.current.model!.nodes.find((n) => n.id === "provider:gpt")).toBeTruthy();
    expect(result.current.parseError).toBeNull();
  });

  it("prefers a live open buffer over the saved file", async () => {
    openFiles = [{ path: "providers/gpt.provider.yaml", content: "kind: Provider\nmetadata: { name: live }\nspec: { id: live, model: claude }" }];
    const { result } = renderHook(() => useArenaWorkloadModel("proj-1"));
    await waitFor(() => expect(result.current.model).not.toBeNull());
    expect(result.current.model!.nodes.find((n) => n.id === "provider:live")).toBeTruthy();
  });

  it("keeps the last-good model and sets parseError when the config is mid-edit", async () => {
    const { result, rerender } = renderHook(() => useArenaWorkloadModel("proj-1"));
    await waitFor(() => expect(result.current.model).not.toBeNull());
    const good = result.current.model;
    openFiles = [{ path: "config.arena.yaml", content: "{{ broken" }];
    rerender();
    await waitFor(() => expect(result.current.parseError).not.toBeNull());
    expect(result.current.model).toBe(good);
  });
});
