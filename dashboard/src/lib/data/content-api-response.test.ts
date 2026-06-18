import { describe, it, expect } from "vitest";

import { ContentApiError } from "./content-api-service";
import { contentErrorResponse } from "./content-api-response";

describe("contentErrorResponse", () => {
  it.each([
    [400, "Bad Request"],
    [403, "Forbidden"],
    [404, "Not Found"],
    [409, "Conflict"],
    [413, "Payload Too Large"],
  ])("maps ContentApiError status %i to %s", async (status, label) => {
    const res = contentErrorResponse(new ContentApiError("nope", status), "fallback");
    expect(res.status).toBe(status);
    expect((await res.json()).error).toBe(label);
  });

  it("maps an unknown ContentApiError status to a 500 label", async () => {
    const res = contentErrorResponse(new ContentApiError("weird", 418), "fallback");
    expect(res.status).toBe(418);
    expect((await res.json()).error).toBe("Internal Server Error");
  });

  it("treats a plain Error as 500 and keeps its message", async () => {
    const res = contentErrorResponse(new Error("boom"), "fallback");
    expect(res.status).toBe(500);
    expect((await res.json()).message).toBe("boom");
  });

  it("uses the fallback message for a non-Error throw", async () => {
    const res = contentErrorResponse("just a string", "fallback message");
    expect(res.status).toBe(500);
    expect((await res.json()).message).toBe("fallback message");
  });
});
