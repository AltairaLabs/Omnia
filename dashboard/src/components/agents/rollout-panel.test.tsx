import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  RolloutPanel,
  deriveRolloutPhase,
  stepLabel,
  buildStepItems,
} from "./rollout-panel";
import type { RolloutConfig, RolloutStatus } from "@/types/agent-runtime";

const meshSpec: RolloutConfig = {
  candidate: { promptPackRef: { name: "rag-hero-pack-v2" } },
  trafficRouting: { mode: "mesh" },
  steps: [{ setWeight: 25 }, { pause: {} }, { setWeight: 100 }],
};

const activeStatus: RolloutStatus = {
  active: true,
  currentStep: 1,
  currentWeight: 25,
  trafficRoutingMode: "mesh",
  trafficWeightEnforced: true,
  message: "step 1: paused indefinitely",
  stableVersion: "0.1.0",
  candidateVersion: "0.2.0",
};

describe("deriveRolloutPhase", () => {
  it("reports in-progress with step position when active", () => {
    expect(deriveRolloutPhase(meshSpec, activeStatus)).toEqual({
      label: "In progress · step 2/3",
      variant: "default",
    });
  });

  it("omits the step suffix when there are no steps", () => {
    expect(deriveRolloutPhase(undefined, { active: true })).toEqual({
      label: "In progress",
      variant: "default",
    });
  });

  it("reports a rollback from the terminal message", () => {
    const p = deriveRolloutPhase(meshSpec, {
      active: false,
      message: "auto-rollback triggered: analysis failed",
    });
    expect(p).toEqual({ label: "Rolled back", variant: "destructive" });
  });

  it("reports a promotion from the terminal message", () => {
    const p = deriveRolloutPhase(meshSpec, { active: false, message: "promoted" });
    expect(p).toEqual({ label: "Promoted", variant: "secondary" });
  });

  it("falls back to idle for an unknown/empty message", () => {
    expect(deriveRolloutPhase(meshSpec, { active: false })).toEqual({
      label: "Idle",
      variant: "outline",
    });
  });
});

describe("stepLabel", () => {
  it("labels a setWeight step (including 0%)", () => {
    expect(stepLabel({ setWeight: 25 })).toBe("Set 25%");
    expect(stepLabel({ setWeight: 0 })).toBe("Set 0%");
  });
  it("labels an analysis step with its template", () => {
    expect(stepLabel({ analysis: { templateName: "eval-gate" } })).toBe("Analysis: eval-gate");
  });
  it("labels a pause with and without duration", () => {
    expect(stepLabel({ pause: { duration: "5m" } })).toBe("Pause 5m");
    expect(stepLabel({ pause: {} })).toBe("Pause");
  });
  it("falls back for an empty step", () => {
    expect(stepLabel({})).toBe("Step");
  });
});

describe("buildStepItems", () => {
  it("assigns unique occurrence-counted keys to duplicate labels", () => {
    const items = buildStepItems([{ pause: {} }, { pause: {} }], 0, true);
    expect(items.map((i) => i.key)).toEqual(["Pause#1", "Pause#2"]);
  });
  it("marks the current step when active, and past steps", () => {
    const items = buildStepItems([{ setWeight: 25 }, { pause: {} }, { setWeight: 100 }], 1, true);
    expect(items[0].className).toContain("bg-muted"); // past
    expect(items[1].className).toContain("border-primary"); // current
    expect(items[2].className).toContain("border-dashed"); // future
  });
  it("does not highlight a current step when inactive", () => {
    const items = buildStepItems([{ pause: {} }], 0, false);
    expect(items[0].className).not.toContain("border-primary");
  });
});

describe("RolloutPanel", () => {
  it("renders nothing when there is no rollout spec or status", () => {
    const { container } = render(<RolloutPanel />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the live split, mode, enforcement, version delta, steps, and message", () => {
    render(<RolloutPanel spec={meshSpec} status={activeStatus} />);
    expect(screen.getByText("In progress · step 2/3")).toBeInTheDocument();
    // Traffic split: candidate 25 -> stable 75.
    expect(screen.getByText("75%")).toBeInTheDocument();
    expect(screen.getByText("25%")).toBeInTheDocument();
    expect(screen.getByText("mesh")).toBeInTheDocument();
    expect(screen.getByText("enforced")).toBeInTheDocument();
    expect(screen.getByText("0.2.0")).toBeInTheDocument();
    expect(screen.getByText("Set 25%")).toBeInTheDocument();
    expect(screen.getByText("step 1: paused indefinitely")).toBeInTheDocument();
  });

  it("labels replicaWeighted weighting as approximate", () => {
    render(
      <RolloutPanel
        spec={{ trafficRouting: { mode: "replicaWeighted" }, steps: [{ setWeight: 50 }] }}
        status={{
          active: true,
          currentStep: 0,
          currentWeight: 50,
          trafficRoutingMode: "replicaWeighted",
          trafficWeightEnforced: false,
        }}
      />,
    );
    expect(screen.getByText("approx")).toBeInTheDocument();
    expect(screen.getByText("replicaWeighted")).toBeInTheDocument();
  });

  it("shows a rollback pill once the rollout has rolled back", () => {
    render(
      <RolloutPanel
        spec={meshSpec}
        status={{ active: false, currentWeight: 0, message: "auto-rollback: pod unhealthy" }}
      />,
    );
    expect(screen.getByText("Rolled back")).toBeInTheDocument();
    // Back to 100% stable.
    expect(screen.getByText("100%")).toBeInTheDocument();
  });
});
