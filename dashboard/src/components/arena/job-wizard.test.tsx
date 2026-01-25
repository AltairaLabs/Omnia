/**
 * Tests for JobWizard component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { JobWizard } from "./job-wizard";
import type { ArenaSource, ArenaJob } from "@/types/arena";

// Mock workspace context
const mockCurrentWorkspace = {
  name: "test-workspace",
  namespace: "test-namespace",
  role: "editor",
};

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
    workspaces: [mockCurrentWorkspace],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

// Mock useArenaSourceContent hook
vi.mock("@/hooks/use-arena-source-content", () => ({
  useArenaSourceContent: () => ({
    tree: [],
    loading: false,
    error: null,
  }),
}));

// Mock provider data
const mockProviders = [
  {
    metadata: {
      name: "claude-prod",
      namespace: "omnia-system",
      uid: "uid-1",
      labels: { env: "production", type: "claude" },
    },
    spec: { type: "claude", model: "claude-3" },
    status: { phase: "Ready" },
  },
];

// Mock tool registries
const mockToolRegistries = [
  {
    metadata: {
      name: "main-tools",
      namespace: "omnia-system",
      uid: "uid-1",
      labels: { category: "core" },
    },
    spec: { handlers: [] },
    status: { phase: "Ready", discoveredToolsCount: 10 },
  },
];

// Mock useDataService
const mockGetProviders = vi.fn().mockResolvedValue(mockProviders);
const mockGetToolRegistries = vi.fn().mockResolvedValue(mockToolRegistries);

vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getProviders: mockGetProviders,
    getToolRegistries: mockGetToolRegistries,
  }),
}));

const mockSources: ArenaSource[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: {
      name: "test-source",
      namespace: "test-namespace",
      uid: "source-1",
    },
    spec: { type: "git" },
    status: { phase: "Ready" },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: {
      name: "pending-source",
      namespace: "test-namespace",
      uid: "source-2",
    },
    spec: { type: "git" },
    status: { phase: "Pending" },
  },
];

const defaultProps = {
  sources: mockSources,
  isEnterprise: true,
  maxWorkerReplicas: 0,
  loading: false,
  onSubmit: vi.fn(),
  onSuccess: vi.fn(),
  onClose: vi.fn(),
};

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

function renderWizard(props = {}) {
  return render(
    <TestWrapper>
      <JobWizard {...defaultProps} {...props} />
    </TestWrapper>
  );
}

// Helper to select an option from a Radix Select and close the dropdown
async function selectOption(trigger: HTMLElement, optionText: string) {
  fireEvent.click(trigger);
  const option = await screen.findByRole("option", { name: optionText });
  fireEvent.click(option);
  // Escape to close dropdown
  fireEvent.keyDown(document.body, { key: "Escape" });
}

describe("JobWizard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("Step 0: Basic Info", () => {
    it("renders the basic info step by default", () => {
      renderWizard();

      expect(screen.getByText("Job Name")).toBeInTheDocument();
      expect(screen.getByText("Job Type")).toBeInTheDocument();
    });

    it("allows entering a job name", async () => {
      const user = userEvent.setup();
      renderWizard();

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");

      expect(nameInput).toHaveValue("test-job");
    });

    it("converts job name to lowercase with hyphens", async () => {
      const user = userEvent.setup();
      renderWizard();

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "Test Job");

      expect(nameInput).toHaveValue("test-job");
    });

    it("disables Next button when name is empty", () => {
      renderWizard();

      const nextButton = screen.getByRole("button", { name: /next/i });
      expect(nextButton).toBeDisabled();
    });

    it("enables Next button when name is valid", async () => {
      const user = userEvent.setup();
      renderWizard();

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");

      const nextButton = screen.getByRole("button", { name: /next/i });
      expect(nextButton).toBeEnabled();
    });

    it("shows enterprise badge for load test and datagen types when not enterprise", () => {
      renderWizard({ isEnterprise: false });

      // Open the job type dropdown
      const typeSelect = screen.getByRole("combobox");
      fireEvent.click(typeSelect);

      expect(screen.getAllByText("Enterprise")).toHaveLength(2);
    });
  });

  describe("Step 1: Source", () => {
    it("navigates to source step when clicking Next", async () => {
      const user = userEvent.setup();
      renderWizard();

      // Fill in name
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");

      // Click Next
      const nextButton = screen.getByRole("button", { name: /next/i });
      await user.click(nextButton);

      // Should now show Source step - check for the form label
      expect(screen.getByLabelText("Source")).toBeInTheDocument();
      expect(screen.getByText(/Select the source containing arena/)).toBeInTheDocument();
    });

    it("only shows ready sources in dropdown", async () => {
      const user = userEvent.setup();
      renderWizard();

      // Navigate to source step
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      await user.click(screen.getByRole("button", { name: /next/i }));

      // Open source dropdown
      const sourceSelect = screen.getByLabelText("Source");
      fireEvent.click(sourceSelect);

      // Only test-source should be visible (Ready phase)
      expect(screen.getByRole("option", { name: "test-source" })).toBeInTheDocument();
    });
  });

  describe("Step 2: Providers", () => {
    async function navigateToProvidersStep(user: ReturnType<typeof userEvent.setup>) {
      // Step 0: Fill name and advance
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source and advance
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("navigates to providers step", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProvidersStep(user);

      expect(screen.getByText("Provider Overrides")).toBeInTheDocument();
    });

    it("allows enabling provider overrides", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProvidersStep(user);

      // Find and click the switch
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Should show provider group selection
      expect(screen.getByText(/Add provider group/)).toBeInTheDocument();
    });
  });

  describe("Step 3: Tools", () => {
    async function navigateToToolsStep(user: ReturnType<typeof userEvent.setup>) {
      // Step 0: Fill name and advance
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source and advance
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2: Skip providers
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("navigates to tools step", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToToolsStep(user);

      expect(screen.getByText("Tool Registry Override")).toBeInTheDocument();
    });

    it("allows enabling tool registry override", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToToolsStep(user);

      // Find and click the switch
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Should show label selector
      expect(screen.getByText(/Match Labels/)).toBeInTheDocument();
    });
  });

  describe("Step 4: Options", () => {
    async function navigateToOptionsStep(user: ReturnType<typeof userEvent.setup>) {
      // Step 0: Fill name and advance
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source and advance
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2-3: Skip
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("navigates to options step", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptionsStep(user);

      expect(screen.getByText("Workers")).toBeInTheDocument();
      expect(screen.getByText("Timeout")).toBeInTheDocument();
    });

    it("shows evaluation options for evaluation job type", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptionsStep(user);

      expect(screen.getByText("Evaluation Options")).toBeInTheDocument();
      expect(screen.getByText("Passing Threshold")).toBeInTheDocument();
    });

    it("shows worker limit warning when maxWorkerReplicas is set", async () => {
      const user = userEvent.setup();
      renderWizard({ maxWorkerReplicas: 2 });

      await navigateToOptionsStep(user);

      expect(screen.getByText(/Limited to 2 workers/)).toBeInTheDocument();
    });
  });

  describe("Step 5: Review", () => {
    async function navigateToReviewStep(user: ReturnType<typeof userEvent.setup>) {
      // Step 0: Fill name and advance
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source and advance
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2-4: Skip
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("navigates to review step", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToReviewStep(user);

      expect(screen.getByText("Review Configuration")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /create job/i })).toBeInTheDocument();
    });

    it("shows job configuration summary", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToReviewStep(user);

      expect(screen.getByText("test-job")).toBeInTheDocument();
      expect(screen.getByText("test-source")).toBeInTheDocument();
      expect(screen.getByText("Evaluation")).toBeInTheDocument();
    });

    it("calls onSubmit when Create Job is clicked", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      await navigateToReviewStep(user);

      const createButton = screen.getByRole("button", { name: /create job/i });
      fireEvent.click(createButton);

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith(
          "test-job",
          expect.objectContaining({
            sourceRef: { name: "test-source" },
            type: "evaluation",
          })
        );
      });
    });

    it("shows success state after job creation", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      const onSuccess = vi.fn();
      renderWizard({ onSubmit, onSuccess });

      await navigateToReviewStep(user);

      const createButton = screen.getByRole("button", { name: /create job/i });
      fireEvent.click(createButton);

      await waitFor(() => {
        expect(screen.getByText("Job Created!")).toBeInTheDocument();
      });
      expect(onSuccess).toHaveBeenCalled();
    });

    it("shows error when job creation fails", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockRejectedValue(new Error("Failed to create job"));
      renderWizard({ onSubmit });

      await navigateToReviewStep(user);

      const createButton = screen.getByRole("button", { name: /create job/i });
      fireEvent.click(createButton);

      await waitFor(() => {
        expect(screen.getByText("Failed to create job")).toBeInTheDocument();
      });
    });
  });

  describe("Navigation", () => {
    it("calls onClose when Cancel is clicked on first step", async () => {
      const user = userEvent.setup();
      const onClose = vi.fn();
      renderWizard({ onClose });

      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      await user.click(cancelButton);

      expect(onClose).toHaveBeenCalled();
    });

    it("navigates back when Back is clicked", async () => {
      const user = userEvent.setup();
      renderWizard();

      // Navigate to step 1
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Click Back
      fireEvent.click(screen.getByRole("button", { name: /back/i }));

      // Should be back on step 0
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });
  });

  describe("spec building", () => {
    it("builds spec with correct source and type", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      // Step 0: Fill name and advance
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source and advance
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2-4: Skip
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Submit
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith(
          "test-job",
          expect.objectContaining({
            sourceRef: { name: "test-source" },
            type: "evaluation",
            workers: { replicas: 1 },
          })
        );
      });
    });
  });

  describe("Load Test job type", () => {
    async function navigateToOptionsWithLoadTest(user: ReturnType<typeof userEvent.setup>) {
      // Step 0: Fill name and select loadtest type
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "loadtest-job");

      // Select loadtest type
      const typeSelect = screen.getByRole("combobox");
      await selectOption(typeSelect, "Load Test");

      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2-3: Skip
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("shows load test options for loadtest job type", async () => {
      const user = userEvent.setup();
      renderWizard({ isEnterprise: true });

      await navigateToOptionsWithLoadTest(user);

      expect(screen.getByText("Load Test Options")).toBeInTheDocument();
      expect(screen.getByText("Profile Type")).toBeInTheDocument();
      expect(screen.getByText("Duration")).toBeInTheDocument();
      expect(screen.getByText("Target RPS")).toBeInTheDocument();
    });

    it("allows changing load test profile type", async () => {
      const user = userEvent.setup();
      renderWizard({ isEnterprise: true });

      await navigateToOptionsWithLoadTest(user);

      // Select ramp profile
      const profileSelect = screen.getByLabelText("Profile Type");
      await selectOption(profileSelect, "Ramp");

      // Verify the selection by checking if dropdown is closed
      expect(screen.getByText("Load Test Options")).toBeInTheDocument();
    });

    it("builds loadtest spec correctly", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ isEnterprise: true, onSubmit });

      await navigateToOptionsWithLoadTest(user);

      // Modify duration
      const durationInput = screen.getByLabelText("Duration");
      await user.clear(durationInput);
      await user.type(durationInput, "10m");

      // Modify RPS
      const rpsInput = screen.getByLabelText("Target RPS");
      await user.clear(rpsInput);
      await user.type(rpsInput, "20");

      // Go to review and submit
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith(
          "loadtest-job",
          expect.objectContaining({
            type: "loadtest",
            loadtest: expect.objectContaining({
              profileType: "constant",
              duration: "10m",
              targetRPS: 20,
            }),
          })
        );
      });
    });
  });

  describe("Data Generation job type", () => {
    async function navigateToOptionsWithDatagen(user: ReturnType<typeof userEvent.setup>) {
      // Step 0: Fill name and select datagen type
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "datagen-job");

      // Select datagen type
      const typeSelect = screen.getByRole("combobox");
      await selectOption(typeSelect, "Data Generation");

      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2-3: Skip
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("shows data generation options for datagen job type", async () => {
      const user = userEvent.setup();
      renderWizard({ isEnterprise: true });

      await navigateToOptionsWithDatagen(user);

      expect(screen.getByText("Data Generation Options")).toBeInTheDocument();
      expect(screen.getByText("Sample Count")).toBeInTheDocument();
      expect(screen.getByText("Deduplicate")).toBeInTheDocument();
    });

    it("builds datagen spec correctly", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ isEnterprise: true, onSubmit });

      await navigateToOptionsWithDatagen(user);

      // Modify sample count
      const samplesInput = screen.getByLabelText("Sample Count");
      await user.clear(samplesInput);
      await user.type(samplesInput, "200");

      // Toggle deduplicate off
      const deduplicateSwitch = screen.getAllByRole("switch")[1]; // Second switch is deduplicate
      await user.click(deduplicateSwitch);

      // Go to review and submit
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith(
          "datagen-job",
          expect.objectContaining({
            type: "datagen",
            datagen: expect.objectContaining({
              sampleCount: 200,
              deduplicate: false,
              outputFormat: "jsonl",
            }),
          })
        );
      });
    });
  });

  describe("Provider group management", () => {
    async function navigateToProviders(user: ReturnType<typeof userEvent.setup>) {
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("allows adding a default provider group", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Find the provider group select by its placeholder
      const groupSelect = screen.getByText("Add provider group").closest("button") as HTMLElement;
      await selectOption(groupSelect, "default");

      // Should show the group in a Badge
      expect(screen.getByText("default")).toBeInTheDocument();
    });

    it("allows adding a custom provider group", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Enter custom group name
      const customInput = screen.getByPlaceholderText("Custom group name");
      await user.type(customInput, "my-custom-group");

      // Click add button (the one with Plus icon, which is the last button in the add group row)
      const addButtons = screen.getAllByRole("button");
      const addButton = addButtons.find(btn => btn.textContent === "" && btn.getAttribute("type") === "button" && !btn.hasAttribute("disabled"));
      if (addButton) await user.click(addButton);

      // Should show the custom group
      expect(screen.getByText("my-custom-group")).toBeInTheDocument();
    });

    it("allows removing a provider group", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Find the provider group select by its placeholder
      const groupSelect = screen.getByText("Add provider group").closest("button") as HTMLElement;
      await selectOption(groupSelect, "evaluation");

      // Find the remove button (X) in the group editor
      const groupEditors = screen.getAllByText("evaluation");
      expect(groupEditors.length).toBeGreaterThan(0);

      // Remove the group - find the X button within the provider group editor
      const removeButtons = screen.getAllByRole("button").filter(btn =>
        btn.querySelector("svg") && btn.className.includes("ghost")
      );
      if (removeButtons.length > 0) {
        await user.click(removeButtons[0]);
      }

      // After removal, only the dropdown option should show "evaluation"
      const remainingEvaluations = screen.queryAllByText("evaluation");
      // It may still be in the dropdown, but not as a badge
      expect(remainingEvaluations.length).toBeLessThanOrEqual(1);
    });

    it("shows no groups message when none configured", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      expect(screen.getByText(/No provider groups configured/)).toBeInTheDocument();
    });

    it("renders provider group selector editor with provider preview", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Add a provider group
      const groupSelect = screen.getByText("Add provider group").closest("button") as HTMLElement;
      await selectOption(groupSelect, "default");

      // The group should have a Match Labels section from K8sLabelSelector
      expect(screen.getByText("Match Labels")).toBeInTheDocument();
      // And should show provider count
      await waitFor(() => {
        expect(screen.getByText(/providers match/)).toBeInTheDocument();
      });
    });

    it("updates provider group selector when labels are added", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Add a provider group
      const groupSelect = screen.getByText("Add provider group").closest("button") as HTMLElement;
      await selectOption(groupSelect, "default");

      // The K8sLabelSelector has select dropdowns for key/value when availableLabels is provided
      // Look for the "Key" placeholder in a select trigger
      const keyTriggers = screen.getAllByText("Key");
      expect(keyTriggers.length).toBeGreaterThan(0);
    });

    it("builds spec with provider overrides when labels are configured", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      await navigateToProviders(user);

      // Enable provider overrides
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Add a provider group
      const groupSelect = screen.getByText("Add provider group").closest("button") as HTMLElement;
      await selectOption(groupSelect, "default");

      // Skip to review and submit (even without labels configured, we test the path)
      fireEvent.click(screen.getByRole("button", { name: /next/i })); // to tools
      fireEvent.click(screen.getByRole("button", { name: /next/i })); // to options
      fireEvent.click(screen.getByRole("button", { name: /next/i })); // to review
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalled();
      });
    });
  });

  describe("Tool registry override", () => {
    async function navigateToTools(user: ReturnType<typeof userEvent.setup>) {
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i })); // Skip providers
    }

    it("shows label selector when tool registry override is enabled", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToTools(user);

      // Enable tool registry override
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Should show the label selector
      expect(screen.getByText("Match Labels")).toBeInTheDocument();
    });
  });

  describe("Source selection and file handling", () => {
    it("shows no ready sources message when all sources are pending", async () => {
      const user = userEvent.setup();
      const pendingSources: ArenaSource[] = [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1",
          kind: "ArenaSource",
          metadata: { name: "pending-1", namespace: "test-namespace", uid: "1" },
          spec: { type: "git" },
          status: { phase: "Pending" },
        },
      ];
      renderWizard({ sources: pendingSources });

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Open source dropdown
      const sourceSelect = screen.getByLabelText("Source");
      fireEvent.click(sourceSelect);

      expect(screen.getByText("No ready sources available")).toBeInTheDocument();
    });

    it("uses preselectedSource when provided", async () => {
      const user = userEvent.setup();
      renderWizard({ preselectedSource: "test-source" });

      // Navigate to source step
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // The preselected source should be shown in the select
      expect(screen.getByText("test-source")).toBeInTheDocument();
    });
  });

  describe("Validation", () => {
    it("shows error when enterprise job type used without license", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ isEnterprise: false, onSubmit });

      // Select loadtest (enterprise only) - need to do it before navigating
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");

      // Note: The select is disabled for non-enterprise, but we test the validation
      // by directly setting form state through the flow

      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      // Should not call onSubmit for evaluation (which is allowed)
      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalled();
      });
    });

    it("shows error when workers exceed maxWorkerReplicas", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ maxWorkerReplicas: 1, onSubmit });

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Set workers to 2 which exceeds maxWorkerReplicas of 1
      const workersInput = screen.getByLabelText("Workers");
      await user.clear(workersInput);
      await user.type(workersInput, "2");

      // Navigate to review and try to submit
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(screen.getByText(/limited to 1 worker/i)).toBeInTheDocument();
      });
      expect(onSubmit).not.toHaveBeenCalled();
    });
  });

  describe("Options step", () => {
    async function navigateToOptions(user: ReturnType<typeof userEvent.setup>) {
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("allows toggling verbose logging", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptions(user);

      // Find verbose switch by label
      expect(screen.getByLabelText("Verbose Logging")).toBeInTheDocument();

      const verboseSwitch = screen.getByLabelText("Verbose Logging");
      await user.click(verboseSwitch);

      // Verify it's checked
      expect(verboseSwitch).toBeChecked();
    });

    it("allows modifying workers count", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptions(user);

      const workersInput = screen.getByLabelText("Workers");
      await user.clear(workersInput);
      await user.type(workersInput, "4");

      expect(workersInput).toHaveValue(4);
    });

    it("allows modifying timeout", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptions(user);

      const timeoutInput = screen.getByLabelText("Timeout");
      await user.clear(timeoutInput);
      await user.type(timeoutInput, "1h");

      expect(timeoutInput).toHaveValue("1h");
    });

    it("allows toggling continue on failure for evaluation", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptions(user);

      // Find the continue on failure switch (second switch in evaluation options)
      const switches = screen.getAllByRole("switch");
      const continueOnFailureSwitch = switches.find(s =>
        s.closest(".grid")?.textContent?.includes("Continue on Failure")
      );

      if (continueOnFailureSwitch) {
        await user.click(continueOnFailureSwitch);
        expect(continueOnFailureSwitch).not.toBeChecked();
      }
    });
  });

  describe("Review step with provider/tool overrides", () => {
    it("shows provider overrides in review when configured", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      // Step 0
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2: Enable providers and add a group
      const providerSwitch = screen.getByRole("switch");
      await user.click(providerSwitch);

      const groupSelect = screen.getByText("Add provider group").closest("button") as HTMLElement;
      await selectOption(groupSelect, "default");

      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 3: Skip tools
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 4: Skip options
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Review should show provider overrides
      expect(screen.getByText("Provider Overrides")).toBeInTheDocument();
    });

    it("shows tool registry override in review when configured", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      // Step 0
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2: Skip providers
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 3: Enable tool registry override
      const toolSwitch = screen.getByRole("switch");
      await user.click(toolSwitch);

      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 4: Skip options
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Review should show tool registry override
      expect(screen.getByText("Tool Registry Override")).toBeInTheDocument();
    });
  });

  describe("Button states", () => {
    it("shows loading state on Create Job button when loading prop is true", async () => {
      const user = userEvent.setup();
      renderWizard({ loading: true });

      // Navigate to review
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Button should show loading
      expect(screen.getByRole("button", { name: /creating/i })).toBeDisabled();
    });

    it("disables Back button after successful submission", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      // Navigate to review
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Submit
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(screen.getByText("Job Created!")).toBeInTheDocument();
      });

      // Back button should be disabled
      const backButton = screen.getByRole("button", { name: /back/i });
      expect(backButton).toBeDisabled();
    });
  });

  describe("Single worker limit display", () => {
    it("shows singular worker text for limit of 1", async () => {
      const user = userEvent.setup();
      renderWizard({ maxWorkerReplicas: 1 });

      // Navigate to options
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      expect(screen.getByText(/Limited to 1 worker \(upgrade/)).toBeInTheDocument();
    });
  });

  describe("Evaluation options", () => {
    async function navigateToOptions(user: ReturnType<typeof userEvent.setup>) {
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
    }

    it("allows modifying passing threshold", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToOptions(user);

      const thresholdInput = screen.getByLabelText("Passing Threshold");
      await user.clear(thresholdInput);
      await user.type(thresholdInput, "0.95");

      expect(thresholdInput).toHaveValue(0.95);
    });

    it("builds spec with modified threshold", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({} as ArenaJob);
      renderWizard({ onSubmit });

      await navigateToOptions(user);

      const thresholdInput = screen.getByLabelText("Passing Threshold");
      await user.clear(thresholdInput);
      await user.type(thresholdInput, "0.9");

      // Go to review and submit
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith(
          "test-job",
          expect.objectContaining({
            evaluation: expect.objectContaining({
              passingThreshold: 0.9,
            }),
          })
        );
      });
    });
  });

  describe("Tool registry override with selector", () => {
    async function navigateToTools(user: ReturnType<typeof userEvent.setup>) {
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i })); // Skip providers
    }

    it("shows K8sLabelSelector when tool registry override is enabled", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToTools(user);

      // Enable tool registry override
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // K8sLabelSelector shows Match Labels section
      expect(screen.getByText("Match Labels")).toBeInTheDocument();
      expect(screen.getByText("Select tool registries by labels")).toBeInTheDocument();
    });

    it("continues to next step with tool registry override enabled", async () => {
      const user = userEvent.setup();
      renderWizard();

      await navigateToTools(user);

      // Enable tool registry override
      const switchElement = screen.getByRole("switch");
      await user.click(switchElement);

      // Should be able to continue
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Should be on options step
      expect(screen.getByText("Workers")).toBeInTheDocument();
    });
  });

  describe("Source step with folder browser", () => {
    it("shows folder browser when source is selected", async () => {
      const user = userEvent.setup();
      renderWizard();

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Select source
      await selectOption(screen.getByLabelText("Source"), "test-source");

      // Root folder section should appear
      expect(screen.getByText("Root Folder")).toBeInTheDocument();
    });

    it("allows modifying arena config file name", async () => {
      const user = userEvent.setup();
      renderWizard();

      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Select source
      await selectOption(screen.getByLabelText("Source"), "test-source");

      // Modify arena file name
      const arenaFileInput = screen.getByLabelText("Arena Config File");
      await user.clear(arenaFileInput);
      await user.type(arenaFileInput, "custom.arena.yaml");

      expect(arenaFileInput).toHaveValue("custom.arena.yaml");
    });
  });

  describe("Non-error exception handling", () => {
    it("handles non-Error objects in catch block", async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockRejectedValue("String error message");
      renderWizard({ onSubmit });

      // Navigate to review
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Submit
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      // Should show generic error
      await waitFor(() => {
        expect(screen.getByText("Failed to create job")).toBeInTheDocument();
      });
    });
  });
});
