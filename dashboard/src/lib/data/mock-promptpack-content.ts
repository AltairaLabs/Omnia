import type { PromptPackContent } from "./types";

// Demo PromptPack content — a deliberate mix so the workload graph shows more
// than single-agent packs: a workflow state machine, a multi-step composition,
// and plain single-prompt packs. Keyed by pack name; unknown names fall back to
// the single-prompt default.
const TEMPLATE_ENGINE = { version: "v1", syntax: "{{variable}}" };

const SINGLE_PACK_CONTENT: PromptPackContent = {
  id: "mock-prompts",
  name: "Mock Prompts",
  version: "1.0.0",
  description: "Single-prompt pack for demo mode",
  template_engine: TEMPLATE_ENGINE,
  prompts: {
    default: {
      id: "default",
      name: "Default Prompt",
      version: "1.0.0",
      system_template: "You are a helpful AI assistant.",
      parameters: { temperature: 0.7 },
    },
  },
};

const WORKFLOW_PACK_CONTENT: PromptPackContent = {
  id: "support-workflow",
  name: "Support Workflow",
  version: "1.2.0",
  description: "Support triage state machine — classify, then resolve or escalate",
  template_engine: TEMPLATE_ENGINE,
  prompts: {
    triage: { id: "triage", name: "Triage", system_template: "Classify the incoming ticket." },
    resolve: { id: "resolve", name: "Resolve", system_template: "Draft a resolution for the customer." },
    escalate: { id: "escalate", name: "Escalate", system_template: "Summarize and hand off to a human agent." },
  },
  workflow: {
    version: 1,
    entry: "triage",
    states: {
      triage: {
        prompt_task: "triage",
        description: "Classify the ticket",
        on_event: { resolved: "resolve", needs_human: "escalate" },
      },
      resolve: { prompt_task: "resolve", description: "Auto-resolve and reply", terminal: true },
      escalate: { prompt_task: "escalate", description: "Hand off to a human", terminal: true },
    },
  },
};

const COMPOSITION_PACK_CONTENT: PromptPackContent = {
  id: "code-composition",
  name: "Code Composition",
  version: "2.1.0",
  description: "Plan → generate → review + test, run as a composition",
  template_engine: TEMPLATE_ENGINE,
  prompts: {
    plan: { id: "plan", name: "Plan", system_template: "Break the coding task into steps." },
    generate: { id: "generate", name: "Generate", system_template: "Write the code for the plan." },
    review: { id: "review", name: "Review", system_template: "Review the generated code." },
    test: { id: "test", name: "Test", system_template: "Write tests for the code." },
  },
  workflow: {
    version: 1,
    entry: "build",
    states: {
      build: {
        prompt_task: "generate",
        description: "Run the codegen composition",
        orchestration: "composition",
        composition: "codegen",
        terminal: true,
      },
    },
  },
  compositions: {
    codegen: {
      version: 1,
      description: "Plan, generate, then review and test in parallel",
      output: "review",
      steps: [
        { id: "plan", kind: "prompt", prompt_task: "plan", description: "Decompose the task" },
        {
          id: "generate",
          kind: "agent",
          prompt_task: "generate",
          depends_on: ["plan"],
          tools: ["fs_write"],
          description: "Write the code",
        },
        {
          id: "checks",
          kind: "parallel",
          depends_on: ["generate"],
          description: "Review and test together",
          branches: [
            { id: "review", kind: "prompt", prompt_task: "review" },
            { id: "test", kind: "prompt", prompt_task: "test" },
          ],
          reduce: { strategy: "barrier", into: "results" },
        },
      ],
    },
  },
};

const PROMPT_PACK_CONTENT_BY_NAME: Record<string, PromptPackContent> = {
  "support-workflow": WORKFLOW_PACK_CONTENT,
  "code-composition": COMPOSITION_PACK_CONTENT,
  // Also vary the production-namespace packs.
  "support-prompts": WORKFLOW_PACK_CONTENT,
  "code-prompts": COMPOSITION_PACK_CONTENT,
};

/** Resolve demo PromptPack content by name; unknown names fall back to single. */
export function mockPromptPackContent(name: string): PromptPackContent {
  return PROMPT_PACK_CONTENT_BY_NAME[name] ?? SINGLE_PACK_CONTENT;
}
