/**
 * Tests for FunctionTestPanel.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

// Monaco doesn't render in jsdom; stand in a plain textarea that preserves the
// editor's value/onChange contract so the panel's toggle/parse logic is tested.
vi.mock("@/components/editors/json-editor", () => ({
  JsonEditor: ({
    value,
    onChange,
    ariaLabel,
  }: {
    value: string;
    onChange: (v: string) => void;
    ariaLabel?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  ),
}));

import { FunctionTestPanel } from "./function-test-panel";

const mockFetch = vi.fn();
global.fetch = mockFetch;

const inputSchema = {
  type: "object",
  properties: {
    topic: { type: "string", description: "What to research" },
  },
  required: ["topic"],
};

function mkResponse(status: number, body: string, contentType = "application/json"): Response {
  return {
    status,
    text: () => Promise.resolve(body),
    headers: { get: () => contentType },
  } as unknown as Response;
}

describe("FunctionTestPanel", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it("renders form fields derived from the input schema", () => {
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    // Form mode is the default for a renderable object schema.
    expect(screen.getByLabelText("topic *")).toBeInTheDocument();
  });

  it("seeds the input from the schema example so Run works out of the box", () => {
    const withExample = {
      type: "object",
      required: ["topic"],
      properties: {
        topic: { type: "string", example: "What drove the 2023 battery surge?" },
      },
    };
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={withExample} />,
    );
    expect(screen.getByLabelText("topic *")).toHaveValue("What drove the 2023 battery surge?");
  });

  it("toggles to JSON mode and shows the sample arguments", () => {
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const textarea = screen.getByLabelText("Input (JSON)") as HTMLTextAreaElement;
    expect(textarea.value).toContain('"topic"');
  });

  it("invokes the function and renders the JSON response", async () => {
    mockFetch.mockResolvedValue(mkResponse(200, '{"summary":"all done"}'));
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/demo/functions/deep-research/invoke",
      expect.objectContaining({ method: "POST" }),
    );
    await screen.findByText("Success");
    expect(screen.getByText(/all done/)).toBeInTheDocument();
  });

  it("unwraps the success envelope, links the invocation, and shows usage", async () => {
    mockFetch.mockResolvedValue(
      mkResponse(
        200,
        JSON.stringify({
          output: { report: "done" },
          invocation_id: "sess-12345678",
          duration_ms: 1500,
          usage: { input_tokens: 10, output_tokens: 20, cost_usd: 0.0012 },
        }),
      ),
    );
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    await screen.findByText("Success");
    // Output is unwrapped (the envelope's `output`, not the whole envelope).
    expect(screen.getByText(/report/)).toBeInTheDocument();
    expect(screen.getByText(/done/)).toBeInTheDocument();
    // Server-side duration is preferred over the client round-trip.
    expect(screen.getByText(/1500ms/)).toBeInTheDocument();
    // invocation_id links to the recorded session.
    const link = screen.getByRole("link", { name: /invocation/i });
    expect(link).toHaveAttribute("href", "/sessions/sess-12345678");
    // Usage chips (tokens combined into one node, plus cost).
    expect(screen.getByText(/10 in \/ 20 out/)).toBeInTheDocument();
  });

  it("renders a Failed result with the error body on a non-2xx response", async () => {
    mockFetch.mockResolvedValue(mkResponse(400, '{"error":"invalid_input"}'));
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    await screen.findByText("Failed");
    expect(screen.getByText(/invalid_input/)).toBeInTheDocument();
  });

  it("renders a plain-text (non-JSON) response body verbatim", async () => {
    mockFetch.mockResolvedValue(mkResponse(200, "raw text body", "text/plain"));
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    await screen.findByText("Success");
    expect(screen.getByText("raw text body")).toBeInTheDocument();
  });

  it("shows a network error when the fetch rejects", async () => {
    mockFetch.mockRejectedValue(new Error("boom"));
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    await screen.findByText("Failed");
    expect(screen.getByText(/boom/)).toBeInTheDocument();
  });

  it("blocks the run and shows an error on invalid JSON", async () => {
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    const textarea = screen.getByLabelText("Input (JSON)");
    fireEvent.change(textarea, { target: { value: "{ not json" } });
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    expect(await screen.findByText(/Invalid JSON/)).toBeInTheDocument();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("falls back to a raw JSON editor when there is no input schema", () => {
    render(<FunctionTestPanel functionName="deep-research" workspace="demo" />);
    // No Form/JSON toggle — just the raw editor seeded with an empty object.
    expect(screen.queryByRole("button", { name: "Form" })).not.toBeInTheDocument();
    const textarea = screen.getByLabelText("Input (JSON)") as HTMLTextAreaElement;
    expect(textarea.value).toBe("{}");
  });

  it("disables Run and shows a notice when the function is not ready", () => {
    render(
      <FunctionTestPanel
        functionName="deep-research"
        workspace="demo"
        inputSchema={inputSchema}
        ready={false}
        unavailableReason="Pending"
      />,
    );
    const run = screen.getByRole("button", { name: /Run/ });
    expect(run).toBeDisabled();
    expect(screen.getByText(/not ready/i)).toBeInTheDocument();
    expect(screen.getByText(/Pending/)).toBeInTheDocument();
  });

  it("does not invoke when the function is not ready", () => {
    render(
      <FunctionTestPanel
        functionName="deep-research"
        workspace="demo"
        inputSchema={inputSchema}
        ready={false}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("enables Run by default (ready omitted)", () => {
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    expect(screen.getByRole("button", { name: /Run/ })).toBeEnabled();
  });

  it("posts the edited form values as the request body", async () => {
    mockFetch.mockResolvedValue(mkResponse(200, "{}"));
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.change(screen.getByLabelText("topic *"), { target: { value: "quantum" } });
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));

    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
    const body = JSON.parse(mockFetch.mock.calls[0][1].body as string);
    expect(body).toEqual({ topic: "quantum" });
  });

  it("runs on Cmd/Ctrl+Enter", async () => {
    mockFetch.mockResolvedValue(mkResponse(200, "{}"));
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.keyDown(screen.getByLabelText("topic *"), { key: "Enter", metaKey: true });
    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
  });

  it("does not run on Cmd+Enter when the function is not ready", () => {
    render(
      <FunctionTestPanel
        functionName="deep-research"
        workspace="demo"
        inputSchema={inputSchema}
        ready={false}
      />,
    );
    fireEvent.keyDown(screen.getByLabelText("topic *"), { key: "Enter", ctrlKey: true });
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("copies the response output to the clipboard", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    mockFetch.mockResolvedValue(
      mkResponse(200, JSON.stringify({ output: { report: "done" }, invocation_id: "s1" })),
    );
    render(
      <FunctionTestPanel functionName="deep-research" workspace="demo" inputSchema={inputSchema} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));
    await screen.findByText("Success");
    fireEvent.click(screen.getByRole("button", { name: /copy/i }));
    expect(writeText).toHaveBeenCalledWith(JSON.stringify({ report: "done" }, null, 2));
  });

  it("notes that a successful output matched the declared output schema", async () => {
    mockFetch.mockResolvedValue(
      mkResponse(200, JSON.stringify({ output: { report: "done" }, invocation_id: "s1" })),
    );
    render(
      <FunctionTestPanel
        functionName="deep-research"
        workspace="demo"
        inputSchema={inputSchema}
        outputSchema={{ type: "object", properties: { report: { type: "string" } } }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/ }));
    await screen.findByText("Success");
    expect(screen.getByText(/matches the output schema/i)).toBeInTheDocument();
  });
});
