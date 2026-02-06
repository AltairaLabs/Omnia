import { describe, it, expect } from "vitest";
import { getFileType, getFileTypeLabel, type FileType } from "./arena-project";

describe("arena-project types", () => {
  describe("getFileType", () => {
    it("should return 'arena' for .arena.yaml files", () => {
      expect(getFileType("config.arena.yaml")).toBe("arena");
      expect(getFileType("test.arena.yml")).toBe("arena");
      expect(getFileType("path/to/file.arena.yaml")).toBe("arena");
    });

    it("should return 'prompt' for .prompt.yaml files", () => {
      expect(getFileType("system.prompt.yaml")).toBe("prompt");
      expect(getFileType("user.prompt.yml")).toBe("prompt");
    });

    it("should return 'provider' for .provider.yaml files", () => {
      expect(getFileType("openai.provider.yaml")).toBe("provider");
      expect(getFileType("anthropic.provider.yml")).toBe("provider");
    });

    it("should return 'scenario' for .scenario.yaml files", () => {
      expect(getFileType("test.scenario.yaml")).toBe("scenario");
      expect(getFileType("benchmark.scenario.yml")).toBe("scenario");
    });

    it("should return 'tool' for .tool.yaml files", () => {
      expect(getFileType("search.tool.yaml")).toBe("tool");
      expect(getFileType("calculator.tool.yml")).toBe("tool");
    });

    it("should return 'persona' for .persona.yaml files", () => {
      expect(getFileType("assistant.persona.yaml")).toBe("persona");
      expect(getFileType("expert.persona.yml")).toBe("persona");
    });

    it("should return 'yaml' for generic yaml files", () => {
      expect(getFileType("config.yaml")).toBe("yaml");
      expect(getFileType("settings.yml")).toBe("yaml");
    });

    it("should return 'json' for json files", () => {
      expect(getFileType("package.json")).toBe("json");
      expect(getFileType("data.JSON")).toBe("json");
    });

    it("should return 'markdown' for markdown files", () => {
      expect(getFileType("README.md")).toBe("markdown");
      expect(getFileType("docs.markdown")).toBe("markdown");
    });

    it("should return 'other' for unknown file types", () => {
      expect(getFileType("script.js")).toBe("other");
      expect(getFileType("image.png")).toBe("other");
      expect(getFileType("noextension")).toBe("other");
    });

    it("should be case-insensitive", () => {
      expect(getFileType("CONFIG.ARENA.YAML")).toBe("arena");
      expect(getFileType("Test.Prompt.YML")).toBe("prompt");
      expect(getFileType("FILE.MD")).toBe("markdown");
    });
  });

  describe("getFileTypeLabel", () => {
    it("should return correct labels for all file types", () => {
      const expectedLabels: Record<FileType, string> = {
        arena: "Arena Config",
        prompt: "Prompt",
        provider: "Provider",
        scenario: "Scenario",
        tool: "Tool",
        persona: "Persona",
        yaml: "YAML",
        json: "JSON",
        markdown: "Markdown",
        other: "File",
      };

      for (const [type, label] of Object.entries(expectedLabels)) {
        expect(getFileTypeLabel(type as FileType)).toBe(label);
      }
    });
  });
});
