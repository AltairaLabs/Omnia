/**
 * Tests for arena-template.ts helper functions
 */
import { describe, it, expect } from "vitest";
import {
  getTemplateDisplayName,
  getDefaultVariableValues,
  validateVariables,
  getTemplateCategories,
  getTemplateTags,
  filterTemplates,
  type TemplateMetadata,
  type TemplateVariable,
} from "./arena-template";

describe("arena-template helpers", () => {
  // ==========================================================================
  // getTemplateDisplayName
  // ==========================================================================
  describe("getTemplateDisplayName", () => {
    it("returns displayName when available", () => {
      const template: TemplateMetadata = {
        name: "basic-chatbot",
        displayName: "Basic Chatbot",
        path: "templates/basic-chatbot",
      };
      expect(getTemplateDisplayName(template)).toBe("Basic Chatbot");
    });

    it("falls back to name when displayName is not set", () => {
      const template: TemplateMetadata = {
        name: "basic-chatbot",
        path: "templates/basic-chatbot",
      };
      expect(getTemplateDisplayName(template)).toBe("basic-chatbot");
    });

    it("falls back to name when displayName is empty", () => {
      const template: TemplateMetadata = {
        name: "basic-chatbot",
        displayName: "",
        path: "templates/basic-chatbot",
      };
      expect(getTemplateDisplayName(template)).toBe("basic-chatbot");
    });
  });

  // ==========================================================================
  // getDefaultVariableValues
  // ==========================================================================
  describe("getDefaultVariableValues", () => {
    it("returns empty object for empty variables", () => {
      expect(getDefaultVariableValues([])).toEqual({});
    });

    it("returns string defaults", () => {
      const variables: TemplateVariable[] = [
        { name: "projectName", type: "string", default: "my-project" },
      ];
      expect(getDefaultVariableValues(variables)).toEqual({
        projectName: "my-project",
      });
    });

    it("returns number defaults parsed as numbers", () => {
      const variables: TemplateVariable[] = [
        { name: "temperature", type: "number", default: "0.7" },
        { name: "maxTokens", type: "number", default: "100" },
      ];
      expect(getDefaultVariableValues(variables)).toEqual({
        temperature: 0.7,
        maxTokens: 100,
      });
    });

    it("returns boolean defaults parsed as booleans", () => {
      const variables: TemplateVariable[] = [
        { name: "enabled", type: "boolean", default: "true" },
        { name: "debug", type: "boolean", default: "false" },
      ];
      expect(getDefaultVariableValues(variables)).toEqual({
        enabled: true,
        debug: false,
      });
    });

    it("returns enum defaults as strings", () => {
      const variables: TemplateVariable[] = [
        { name: "provider", type: "enum", default: "openai", options: ["openai", "anthropic"] },
      ];
      expect(getDefaultVariableValues(variables)).toEqual({
        provider: "openai",
      });
    });

    it("skips variables without defaults", () => {
      const variables: TemplateVariable[] = [
        { name: "required", type: "string", required: true },
        { name: "optional", type: "string", default: "value" },
      ];
      expect(getDefaultVariableValues(variables)).toEqual({
        optional: "value",
      });
    });

    it("handles invalid number defaults gracefully", () => {
      const variables: TemplateVariable[] = [
        { name: "num", type: "number", default: "invalid" },
      ];
      // parseFloat("invalid") returns NaN, which is falsy, so it becomes 0
      const result = getDefaultVariableValues(variables);
      expect(result.num).toBe(0);
    });
  });

  // ==========================================================================
  // validateVariables
  // ==========================================================================
  describe("validateVariables", () => {
    it("returns empty array for valid values", () => {
      const variables: TemplateVariable[] = [
        { name: "name", type: "string", required: true },
      ];
      const values = { name: "test" };
      expect(validateVariables(variables, values)).toEqual([]);
    });

    it("returns error for missing required variable", () => {
      const variables: TemplateVariable[] = [
        { name: "name", type: "string", required: true },
      ];
      const values = {};
      const errors = validateVariables(variables, values);
      expect(errors).toHaveLength(1);
      expect(errors[0].variable).toBe("name");
      expect(errors[0].message).toContain("required");
    });

    it("returns error for empty required variable", () => {
      const variables: TemplateVariable[] = [
        { name: "name", type: "string", required: true },
      ];
      const values = { name: "" };
      const errors = validateVariables(variables, values);
      expect(errors).toHaveLength(1);
      expect(errors[0].variable).toBe("name");
    });

    it("skips validation for optional missing variables", () => {
      const variables: TemplateVariable[] = [
        { name: "optional", type: "string" },
      ];
      const values = {};
      expect(validateVariables(variables, values)).toEqual([]);
    });

    it("validates string pattern", () => {
      const variables: TemplateVariable[] = [
        { name: "name", type: "string", pattern: "^[a-z]+$" },
      ];

      expect(validateVariables(variables, { name: "valid" })).toEqual([]);

      const errors = validateVariables(variables, { name: "Invalid123" });
      expect(errors).toHaveLength(1);
      expect(errors[0].message).toContain("pattern");
    });

    it("validates number type", () => {
      const variables: TemplateVariable[] = [
        { name: "count", type: "number" },
      ];

      expect(validateVariables(variables, { count: 5 })).toEqual([]);
      expect(validateVariables(variables, { count: "10" })).toEqual([]);

      const errors = validateVariables(variables, { count: "not-a-number" });
      expect(errors).toHaveLength(1);
      expect(errors[0].message).toContain("number");
    });

    it("validates number min constraint", () => {
      const variables: TemplateVariable[] = [
        { name: "temp", type: "number", min: "0" },
      ];

      expect(validateVariables(variables, { temp: 0.5 })).toEqual([]);

      const errors = validateVariables(variables, { temp: -1 });
      expect(errors).toHaveLength(1);
      expect(errors[0].message).toContain(">=");
    });

    it("validates number max constraint", () => {
      const variables: TemplateVariable[] = [
        { name: "temp", type: "number", max: "2" },
      ];

      expect(validateVariables(variables, { temp: 1.5 })).toEqual([]);

      const errors = validateVariables(variables, { temp: 3 });
      expect(errors).toHaveLength(1);
      expect(errors[0].message).toContain("<=");
    });

    it("allows any boolean values without validation", () => {
      // Note: Current implementation does not validate boolean types
      // All values pass through without type checking
      const variables: TemplateVariable[] = [
        { name: "enabled", type: "boolean" },
      ];

      expect(validateVariables(variables, { enabled: true })).toEqual([]);
      expect(validateVariables(variables, { enabled: false })).toEqual([]);
      expect(validateVariables(variables, { enabled: "true" })).toEqual([]);
      expect(validateVariables(variables, { enabled: "false" })).toEqual([]);
      // Boolean validation is not implemented, so "yes" passes without error
      expect(validateVariables(variables, { enabled: "yes" })).toEqual([]);
    });

    it("validates enum type", () => {
      const variables: TemplateVariable[] = [
        { name: "provider", type: "enum", options: ["openai", "anthropic"] },
      ];

      expect(validateVariables(variables, { provider: "openai" })).toEqual([]);

      const errors = validateVariables(variables, { provider: "invalid" });
      expect(errors).toHaveLength(1);
      expect(errors[0].message).toContain("one of");
    });

    it("collects multiple validation errors", () => {
      const variables: TemplateVariable[] = [
        { name: "name", type: "string", required: true },
        { name: "count", type: "number", required: true },
      ];
      const values = {};
      const errors = validateVariables(variables, values);
      expect(errors).toHaveLength(2);
    });
  });

  // ==========================================================================
  // getTemplateCategories
  // ==========================================================================
  describe("getTemplateCategories", () => {
    it("returns empty array for empty templates", () => {
      expect(getTemplateCategories([])).toEqual([]);
    });

    it("returns unique categories sorted", () => {
      const templates: TemplateMetadata[] = [
        { name: "t1", category: "chatbot", path: "p1" },
        { name: "t2", category: "agent", path: "p2" },
        { name: "t3", category: "chatbot", path: "p3" },
      ];
      expect(getTemplateCategories(templates)).toEqual(["agent", "chatbot"]);
    });

    it("skips templates without categories", () => {
      const templates: TemplateMetadata[] = [
        { name: "t1", category: "chatbot", path: "p1" },
        { name: "t2", path: "p2" },
      ];
      expect(getTemplateCategories(templates)).toEqual(["chatbot"]);
    });
  });

  // ==========================================================================
  // getTemplateTags
  // ==========================================================================
  describe("getTemplateTags", () => {
    it("returns empty array for empty templates", () => {
      expect(getTemplateTags([])).toEqual([]);
    });

    it("returns unique tags sorted", () => {
      const templates: TemplateMetadata[] = [
        { name: "t1", tags: ["mock", "beginner"], path: "p1" },
        { name: "t2", tags: ["openai", "beginner"], path: "p2" },
      ];
      expect(getTemplateTags(templates)).toEqual(["beginner", "mock", "openai"]);
    });

    it("skips templates without tags", () => {
      const templates: TemplateMetadata[] = [
        { name: "t1", tags: ["test"], path: "p1" },
        { name: "t2", path: "p2" },
      ];
      expect(getTemplateTags(templates)).toEqual(["test"]);
    });
  });

  // ==========================================================================
  // filterTemplates
  // ==========================================================================
  describe("filterTemplates", () => {
    const templates: TemplateMetadata[] = [
      {
        name: "basic-chatbot",
        displayName: "Basic Chatbot",
        description: "A simple chatbot",
        category: "chatbot",
        tags: ["mock", "beginner"],
        path: "p1"
      },
      {
        name: "advanced-agent",
        displayName: "Advanced Agent",
        description: "A complex agent",
        category: "agent",
        tags: ["openai", "advanced"],
        path: "p2"
      },
      {
        name: "mock-assistant",
        displayName: "Mock Assistant",
        description: "Testing assistant",
        category: "assistant",
        tags: ["mock", "testing"],
        path: "p3"
      },
    ];

    it("returns all templates with empty filters", () => {
      expect(filterTemplates(templates, {})).toEqual(templates);
    });

    it("filters by category", () => {
      const filtered = filterTemplates(templates, { category: "chatbot" });
      expect(filtered).toHaveLength(1);
      expect(filtered[0].name).toBe("basic-chatbot");
    });

    it("filters by category case-insensitive", () => {
      const filtered = filterTemplates(templates, { category: "CHATBOT" });
      expect(filtered).toHaveLength(1);
    });

    it("filters by tags", () => {
      const filtered = filterTemplates(templates, { tags: ["mock"] });
      expect(filtered).toHaveLength(2);
      expect(filtered.map(t => t.name)).toContain("basic-chatbot");
      expect(filtered.map(t => t.name)).toContain("mock-assistant");
    });

    it("filters by multiple tags (OR logic)", () => {
      const filtered = filterTemplates(templates, { tags: ["openai", "testing"] });
      expect(filtered).toHaveLength(2);
    });

    it("filters by search query in name", () => {
      const filtered = filterTemplates(templates, { search: "chatbot" });
      expect(filtered).toHaveLength(1);
      expect(filtered[0].name).toBe("basic-chatbot");
    });

    it("filters by search query in displayName", () => {
      const filtered = filterTemplates(templates, { search: "Advanced" });
      expect(filtered).toHaveLength(1);
      expect(filtered[0].name).toBe("advanced-agent");
    });

    it("filters by search query in description", () => {
      const filtered = filterTemplates(templates, { search: "simple" });
      expect(filtered).toHaveLength(1);
      expect(filtered[0].name).toBe("basic-chatbot");
    });

    it("combines multiple filters", () => {
      const filtered = filterTemplates(templates, {
        category: "chatbot",
        tags: ["mock"],
        search: "basic"
      });
      expect(filtered).toHaveLength(1);
      expect(filtered[0].name).toBe("basic-chatbot");
    });

    it("returns empty for no matches", () => {
      const filtered = filterTemplates(templates, { category: "nonexistent" });
      expect(filtered).toEqual([]);
    });
  });
});
