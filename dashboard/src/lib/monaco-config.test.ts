import { describe, it, expect, vi } from "vitest";

const configMock = vi.fn();
vi.mock("@monaco-editor/react", () => ({
  loader: { config: configMock },
}));

// monaco-editor is aliased to @codingame/monaco-vscode-editor-api, which touches
// the DOM on load; stub it so the client-only dynamic import resolves in jsdom.
const monacoStub = { editor: {} };
vi.mock("monaco-editor", () => monacoStub);

describe("monaco-config", () => {
  it("hands @monaco-editor/react the bundled Monaco instance (shared with the LSP client)", async () => {
    // Importing the module runs its side effect: a client-only dynamic import of
    // monaco-editor, then loader.config({ monaco }). The dynamic import resolves
    // asynchronously, so wait for the call.
    await import("./monaco-config");
    await vi.waitFor(() =>
      expect(configMock).toHaveBeenCalledWith({ monaco: monacoStub }),
    );
  });
});
