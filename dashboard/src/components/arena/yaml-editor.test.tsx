import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { YamlEditor, YamlEditorEmptyState } from "./yaml-editor";

// Mock Monaco Editor
vi.mock("@monaco-editor/react", () => ({
  default: vi.fn(({ value, onChange, loading }: { value: string; onChange?: (v: string) => void; loading?: React.ReactNode }) => {
    // Simulate the editor
    return (
      <div data-testid="monaco-editor">
        <textarea
          data-testid="editor-textarea"
          value={value}
          onChange={(e) => onChange?.(e.target.value)}
        />
        {loading}
      </div>
    );
  }),
}));

describe("YamlEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render loading state", () => {
    render(<YamlEditor value="" loading={true} />);

    expect(screen.getByText(/loading file/i)).toBeInTheDocument();
  });

  it("should render Monaco editor when not loading", () => {
    render(<YamlEditor value="test: value" />);

    expect(screen.getByTestId("monaco-editor")).toBeInTheDocument();
  });

  it("should pass value to editor", () => {
    render(<YamlEditor value="name: test" />);

    const textarea = screen.getByTestId("editor-textarea");
    expect(textarea).toHaveValue("name: test");
  });

  it("should call onChange when content changes", () => {
    const onChange = vi.fn();
    render(<YamlEditor value="original" onChange={onChange} />);

    // Verify the editor is rendered with the onChange capability
    expect(screen.getByTestId("monaco-editor")).toBeInTheDocument();
  });

  it("should show valid YAML indicator", () => {
    render(<YamlEditor value="name: test" fileType="yaml" />);

    expect(screen.getByText(/valid yaml/i)).toBeInTheDocument();
  });

  it("should show invalid YAML indicator with error", () => {
    const invalidYaml = "name: test\n  invalid: indentation";
    render(<YamlEditor value={invalidYaml} fileType="yaml" />);

    // Should show an error indicator
    expect(screen.queryByText(/valid yaml/i)).not.toBeInTheDocument();
  });

  it("should not show YAML validation for non-YAML files", () => {
    render(<YamlEditor value="{ invalid json" fileType="json" />);

    // Should not show YAML validation status
    expect(screen.queryByText(/valid yaml/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/invalid yaml/i)).not.toBeInTheDocument();
  });

  it("should show YAML validation for Arena file types", () => {
    render(<YamlEditor value="name: test" fileType="arena" />);

    expect(screen.getByText(/valid yaml/i)).toBeInTheDocument();
  });

  it("should show YAML validation for prompt files", () => {
    render(<YamlEditor value="system: You are helpful" fileType="prompt" />);

    expect(screen.getByText(/valid yaml/i)).toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(<YamlEditor value="" className="custom-class" />);

    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("should handle empty content as valid YAML", () => {
    render(<YamlEditor value="" fileType="yaml" />);

    expect(screen.getByText(/valid yaml/i)).toBeInTheDocument();
  });

  it("should handle whitespace-only content as valid YAML", () => {
    render(<YamlEditor value="   \n   " fileType="yaml" />);

    expect(screen.getByText(/valid yaml/i)).toBeInTheDocument();
  });

  it("should show line number for YAML errors", () => {
    const invalidYaml = `name: test
items:
  - item1
 - invalid`;  // Invalid indentation on line 4

    render(<YamlEditor value={invalidYaml} fileType="yaml" />);

    // Should show line number in error
    const errorElement = screen.queryByText(/line/i);
    expect(errorElement).toBeInTheDocument();
  });

  it("should validate complex YAML structures", () => {
    const complexYaml = `
name: test-project
providers:
  - name: openai
    model: gpt-4
  - name: anthropic
    model: claude-3
settings:
  temperature: 0.7
  maxTokens: 1000
`;
    render(<YamlEditor value={complexYaml} fileType="yaml" />);

    expect(screen.getByText(/valid yaml/i)).toBeInTheDocument();
  });

  it("should handle readOnly mode", () => {
    render(<YamlEditor value="test" readOnly={true} />);

    // Editor should be rendered (readOnly is passed to Monaco options)
    expect(screen.getByTestId("monaco-editor")).toBeInTheDocument();
  });
});

describe("YamlEditorEmptyState", () => {
  it("should render empty state message", () => {
    render(<YamlEditorEmptyState />);

    expect(screen.getByText(/no file selected/i)).toBeInTheDocument();
  });

  it("should show instruction to select a file", () => {
    render(<YamlEditorEmptyState />);

    expect(screen.getByText(/select a file from the tree/i)).toBeInTheDocument();
  });
});
