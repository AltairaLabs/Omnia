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

  it("calls onSecretKeyChange with empty string when selecting the default sentinel", async () => {
    setSecrets([{ name: "anthropic-creds", namespace: "ns", keys: ["ANTHROPIC_API_KEY", "alt"] }]);
    const onKeyChange = vi.fn();
    render(
      <SecretKeySelect
        namespace="ns" secretName="anthropic-creds" secretKey="ANTHROPIC_API_KEY"
        onSecretNameChange={() => {}} onSecretKeyChange={onKeyChange} idPrefix="cred"
      />
    );
    // Open the key dropdown
    const keySelect = screen.getByTestId("cred-key-select");
    fireEvent.click(keySelect);
    // Click the default sentinel option
    const defaultOption = await screen.findByRole("option", { name: /use provider default/i });
    fireEvent.click(defaultOption);
    // Should call with "" (empty string), not the sentinel
    expect(onKeyChange).toHaveBeenCalledWith("");
  });

  it("calls onSecretKeyChange with the key value when selecting a real key", async () => {
    setSecrets([{ name: "anthropic-creds", namespace: "ns", keys: ["ANTHROPIC_API_KEY", "alt"] }]);
    const onKeyChange = vi.fn();
    render(
      <SecretKeySelect
        namespace="ns" secretName="anthropic-creds" secretKey=""
        onSecretNameChange={() => {}} onSecretKeyChange={onKeyChange} idPrefix="cred"
      />
    );
    // Open the key dropdown
    const keySelect = screen.getByTestId("cred-key-select");
    fireEvent.click(keySelect);
    // Click a real key option
    const keyOption = await screen.findByRole("option", { name: "ANTHROPIC_API_KEY" });
    fireEvent.click(keyOption);
    // Should call with the key value
    expect(onKeyChange).toHaveBeenCalledWith("ANTHROPIC_API_KEY");
  });

  it("shows add-secret button in the non-empty state and calls onAddSecret when clicked", () => {
    setSecrets([{ name: "anthropic-creds", namespace: "ns", keys: ["ANTHROPIC_API_KEY", "alt"] }]);
    const onAdd = vi.fn();
    render(
      <SecretKeySelect
        namespace="ns" secretName="anthropic-creds" secretKey="ANTHROPIC_API_KEY"
        onSecretNameChange={() => {}} onSecretKeyChange={() => {}} idPrefix="cred" onAddSecret={onAdd}
      />
    );
    // Add-secret button should be present in the non-empty state (secret selection area)
    const addButton = screen.getByRole("button", { name: /add credential secret/i });
    expect(addButton).toBeTruthy();
    // Click it
    addButton.click();
    expect(onAdd).toHaveBeenCalled();
  });

  it("shows '<name> (not found)' when secretName is set but not in list", () => {
    setSecrets([{ name: "other-secret", namespace: "ns", keys: ["KEY"] }]);
    render(
      <SecretKeySelect
        namespace="ns" secretName="missing-secret" secretKey=""
        onSecretNameChange={() => {}} onSecretKeyChange={() => {}} idPrefix="cred"
      />
    );
    expect(screen.getByText("missing-secret (not found)")).toBeTruthy();
  });

  it("shows current secretKey as selectable option when secret not in list and key is set", async () => {
    setSecrets([{ name: "other-secret", namespace: "ns", keys: ["KEY"] }]);
    render(
      <SecretKeySelect
        namespace="ns" secretName="missing-secret" secretKey="MY_KEY"
        onSecretNameChange={() => {}} onSecretKeyChange={() => {}} idPrefix="cred"
      />
    );
    const keySelect = screen.getByTestId("cred-key-select");
    fireEvent.click(keySelect);
    expect(await screen.findByRole("option", { name: "MY_KEY" })).toBeTruthy();
  });
});
