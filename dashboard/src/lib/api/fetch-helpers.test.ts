import { describe, it, expect } from "vitest";
import { handleMutationResponse } from "./fetch-helpers";

describe("handleMutationResponse", () => {
  it("returns the response when status is ok", async () => {
    const response = new Response("ok", { status: 200 });
    const result = await handleMutationResponse(response, "fallback");
    expect(result).toBe(response);
  });

  it("throws with server error text when response is not ok", async () => {
    const response = new Response("Server error details", { status: 500 });
    await expect(handleMutationResponse(response, "fallback")).rejects.toThrow("Server error details");
  });

  it("throws with fallback message when response body is empty", async () => {
    const response = new Response("", { status: 400 });
    await expect(handleMutationResponse(response, "Something went wrong")).rejects.toThrow("Something went wrong");
  });
});
