import { describe, it, expect } from "vitest";
import {
  ARENA_FILE_TYPES,
  getFileTypeConfig,
  generateUniqueBaseName,
  generateFileName,
  generateFileContent,
  type ArenaFileKind,
} from "./file-templates";

describe("file-templates", () => {
  describe("ARENA_FILE_TYPES", () => {
    it("should have all five Arena file types", () => {
      expect(ARENA_FILE_TYPES).toHaveLength(5);
      const kinds = ARENA_FILE_TYPES.map((t) => t.kind);
      expect(kinds).toContain("prompt");
      expect(kinds).toContain("provider");
      expect(kinds).toContain("scenario");
      expect(kinds).toContain("tool");
      expect(kinds).toContain("persona");
    });

    it("should have correct extensions for each type", () => {
      const extensionMap: Record<ArenaFileKind, string> = {
        prompt: ".prompt.yaml",
        provider: ".provider.yaml",
        scenario: ".scenario.yaml",
        tool: ".tool.yaml",
        persona: ".persona.yaml",
      };

      ARENA_FILE_TYPES.forEach((fileType) => {
        expect(fileType.extension).toBe(extensionMap[fileType.kind]);
      });
    });

    it("should have non-empty templates for each type", () => {
      ARENA_FILE_TYPES.forEach((fileType) => {
        expect(fileType.template).toBeTruthy();
        expect(fileType.template.length).toBeGreaterThan(0);
      });
    });
  });

  describe("getFileTypeConfig", () => {
    it("should return config for valid file kind", () => {
      const config = getFileTypeConfig("prompt");
      expect(config).toBeDefined();
      expect(config?.kind).toBe("prompt");
      expect(config?.extension).toBe(".prompt.yaml");
    });

    it("should return undefined for invalid file kind", () => {
      // @ts-expect-error - testing invalid input
      const config = getFileTypeConfig("invalid");
      expect(config).toBeUndefined();
    });
  });

  describe("generateUniqueBaseName", () => {
    it("should generate base name for each file type", () => {
      expect(generateUniqueBaseName("prompt")).toBe("new-prompt");
      expect(generateUniqueBaseName("provider")).toBe("new-provider");
      expect(generateUniqueBaseName("scenario")).toBe("new-scenario");
      expect(generateUniqueBaseName("tool")).toBe("new-tool");
      expect(generateUniqueBaseName("persona")).toBe("new-persona");
    });
  });

  describe("generateFileName", () => {
    it("should generate correct filename with extension", () => {
      expect(generateFileName("my-prompt", "prompt")).toBe("my-prompt.prompt.yaml");
      expect(generateFileName("api-provider", "provider")).toBe("api-provider.provider.yaml");
      expect(generateFileName("test-scenario", "scenario")).toBe("test-scenario.scenario.yaml");
      expect(generateFileName("search-tool", "tool")).toBe("search-tool.tool.yaml");
      expect(generateFileName("assistant", "persona")).toBe("assistant.persona.yaml");
    });

    it("should fallback to .yaml for invalid kind", () => {
      // @ts-expect-error - testing invalid input
      expect(generateFileName("test", "invalid")).toBe("test.yaml");
    });
  });

  describe("generateFileContent", () => {
    it("should replace {{name}} placeholders with the provided name", () => {
      const content = generateFileContent("my-prompt", "prompt");
      expect(content).toContain("name: my-prompt");
      expect(content).not.toContain("{{name}}");
    });

    it("should generate valid YAML structure for prompt", () => {
      const content = generateFileContent("test-prompt", "prompt");
      expect(content).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
      expect(content).toContain("kind: PromptConfig");
      expect(content).toContain("name: test-prompt");
      expect(content).toContain("task_type: general");
      expect(content).toContain("system_template:");
    });

    it("should generate valid YAML structure for provider", () => {
      const content = generateFileContent("test-provider", "provider");
      expect(content).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
      expect(content).toContain("kind: Provider");
      expect(content).toContain("name: test-provider");
      expect(content).toContain("id: test-provider");
      expect(content).toContain("type: openai");
      expect(content).toContain("model: gpt-4o-mini");
    });

    it("should generate valid YAML structure for scenario", () => {
      const content = generateFileContent("test-scenario", "scenario");
      expect(content).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
      expect(content).toContain("kind: Scenario");
      expect(content).toContain("name: test-scenario");
      expect(content).toContain("inputs: {}");
    });

    it("should generate valid YAML structure for tool", () => {
      const content = generateFileContent("test-tool", "tool");
      expect(content).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
      expect(content).toContain("kind: Tool");
      expect(content).toContain("name: test-tool");
      expect(content).toContain("parameters:");
      expect(content).toContain("type: object");
    });

    it("should generate valid YAML structure for persona", () => {
      const content = generateFileContent("test-persona", "persona");
      expect(content).toContain("apiVersion: promptkit.altairalabs.ai/v1alpha1");
      expect(content).toContain("kind: Persona");
      expect(content).toContain("name: test-persona");
      expect(content).toContain("traits: []");
    });

    it("should return empty string for invalid kind", () => {
      // @ts-expect-error - testing invalid input
      const content = generateFileContent("test", "invalid");
      expect(content).toBe("");
    });
  });
});
