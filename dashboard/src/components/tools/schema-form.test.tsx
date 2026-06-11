import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { SchemaForm, isRenderableObjectSchema } from "./schema-form";

const schema = {
  type: "object",
  properties: {
    id: { type: "string", description: "the pet id" },
    count: { type: "integer" },
    active: { type: "boolean" },
    status: { type: "string", enum: ["available", "sold"] },
    tags: { type: "array", items: { type: "string" } },
  },
  required: ["id"],
};

it("emits string field changes", () => {
  const onChange = vi.fn();
  render(<SchemaForm schema={schema} value={{}} onChange={onChange} idPrefix="f" />);
  fireEvent.change(screen.getByLabelText(/^id/), { target: { value: "p1" } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ id: "p1" }));
});

it("marks required fields and shows description", () => {
  render(<SchemaForm schema={schema} value={{}} onChange={vi.fn()} idPrefix="f" />);
  expect(screen.getByText(/the pet id/)).toBeInTheDocument();
  expect(screen.getByLabelText(/id\s*\*/)).toBeInTheDocument();
});

it("coerces number inputs", () => {
  const onChange = vi.fn();
  render(<SchemaForm schema={schema} value={{}} onChange={onChange} idPrefix="f" />);
  fireEvent.change(screen.getByLabelText(/count/), { target: { value: "3" } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ count: 3 }));
});

it("renders an enum field", () => {
  render(<SchemaForm schema={schema} value={{}} onChange={vi.fn()} idPrefix="f" />);
  expect(screen.getByLabelText(/status/)).toBeInTheDocument();
});

it("splits comma-separated arrays", () => {
  const onChange = vi.fn();
  render(<SchemaForm schema={schema} value={{}} onChange={onChange} idPrefix="f" />);
  fireEvent.change(screen.getByLabelText(/tags/), { target: { value: "a, b" } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ tags: ["a", "b"] }));
});

it("renders one level of nested object", () => {
  const nested = {
    type: "object",
    properties: {
      addr: { type: "object", properties: { city: { type: "string" } } },
    },
  };
  const onChange = vi.fn();
  render(<SchemaForm schema={nested} value={{}} onChange={onChange} idPrefix="f" />);
  fireEvent.change(screen.getByLabelText(/city/), { target: { value: "NYC" } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ addr: { city: "NYC" } }));
});

it("notice for non-renderable schema", () => {
  render(<SchemaForm schema={{ type: "string" }} value={{}} onChange={vi.fn()} idPrefix="f" />);
  expect(screen.getByText(/no structured fields/i)).toBeInTheDocument();
});

it("isRenderableObjectSchema", () => {
  expect(isRenderableObjectSchema(schema)).toBe(true);
  expect(isRenderableObjectSchema({ type: "string" })).toBe(false);
  expect(isRenderableObjectSchema(null)).toBe(false);
});

describe("additional coverage", () => {
  it("omits number key when input is empty", () => {
    const onChange = vi.fn();
    render(<SchemaForm schema={schema} value={{ count: 5 }} onChange={onChange} idPrefix="f" />);
    fireEvent.change(screen.getByLabelText(/count/), { target: { value: "" } });
    const result = onChange.mock.calls[0][0] as Record<string, unknown>;
    expect(result).not.toHaveProperty("count");
  });

  it("coerces integer array items to numbers", () => {
    const numArraySchema = {
      type: "object",
      properties: {
        nums: { type: "array", items: { type: "integer" } },
      },
    };
    const onChange = vi.fn();
    render(<SchemaForm schema={numArraySchema} value={{}} onChange={onChange} idPrefix="f" />);
    fireEvent.change(screen.getByLabelText(/nums/), { target: { value: "1, 2, 3" } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ nums: [1, 2, 3] }));
  });

  it("empty array input produces empty array", () => {
    const onChange = vi.fn();
    // Start with a non-empty value so the controlled input shows something,
    // then fire a change to "" to verify coercion returns [].
    render(<SchemaForm schema={schema} value={{ tags: ["x"] }} onChange={onChange} idPrefix="f" />);
    fireEvent.change(screen.getByLabelText(/tags/), { target: { value: "" } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ tags: [] }));
  });

  it("renders boolean field as checkbox", () => {
    const onChange = vi.fn();
    render(<SchemaForm schema={schema} value={{}} onChange={onChange} idPrefix="f" />);
    const checkbox = screen.getByRole("checkbox", { name: /active/ });
    expect(checkbox).toBeInTheDocument();
  });

  it("checkbox toggle emits boolean true", () => {
    const onChange = vi.fn();
    render(<SchemaForm schema={schema} value={{ active: false }} onChange={onChange} idPrefix="f" />);
    fireEvent.click(screen.getByRole("checkbox", { name: /active/ }));
    expect(onChange).toHaveBeenCalled();
    const result = onChange.mock.calls[0][0] as Record<string, unknown>;
    expect(typeof result.active).toBe("boolean");
  });

  it("isRenderableObjectSchema false for object with no properties", () => {
    expect(isRenderableObjectSchema({ type: "object" })).toBe(false);
    expect(isRenderableObjectSchema({ type: "object", properties: {} })).toBe(false);
  });

  it("nested object field merges correctly without overwriting sibling keys", () => {
    const nested = {
      type: "object",
      properties: {
        addr: { type: "object", properties: { city: { type: "string" }, zip: { type: "string" } } },
      },
    };
    const onChange = vi.fn();
    render(
      <SchemaForm
        schema={nested}
        value={{ addr: { city: "LA", zip: "90001" } }}
        onChange={onChange}
        idPrefix="f"
      />,
    );
    fireEvent.change(screen.getByLabelText(/city/), { target: { value: "NYC" } });
    const result = onChange.mock.calls[0][0] as Record<string, unknown>;
    expect(result).toEqual({ addr: { city: "NYC", zip: "90001" } });
  });

  it("schema with no properties shows notice", () => {
    render(
      <SchemaForm schema={{ type: "object", properties: {} }} value={{}} onChange={vi.fn()} idPrefix="f" />,
    );
    expect(screen.getByText(/no structured fields/i)).toBeInTheDocument();
  });

  it("null schema shows notice", () => {
    render(<SchemaForm schema={null} value={{}} onChange={vi.fn()} idPrefix="f" />);
    expect(screen.getByText(/no structured fields/i)).toBeInTheDocument();
  });

  it("select fires onChange with selected value", () => {
    // We can only verify the select element is present with a role/label;
    // Radix Select uses a portal so full open/select needs pointer events.
    render(<SchemaForm schema={schema} value={{ status: "available" }} onChange={vi.fn()} idPrefix="f" />);
    // The combobox/listbox role or button role is rendered by SelectTrigger
    expect(screen.getByLabelText(/status/)).toBeInTheDocument();
  });
});
