import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

vi.mock("@/hooks/resources", () => ({ useSecrets: vi.fn() }));

import { useSecrets } from "@/hooks/resources";
import { SecretKeySelect, USE_PROVIDER_DEFAULT } from "./secret-key-select";

function setSecrets(secrets: Array<{ name: string; namespace: string; keys: string[] }>) {
  vi.mocked(useSecrets).mockReturnValue({ data: secrets, isLoading: false, error: null } as never);
}

describe("SecretKeySelect", () => {
  it("lists secret names and the selected secret's keys plus the default option", async () => {
    setSecrets([{ name: "anthropic-creds", namespace: "ns", keys: ["ANTHROPIC_API_KEY", "alt"] }]);
    render(
      <SecretKeySelect
        namespace="ns" secretName="anthropic-creds" secretKey=""
        onSecretNameChange={() => {}} onSecretKeyChange={() => {}} idPrefix="cred"
      />
    );
    // Secret option present (shown as trigger value)
    expect(screen.getByText("anthropic-creds")).toBeTruthy();
    // Open the key dropdown to reveal portal-rendered options
    const keySelect = screen.getByTestId("cred-key-select");
    fireEvent.click(keySelect);
    // Options are now rendered in portal — query document-level
    expect(await screen.findByRole("option", { name: /use provider default/i })).toBeTruthy();
    expect(screen.getByRole("option", { name: "ANTHROPIC_API_KEY" })).toBeTruthy();
    // Sanity-check the sentinel is exported as a string constant
    expect(USE_PROVIDER_DEFAULT).toBe("__default__");
  });

  it("shows an empty state with an add-secret action when no secrets exist", () => {
    setSecrets([]);
    const onAdd = vi.fn();
    render(
      <SecretKeySelect
        namespace="ns" secretName="" secretKey=""
        onSecretNameChange={() => {}} onSecretKeyChange={() => {}} idPrefix="cred" onAddSecret={onAdd}
      />
    );
    expect(screen.getByText(/no credential secrets/i)).toBeTruthy();
    screen.getByRole("button", { name: /add credential secret/i }).click();
    expect(onAdd).toHaveBeenCalled();
  });
});
