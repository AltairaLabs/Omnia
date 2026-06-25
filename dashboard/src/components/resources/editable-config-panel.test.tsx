import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { EditableConfigPanel } from "./editable-config-panel";
import { ResourceUpdateError } from "@/hooks/use-tool-registry-mutations";

const can = vi.fn();
vi.mock("@/hooks/use-permissions", () => ({
  usePermissions: () => ({ can }),
  Permission: { TOOLS_EDIT: "tools:edit" },
}));

const readOnly = { isReadOnly: false, message: "GitOps read-only" };
vi.mock("@/hooks/use-read-only", () => ({ useReadOnly: () => readOnly }));

vi.mock("@/hooks/use-toast", () => ({ toast: vi.fn() }));

// Drive the editor through a textarea so tests can edit the YAML.
vi.mock("@/components/arena/yaml-editor", () => ({
  YamlEditor: ({ value, onChange }: { value: string; onChange?: (v: string) => void }) => (
    <textarea aria-label="yaml" value={value} onChange={(e) => onChange?.(e.target.value)} />
  ),
}));

const RESOURCE = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ToolRegistry",
  metadata: { name: "gh", namespace: "ws-ns", resourceVersion: "42" },
  spec: { handlers: [] },
};

function renderPanel(onSave = vi.fn().mockResolvedValue(RESOURCE)) {
  return render(
    <EditableConfigPanel
      kind="ToolRegistry"
      name="gh"
      resource={RESOURCE}
      editPermission={"tools:edit" as never}
      onSave={onSave}
    />
  );
}

describe("EditableConfigPanel", () => {
  beforeEach(() => {
    can.mockReset();
    readOnly.isReadOnly = false;
  });

  it("renders read-only (no editor, no Save) when the user lacks the permission", () => {
    can.mockReturnValue(false);
    renderPanel();
    expect(screen.queryByLabelText("yaml")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /save/i })).not.toBeInTheDocument();
  });

  it("renders read-only when global read-only mode is on, even for editors", () => {
    can.mockReturnValue(true);
    readOnly.isReadOnly = true;
    renderPanel();
    expect(screen.queryByLabelText("yaml")).not.toBeInTheDocument();
    expect(screen.getByText(/GitOps read-only/i)).toBeInTheDocument();
  });

  it("renders the editor with Save disabled until a valid edit is made", () => {
    can.mockReturnValue(true);
    renderPanel();
    const save = screen.getByRole("button", { name: /save/i });
    expect(save).toBeDisabled();

    fireEvent.change(screen.getByLabelText("yaml"), {
      target: { value: "spec:\n  handlers:\n    - name: h\n" },
    });
    expect(screen.getByRole("button", { name: /save/i })).toBeEnabled();
  });

  it("disables Save when the edited YAML is invalid", () => {
    can.mockReturnValue(true);
    renderPanel();
    fireEvent.change(screen.getByLabelText("yaml"), { target: { value: "spec:\n  - : :\nbad" } });
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
  });

  it("opens a confirm dialog and calls onSave with resourceVersion on confirm", async () => {
    can.mockReturnValue(true);
    const onSave = vi.fn().mockResolvedValue(RESOURCE);
    renderPanel(onSave);

    fireEvent.change(screen.getByLabelText("yaml"), {
      target: { value: "spec:\n  handlers:\n    - name: h\n" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    fireEvent.click(await screen.findByRole("button", { name: /apply/i }));

    await waitFor(() => expect(onSave).toHaveBeenCalled());
    const body = onSave.mock.calls[0][0];
    expect(body.metadata.resourceVersion).toBe("42");
    expect(body.spec.handlers[0].name).toBe("h");
  });

  it("shows the conflict message on a 409 ResourceUpdateError", async () => {
    can.mockReturnValue(true);
    const onSave = vi.fn().mockRejectedValue(new ResourceUpdateError(409, "modified"));
    renderPanel(onSave);

    fireEvent.change(screen.getByLabelText("yaml"), {
      target: { value: "spec:\n  handlers: []\n" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    fireEvent.click(await screen.findByRole("button", { name: /apply/i }));

    expect(await screen.findByText(/changed since you loaded it/i)).toBeInTheDocument();
  });

  it("shows the validation message on a 422 ResourceUpdateError", async () => {
    can.mockReturnValue(true);
    const onSave = vi
      .fn()
      .mockRejectedValue(new ResourceUpdateError(422, "spec.handlers[0].type: Unsupported value"));
    renderPanel(onSave);

    fireEvent.change(screen.getByLabelText("yaml"), {
      target: { value: "spec:\n  handlers: []\n" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    fireEvent.click(await screen.findByRole("button", { name: /apply/i }));

    expect(await screen.findByText(/Unsupported value/i)).toBeInTheDocument();
  });
});
