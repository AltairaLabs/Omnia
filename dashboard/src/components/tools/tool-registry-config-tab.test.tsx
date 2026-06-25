import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ToolRegistryConfigTab } from "./tool-registry-config-tab";

const updateToolRegistry = vi.fn();
vi.mock("@/hooks/use-tool-registry-mutations", () => ({
  useToolRegistryMutations: () => ({ updateToolRegistry }),
  ResourceUpdateError: class extends Error {},
}));

const invalidateQueries = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries }),
}));

// Capture the props EditableConfigPanel is mounted with.
const panelProps: Record<string, unknown> = {};
vi.mock("@/components/resources/editable-config-panel", () => ({
  EditableConfigPanel: (props: Record<string, unknown>) => {
    Object.assign(panelProps, props);
    return <div data-testid="panel" />;
  },
}));

vi.mock("@/hooks/use-permissions", () => ({ Permission: { TOOLS_EDIT: "tools:edit" } }));

describe("ToolRegistryConfigTab", () => {
  it("mounts EditableConfigPanel with ToolRegistry kind, name and edit permission", () => {
    const registry = { metadata: { name: "gh" }, spec: { handlers: [] } } as never;
    render(<ToolRegistryConfigTab registry={registry} />);
    expect(screen.getByTestId("panel")).toBeInTheDocument();
    expect(panelProps.kind).toBe("ToolRegistry");
    expect(panelProps.name).toBe("gh");
    expect(panelProps.editPermission).toBe("tools:edit");
  });

  it("falls back to an empty name when metadata.name is missing", () => {
    const registry = { metadata: {}, spec: { handlers: [] } } as never;
    render(<ToolRegistryConfigTab registry={registry} />);
    expect(panelProps.name).toBe("");
  });

  it("onSave calls updateToolRegistry then invalidates the toolRegistry query", async () => {
    const registry = { metadata: { name: "gh" }, spec: { handlers: [] } } as never;
    updateToolRegistry.mockResolvedValue({ metadata: { name: "gh" }, spec: {} });
    render(<ToolRegistryConfigTab registry={registry} />);

    const onSave = panelProps.onSave as (b: unknown) => Promise<unknown>;
    await onSave({ metadata: { resourceVersion: "42" }, spec: {} });

    expect(updateToolRegistry).toHaveBeenCalledWith("gh", {
      metadata: { resourceVersion: "42" },
      spec: {},
    });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["toolRegistry"] });
  });
});
