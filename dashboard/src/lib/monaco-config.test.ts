import { describe, it, expect, vi } from "vitest";

const configMock = vi.fn();
vi.mock("@monaco-editor/react", () => ({
  loader: { config: configMock },
}));

describe("monaco-config", () => {
  it("points the Monaco loader at the self-hosted /monaco/vs path", async () => {
    // Importing the module runs its side effect (loader.config). Self-hosting
    // is what lets the editor load under the dashboard CSP (no jsdelivr CDN).
    await import("./monaco-config");
    expect(configMock).toHaveBeenCalledWith({ paths: { vs: "/monaco/vs" } });
  });
});
