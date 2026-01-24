import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  K8sLabelSelector,
  matchesLabelSelector,
  type LabelSelectorValue,
} from "./k8s-label-selector";

describe("K8sLabelSelector", () => {
  const defaultProps = {
    value: {},
    onChange: vi.fn(),
  };

  it("should render with label and description", () => {
    render(
      <K8sLabelSelector
        {...defaultProps}
        label="Test Label"
        description="Test description"
      />
    );

    expect(screen.getByText("Test Label")).toBeInTheDocument();
    expect(screen.getByText("Test description")).toBeInTheDocument();
  });

  it("should render empty state when no selectors configured", () => {
    render(<K8sLabelSelector {...defaultProps} />);

    expect(
      screen.getByText(/No selectors configured/)
    ).toBeInTheDocument();
  });

  it("should display existing matchLabels", () => {
    const value: LabelSelectorValue = {
      matchLabels: {
        env: "production",
        tier: "frontend",
      },
    };

    render(<K8sLabelSelector {...defaultProps} value={value} />);

    expect(screen.getByText("env=production")).toBeInTheDocument();
    expect(screen.getByText("tier=frontend")).toBeInTheDocument();
  });

  it("should call onChange when removing a matchLabel", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const value: LabelSelectorValue = {
      matchLabels: {
        env: "production",
        tier: "frontend",
      },
    };

    render(<K8sLabelSelector value={value} onChange={onChange} />);

    // Find the remove button for the first label
    const removeButtons = screen.getAllByRole("button");
    const removeButton = removeButtons.find(
      (btn) => btn.querySelector("svg")?.classList.contains("lucide-x")
    );

    if (removeButton) {
      await user.click(removeButton);

      expect(onChange).toHaveBeenCalledWith(
        expect.objectContaining({
          matchLabels: expect.any(Object),
        })
      );
    }
  });

  it("should toggle advanced expressions section", async () => {
    const user = userEvent.setup();
    render(<K8sLabelSelector {...defaultProps} />);

    // Find and click the toggle button
    const toggleButton = screen.getByText(/Show advanced expressions/);
    await user.click(toggleButton);

    expect(screen.getByText("Match Expressions")).toBeInTheDocument();

    // Toggle back
    await user.click(screen.getByText(/Hide advanced expressions/));
    expect(screen.queryByText("Match Expressions")).not.toBeInTheDocument();
  });

  it("should display existing matchExpressions", () => {
    const value: LabelSelectorValue = {
      matchExpressions: [
        { key: "env", operator: "In", values: ["prod", "staging"] },
        { key: "tier", operator: "Exists" },
      ],
    };

    render(<K8sLabelSelector {...defaultProps} value={value} />);

    // Expressions section should be visible since we have expressions
    expect(screen.getByText("env in (prod, staging)")).toBeInTheDocument();
    expect(screen.getByText("tier")).toBeInTheDocument();
  });

  it("should disable inputs when disabled prop is true", () => {
    render(<K8sLabelSelector {...defaultProps} disabled />);

    // The add buttons should not be visible
    expect(screen.queryByRole("button", { name: /add/i })).not.toBeInTheDocument();
  });

  it("should render preview component when provided and selector is not empty", () => {
    const value: LabelSelectorValue = {
      matchLabels: { env: "production" },
    };

    render(
      <K8sLabelSelector
        {...defaultProps}
        value={value}
        previewComponent={<div data-testid="preview">Preview Content</div>}
      />
    );

    expect(screen.getByTestId("preview")).toBeInTheDocument();
  });

  it("should not render preview component when selector is empty", () => {
    render(
      <K8sLabelSelector
        {...defaultProps}
        value={{}}
        previewComponent={<div data-testid="preview">Preview Content</div>}
      />
    );

    expect(screen.queryByTestId("preview")).not.toBeInTheDocument();
  });
});

describe("matchesLabelSelector", () => {
  describe("matchLabels", () => {
    it("should return true when all matchLabels match exactly", () => {
      const resourceLabels = { env: "production", tier: "frontend" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
    });

    it("should return false when a matchLabel does not match", () => {
      const resourceLabels = { env: "staging", tier: "frontend" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });

    it("should return false when a matchLabel key is missing", () => {
      const resourceLabels = { tier: "frontend" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });

    it("should return true with empty matchLabels", () => {
      const resourceLabels = { env: "production" };
      const selector: LabelSelectorValue = {
        matchLabels: {},
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
    });

    it("should handle undefined resource labels", () => {
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
      };

      expect(matchesLabelSelector(undefined, selector)).toBe(false);
    });

    it("should require all matchLabels to match (AND logic)", () => {
      const resourceLabels = { env: "production", tier: "backend" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production", tier: "frontend" },
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });
  });

  describe("matchExpressions", () => {
    describe("In operator", () => {
      it("should return true when value is in list", () => {
        const resourceLabels = { env: "production" };
        const selector: LabelSelectorValue = {
          matchExpressions: [
            { key: "env", operator: "In", values: ["production", "staging"] },
          ],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
      });

      it("should return false when value is not in list", () => {
        const resourceLabels = { env: "development" };
        const selector: LabelSelectorValue = {
          matchExpressions: [
            { key: "env", operator: "In", values: ["production", "staging"] },
          ],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
      });

      it("should return false when key is missing", () => {
        const resourceLabels = { tier: "frontend" };
        const selector: LabelSelectorValue = {
          matchExpressions: [
            { key: "env", operator: "In", values: ["production"] },
          ],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
      });
    });

    describe("NotIn operator", () => {
      it("should return true when value is not in list", () => {
        const resourceLabels = { env: "production" };
        const selector: LabelSelectorValue = {
          matchExpressions: [
            { key: "env", operator: "NotIn", values: ["staging", "development"] },
          ],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
      });

      it("should return false when value is in list", () => {
        const resourceLabels = { env: "staging" };
        const selector: LabelSelectorValue = {
          matchExpressions: [
            { key: "env", operator: "NotIn", values: ["staging", "development"] },
          ],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
      });

      it("should return true when key is missing", () => {
        const resourceLabels = { tier: "frontend" };
        const selector: LabelSelectorValue = {
          matchExpressions: [
            { key: "env", operator: "NotIn", values: ["staging"] },
          ],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
      });
    });

    describe("Exists operator", () => {
      it("should return true when key exists", () => {
        const resourceLabels = { env: "production" };
        const selector: LabelSelectorValue = {
          matchExpressions: [{ key: "env", operator: "Exists" }],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
      });

      it("should return false when key does not exist", () => {
        const resourceLabels = { tier: "frontend" };
        const selector: LabelSelectorValue = {
          matchExpressions: [{ key: "env", operator: "Exists" }],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
      });
    });

    describe("DoesNotExist operator", () => {
      it("should return true when key does not exist", () => {
        const resourceLabels = { tier: "frontend" };
        const selector: LabelSelectorValue = {
          matchExpressions: [{ key: "env", operator: "DoesNotExist" }],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
      });

      it("should return false when key exists", () => {
        const resourceLabels = { env: "production" };
        const selector: LabelSelectorValue = {
          matchExpressions: [{ key: "env", operator: "DoesNotExist" }],
        };

        expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
      });
    });

    it("should require all expressions to match (AND logic)", () => {
      const resourceLabels = { env: "production", tier: "backend" };
      const selector: LabelSelectorValue = {
        matchExpressions: [
          { key: "env", operator: "In", values: ["production"] },
          { key: "tier", operator: "In", values: ["frontend"] },
        ],
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });
  });

  describe("combined matchLabels and matchExpressions", () => {
    it("should require both matchLabels and matchExpressions to pass", () => {
      const resourceLabels = { env: "production", tier: "frontend", version: "v2" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
        matchExpressions: [
          { key: "version", operator: "In", values: ["v1", "v2"] },
        ],
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
    });

    it("should fail if matchLabels fail even if matchExpressions pass", () => {
      const resourceLabels = { env: "staging", version: "v2" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
        matchExpressions: [
          { key: "version", operator: "In", values: ["v1", "v2"] },
        ],
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });

    it("should fail if matchExpressions fail even if matchLabels pass", () => {
      const resourceLabels = { env: "production", version: "v3" };
      const selector: LabelSelectorValue = {
        matchLabels: { env: "production" },
        matchExpressions: [
          { key: "version", operator: "In", values: ["v1", "v2"] },
        ],
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });
  });

  describe("edge cases", () => {
    it("should return true for empty selector", () => {
      const resourceLabels = { env: "production" };
      const selector: LabelSelectorValue = {};

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
    });

    it("should return true for undefined selector parts", () => {
      const resourceLabels = { env: "production" };
      const selector: LabelSelectorValue = {
        matchLabels: undefined,
        matchExpressions: undefined,
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
    });

    it("should handle empty values array for In operator", () => {
      const resourceLabels = { env: "production" };
      const selector: LabelSelectorValue = {
        matchExpressions: [{ key: "env", operator: "In", values: [] }],
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(false);
    });

    it("should handle empty values array for NotIn operator", () => {
      const resourceLabels = { env: "production" };
      const selector: LabelSelectorValue = {
        matchExpressions: [{ key: "env", operator: "NotIn", values: [] }],
      };

      expect(matchesLabelSelector(resourceLabels, selector)).toBe(true);
    });
  });
});
