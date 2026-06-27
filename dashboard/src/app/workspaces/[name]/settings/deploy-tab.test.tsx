import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { DeployTab } from "./deploy-tab";

vi.mock("@/components/workspace/export-deploy-profile", () => ({
  default: ({ workspace }: { workspace: string }) => <div>export:{workspace}</div>,
}));

describe("DeployTab", () => {
  it("renders the export component for the workspace", () => {
    render(<DeployTab workspaceName="team-acme" />);
    expect(screen.getByText("export:team-acme")).toBeInTheDocument();
  });
});
