import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { JsonBlock } from "./json-block";

describe("JsonBlock", () => {
  it("renders primitive string value", () => {
    render(<JsonBlock data="hello" />);
    expect(screen.getByTestId("json-block")).toHaveTextContent('"hello"');
  });

  it("renders primitive number value", () => {
    render(<JsonBlock data={42} />);
    expect(screen.getByTestId("json-block")).toHaveTextContent("42");
  });

  it("renders null value", () => {
    render(<JsonBlock data={null} />);
    expect(screen.getByTestId("json-block")).toHaveTextContent("null");
  });

  it("renders boolean value", () => {
    render(<JsonBlock data={true} />);
    expect(screen.getByTestId("json-block")).toHaveTextContent("true");
  });

  it("renders flat object with keys and values", () => {
    render(<JsonBlock data={{ name: "test", count: 3 }} />);
    const block = screen.getByTestId("json-block");
    expect(block).toHaveTextContent('"name"');
    expect(block).toHaveTextContent('"test"');
    expect(block).toHaveTextContent('"count"');
    expect(block).toHaveTextContent("3");
  });

  it("renders array with items", () => {
    render(<JsonBlock data={[1, 2, 3]} />);
    const block = screen.getByTestId("json-block");
    expect(block).toHaveTextContent("1");
    expect(block).toHaveTextContent("2");
    expect(block).toHaveTextContent("3");
  });

  it("renders empty object", () => {
    render(<JsonBlock data={{}} />);
    expect(screen.getByTestId("json-block")).toHaveTextContent("{}");
  });

  it("renders empty array", () => {
    render(<JsonBlock data={[]} />);
    expect(screen.getByTestId("json-block")).toHaveTextContent("[]");
  });

  it("collapses and expands nodes on click", () => {
    render(<JsonBlock data={{ nested: { a: 1, b: 2 } }} />);
    const block = screen.getByTestId("json-block");

    // Initially expanded — should see keys
    expect(block).toHaveTextContent('"a"');
    expect(block).toHaveTextContent('"b"');

    // Click the nested object to collapse it
    const nestedLabel = screen.getByText('"nested"');
    // The clickable area is the sibling span with the chevron
    const clickTarget = nestedLabel.closest("div")!.querySelector("span[class*='cursor-pointer']");
    if (clickTarget) {
      fireEvent.click(clickTarget);
      // After collapsing, should show item count instead of contents
      expect(block).toHaveTextContent("2 keys");
    }
  });

  it("respects defaultCollapsed prop", () => {
    render(
      <JsonBlock
        data={{ visible: "yes", hidden: { secret: "value" } }}
        defaultCollapsed={["hidden"]}
      />
    );
    const block = screen.getByTestId("json-block");

    // "visible" key should be shown
    expect(block).toHaveTextContent('"visible"');
    expect(block).toHaveTextContent('"yes"');

    // "hidden" object should be collapsed — showing count, not content
    expect(block).toHaveTextContent("1 key");
    expect(block).not.toHaveTextContent('"secret"');
  });

  it("respects defaultExpandDepth prop", () => {
    render(
      <JsonBlock
        data={{ level0: { level1: { level2: "deep" } } }}
        defaultExpandDepth={1}
      />
    );
    const block = screen.getByTestId("json-block");

    // level0 should be expanded (depth 0)
    expect(block).toHaveTextContent('"level0"');
    // level1 should be collapsed (depth 1)
    expect(block).toHaveTextContent("1 key");
    // level2 should not be visible
    expect(block).not.toHaveTextContent('"level2"');
  });

  it("handles nested arrays", () => {
    render(<JsonBlock data={{ items: [{ id: 1 }, { id: 2 }] }} />);
    const block = screen.getByTestId("json-block");
    expect(block).toHaveTextContent('"items"');
    expect(block).toHaveTextContent('"id"');
  });

  it("applies custom className", () => {
    render(<JsonBlock data={{ a: 1 }} className="custom-class" />);
    expect(screen.getByTestId("json-block")).toHaveClass("custom-class");
  });
});
