import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TemplateCreateFlow } from "./template-create-flow";

const renderMock = vi.fn();
const previewMock = vi.fn();

vi.mock("@/hooks/arena", () => ({
  useTemplateSources: () => ({ sources: [{ metadata: { name: "src" } }], loading: false, error: null, refetch: vi.fn() }),
  useAllTemplates: () => ({ templates: [{ name: "starter", sourceName: "src" }], loading: false, error: null, refetch: vi.fn() }),
  useTemplateRendering: () => ({ preview: previewMock, render: renderMock }),
}));

vi.mock("@/hooks/resources", () => ({
  useProviders: () => ({ data: [{ metadata: { name: "claude" }, spec: { model: "claude-opus-4-8" } }] }),
}));

// Replace the heavy child components with light stubs that exercise the flow.
vi.mock("./template-browser", () => ({
  TemplateBrowser: ({ onSelectTemplate }: { onSelectTemplate?: (t: unknown, s: string) => void }) => (
    <button type="button" onClick={() => onSelectTemplate?.({ name: "starter" }, "src")}>
      pick-starter
    </button>
  ),
}));
vi.mock("./template-wizard", () => ({
  TemplateWizard: ({
    onSuccess,
    onClose,
    onPreview,
    onSubmit,
    template,
  }: {
    onSuccess?: (id: string) => void;
    onClose?: () => void;
    onPreview?: (input: { variables: Record<string, string> }) => void;
    onSubmit?: (input: { variables: Record<string, string>; projectName: string }) => void;
    template: { name: string };
  }) => (
    <div>
      <span>wizard:{template.name}</span>
      <button type="button" onClick={() => onPreview?.({ variables: { a: "b" } })}>preview</button>
      <button type="button" onClick={() => onSubmit?.({ variables: { a: "b" }, projectName: "p" })}>submit</button>
      <button type="button" onClick={() => onSuccess?.("proj-1")}>finish</button>
      <button type="button" onClick={() => onClose?.()}>back</button>
    </div>
  ),
}));

describe("TemplateCreateFlow", () => {
  beforeEach(() => {
    renderMock.mockReset();
    previewMock.mockReset();
  });

  it("shows the browser first, then the wizard once a template is picked", () => {
    render(<TemplateCreateFlow onSuccess={vi.fn()} />);
    expect(screen.getByText("pick-starter")).toBeInTheDocument();
    fireEvent.click(screen.getByText("pick-starter"));
    expect(screen.getByText("wizard:starter")).toBeInTheDocument();
  });

  it("returns to the browser when the wizard is closed", () => {
    render(<TemplateCreateFlow onSuccess={vi.fn()} />);
    fireEvent.click(screen.getByText("pick-starter"));
    fireEvent.click(screen.getByText("back"));
    expect(screen.getByText("pick-starter")).toBeInTheDocument();
  });

  it("forwards the created project id to onSuccess", () => {
    const onSuccess = vi.fn();
    render(<TemplateCreateFlow onSuccess={onSuccess} />);
    fireEvent.click(screen.getByText("pick-starter"));
    fireEvent.click(screen.getByText("finish"));
    expect(onSuccess).toHaveBeenCalledWith("proj-1");
  });

  it("routes preview and submit to the selected template's source + name", () => {
    previewMock.mockResolvedValue({ files: [] });
    renderMock.mockResolvedValue({ projectId: "proj-1" });
    render(<TemplateCreateFlow onSuccess={vi.fn()} />);
    fireEvent.click(screen.getByText("pick-starter"));

    fireEvent.click(screen.getByText("preview"));
    expect(previewMock).toHaveBeenCalledWith("src", "starter", { variables: { a: "b" } });

    fireEvent.click(screen.getByText("submit"));
    expect(renderMock).toHaveBeenCalledWith("src", "starter", { variables: { a: "b" }, projectName: "p" });
  });
});
