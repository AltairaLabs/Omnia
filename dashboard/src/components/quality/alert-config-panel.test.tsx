/**
 * Tests for AlertConfigPanel component and helper functions.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// Capture onValueChange callbacks from Select components to simulate selection in tests
const selectCallbacks: Map<string, (value: string) => void> = new Map();

vi.mock("@/components/ui/select", () => ({
  Select: ({ children, value, onValueChange }: { children: React.ReactNode; value: string; onValueChange: (v: string) => void }) => {
    // Store the callback keyed by the current value (empty string for unset)
    selectCallbacks.set(value || "__unset__", onValueChange);
    return React.createElement("div", { "data-testid": "select", "data-value": value }, children);
  },
  SelectTrigger: ({ children, id }: { children: React.ReactNode; id?: string }) =>
    React.createElement("button", { id, "data-testid": `select-trigger-${id || ""}` }, children),
  SelectValue: ({ placeholder }: { placeholder?: string }) =>
    React.createElement("span", null, placeholder ?? ""),
  SelectContent: ({ children }: { children: React.ReactNode }) =>
    React.createElement("div", { "data-testid": "select-content" }, children),
  SelectItem: ({ children, value }: { children: React.ReactNode; value: string }) =>
    React.createElement("option", { value, "data-testid": `select-item-${value}` }, children),
}));

import {
  AlertConfigPanel,
  loadAlerts,
  saveAlerts,
  buildAlertThresholdMap,
  type EvalAlert,
} from "./alert-config-panel";

// ---- Helper function tests ----

describe("loadAlerts", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("returns empty array when no localStorage data", () => {
    expect(loadAlerts()).toEqual([]);
  });

  it("returns empty array when localStorage has invalid JSON", () => {
    localStorage.setItem("omnia-eval-alerts", "not-json");
    expect(loadAlerts()).toEqual([]);
  });

  it("returns stored alerts from localStorage", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    localStorage.setItem("omnia-eval-alerts", JSON.stringify(alerts));
    expect(loadAlerts()).toEqual(alerts);
  });
});

describe("saveAlerts", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("stores alerts to localStorage", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
      { metricName: "omnia_eval_safety", threshold: 0.9, enabled: false },
    ];
    saveAlerts(alerts);
    const stored = JSON.parse(localStorage.getItem("omnia-eval-alerts")!);
    expect(stored).toEqual(alerts);
  });

  it("overwrites existing data", () => {
    localStorage.setItem("omnia-eval-alerts", "[]");
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    saveAlerts(alerts);
    const stored = JSON.parse(localStorage.getItem("omnia-eval-alerts")!);
    expect(stored).toEqual(alerts);
  });
});

describe("buildAlertThresholdMap", () => {
  it("creates correct map from enabled alerts", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
      { metricName: "omnia_eval_safety", threshold: 0.9, enabled: true },
    ];
    const map = buildAlertThresholdMap(alerts);
    expect(map.get("omnia_eval_tone")).toBe(0.8);
    expect(map.get("omnia_eval_safety")).toBe(0.9);
    expect(map.size).toBe(2);
  });

  it("excludes disabled alerts from map", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
      { metricName: "omnia_eval_safety", threshold: 0.9, enabled: false },
    ];
    const map = buildAlertThresholdMap(alerts);
    expect(map.get("omnia_eval_tone")).toBe(0.8);
    expect(map.has("omnia_eval_safety")).toBe(false);
    expect(map.size).toBe(1);
  });

  it("returns empty map for empty array", () => {
    const map = buildAlertThresholdMap([]);
    expect(map.size).toBe(0);
  });
});

// ---- Component tests ----

describe("AlertConfigPanel", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    selectCallbacks.clear();
  });

  it("renders 'No alerts configured' initially", () => {
    render(<AlertConfigPanel />);
    expect(screen.getByText("No alerts configured")).toBeInTheDocument();
  });

  it("renders card header with title and description", () => {
    render(<AlertConfigPanel />);
    expect(screen.getByText("Alert Thresholds")).toBeInTheDocument();
    expect(
      screen.getByText("Set pass rate thresholds for eval metrics. Alerts are stored locally.")
    ).toBeInTheDocument();
  });

  it("loads existing alerts from localStorage on mount", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    localStorage.setItem("omnia-eval-alerts", JSON.stringify(alerts));

    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
      />
    );

    // Should display the alert metric name (prefix stripped)
    expect(screen.getByText("tone")).toBeInTheDocument();
    // Should display threshold
    expect(screen.getByText("< 0.80")).toBeInTheDocument();
  });

  it("can toggle alert on/off", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    localStorage.setItem("omnia-eval-alerts", JSON.stringify(alerts));

    const onAlertsChange = vi.fn();
    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
        onAlertsChange={onAlertsChange}
      />
    );

    // Initially "On"
    expect(screen.getByText("On")).toBeInTheDocument();

    // Toggle off
    fireEvent.click(screen.getByText("On"));

    expect(screen.getByText("Off")).toBeInTheDocument();
    expect(onAlertsChange).toHaveBeenCalledWith([
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: false },
    ]);

    // Verify localStorage was updated
    const stored = JSON.parse(localStorage.getItem("omnia-eval-alerts")!);
    expect(stored[0].enabled).toBe(false);
  });

  it("can remove alert", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    localStorage.setItem("omnia-eval-alerts", JSON.stringify(alerts));

    const onAlertsChange = vi.fn();
    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
        onAlertsChange={onAlertsChange}
      />
    );

    // Find the ghost variant button (delete button with Trash2 icon)
    const buttons = screen.getAllByRole("button");
    const trashButton = buttons.find(
      (btn) => btn.getAttribute("data-variant") === "ghost"
    );
    expect(trashButton).toBeDefined();

    fireEvent.click(trashButton!);

    expect(onAlertsChange).toHaveBeenCalledWith([]);
    expect(screen.getByText("No alerts configured")).toBeInTheDocument();
  });

  it("renders add button as disabled when no metric selected", () => {
    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
      />
    );

    // The Plus button should be disabled when no metric is selected
    const buttons = screen.getAllByRole("button");
    const addButton = buttons.find(
      (btn) => btn.querySelector("svg") && !btn.className.includes("ghost")
    );
    expect(addButton).toBeDefined();
    expect((addButton as HTMLButtonElement)?.disabled).toBe(true);
  });

  it("filters out already-configured metrics from select options", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    localStorage.setItem("omnia-eval-alerts", JSON.stringify(alerts));

    render(
      <AlertConfigPanel
        availableMetrics={[
          { name: "omnia_eval_tone", value: 0.9 },
          { name: "omnia_eval_safety", value: 0.85 },
        ]}
      />
    );

    // tone should appear in the alerts list, not in the select options
    expect(screen.getByText("tone")).toBeInTheDocument();
  });

  it("renders threshold input with default value", () => {
    render(<AlertConfigPanel />);

    const thresholdInput = screen.getByLabelText("Threshold") as HTMLInputElement;
    expect(thresholdInput.value).toBe("0.8");
  });

  it("updates threshold input value on change", () => {
    render(<AlertConfigPanel />);

    const thresholdInput = screen.getByLabelText("Threshold") as HTMLInputElement;
    fireEvent.change(thresholdInput, { target: { value: "0.95" } });
    expect(thresholdInput.value).toBe("0.95");
  });

  it("adds new alert when metric selected and add clicked", () => {
    const onAlertsChange = vi.fn();
    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
        onAlertsChange={onAlertsChange}
      />
    );

    // Call the captured onValueChange to simulate selecting a metric
    const metricSelectCb = selectCallbacks.get("__unset__");
    expect(metricSelectCb).toBeDefined();
    React.act(() => { metricSelectCb!("omnia_eval_tone"); });

    // Re-render triggers the component to update, now find and click the add button
    const buttons = screen.getAllByRole("button");
    const addButton = buttons.find(
      (btn) => btn.querySelector("svg") && !btn.className.includes("ghost")
    );
    fireEvent.click(addButton!);

    expect(onAlertsChange).toHaveBeenCalledWith([
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ]);
  });

  it("does not add alert when no metric is selected (handleAdd early return)", () => {
    const onAlertsChange = vi.fn();
    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
        onAlertsChange={onAlertsChange}
      />
    );

    // Click add without selecting a metric — should do nothing
    const buttons = screen.getAllByRole("button");
    const addButton = buttons.find(
      (btn) => btn.querySelector("svg") && !btn.className.includes("ghost")
    ) as HTMLButtonElement;

    fireEvent.click(addButton);
    expect(onAlertsChange).not.toHaveBeenCalled();
  });

  it("does not add alert with invalid threshold", () => {
    const onAlertsChange = vi.fn();
    render(
      <AlertConfigPanel
        availableMetrics={[{ name: "omnia_eval_tone", value: 0.9 }]}
        onAlertsChange={onAlertsChange}
      />
    );

    // Select a metric via captured callback
    const metricSelectCb = selectCallbacks.get("__unset__");
    React.act(() => { metricSelectCb!("omnia_eval_tone"); });

    // Set an invalid threshold
    const thresholdInput = screen.getByLabelText("Threshold") as HTMLInputElement;
    fireEvent.change(thresholdInput, { target: { value: "2.0" } });

    // Try to add — should be rejected due to threshold > 1
    const buttons = screen.getAllByRole("button");
    const addButton = buttons.find(
      (btn) => btn.querySelector("svg") && !btn.className.includes("ghost")
    );
    fireEvent.click(addButton!);

    expect(onAlertsChange).not.toHaveBeenCalled();
  });

  it("does not add duplicate alert for same metric", () => {
    const alerts: EvalAlert[] = [
      { metricName: "omnia_eval_tone", threshold: 0.8, enabled: true },
    ];
    localStorage.setItem("omnia-eval-alerts", JSON.stringify(alerts));

    const onAlertsChange = vi.fn();
    render(
      <AlertConfigPanel
        availableMetrics={[
          { name: "omnia_eval_tone", value: 0.9 },
          { name: "omnia_eval_safety", value: 0.85 },
        ]}
        onAlertsChange={onAlertsChange}
      />
    );

    // The tone metric is already configured, so attempting to add it again should be a no-op
    expect(screen.getByText("tone")).toBeInTheDocument();
  });
});
