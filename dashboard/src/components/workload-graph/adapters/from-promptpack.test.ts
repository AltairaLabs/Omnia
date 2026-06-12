import { describe, it, expect } from "vitest";
import { promptPackToWorkload } from "./from-promptpack";

describe("promptPackToWorkload", () => {
  it("returns the derived skeleton at definition altitude", () => {
    const model = promptPackToWorkload({
      id: "greeter",
      prompts: { main: { id: "main", name: "Greeter", system_template: "hi" } },
    });
    expect(model.altitude).toBe("definition");
    expect(model.tier).toBe("solo");
    expect(model.nodes).toHaveLength(1);
  });

  it("returns an empty-but-valid model for undefined content", () => {
    const model = promptPackToWorkload(undefined);
    expect(model.tier).toBe("solo");
    expect(model.nodes).toEqual([]);
  });
});
