/**
 * Tests for the extracted API keys table presentational components
 * (KeyExpiration, ApiKeysContent). The container-level behaviour is covered
 * by api-keys-section.test.tsx; these unit tests pin the branch logic of the
 * presentational pieces directly — notably KeyExpiration's "Never" path, which
 * the integration fixtures don't exercise.
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { KeyExpiration, ApiKeysContent, type ApiKeyInfo } from "./api-keys-table";

function makeKey(overrides: Partial<ApiKeyInfo> = {}): ApiKeyInfo {
  return {
    id: "key-1",
    name: "Test Key",
    keyPrefix: "omnia_***abc",
    role: "admin",
    expiresAt: null,
    createdAt: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    lastUsedAt: null,
    isExpired: false,
    ...overrides,
  };
}

describe("KeyExpiration", () => {
  it("renders an Expired badge when isExpired", () => {
    render(<KeyExpiration isExpired={true} expiresAt={null} />);
    expect(screen.getByText("Expired")).toBeInTheDocument();
  });

  it("renders a relative time when an expiry date is set", () => {
    const future = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString();
    render(<KeyExpiration isExpired={false} expiresAt={future} />);
    expect(screen.getByText(/in \d+ days?|in about/i)).toBeInTheDocument();
  });

  it("renders Never when there is no expiry and the key is not expired", () => {
    render(<KeyExpiration isExpired={false} expiresAt={null} />);
    expect(screen.getByText("Never")).toBeInTheDocument();
  });
});

describe("ApiKeysContent", () => {
  const base = {
    canCreateDelete: true,
    isFileMode: false,
    onDeleteKey: vi.fn(),
  };

  it("renders skeletons while loading", () => {
    const { container } = render(
      <ApiKeysContent {...base} isLoading error={null} keys={undefined} />
    );
    expect(container.querySelectorAll(".space-y-2 > *").length).toBeGreaterThan(0);
  });

  it("renders an error message when error is set", () => {
    render(
      <ApiKeysContent
        {...base}
        isLoading={false}
        error={new Error("boom")}
        keys={undefined}
      />
    );
    expect(screen.getByText("Failed to load API keys")).toBeInTheDocument();
  });

  it("renders the create hint in the empty state when the user can create", () => {
    render(<ApiKeysContent {...base} isLoading={false} error={null} keys={[]} />);
    expect(screen.getByText("No API keys yet")).toBeInTheDocument();
    expect(
      screen.getByText("Create one to access the API programmatically")
    ).toBeInTheDocument();
  });

  it("renders the admin-provision hint in file mode", () => {
    render(
      <ApiKeysContent
        {...base}
        canCreateDelete={false}
        isFileMode
        isLoading={false}
        error={null}
        keys={[]}
      />
    );
    expect(
      screen.getByText("Contact your administrator to provision API keys")
    ).toBeInTheDocument();
  });

  it("renders a row per key and fires onDeleteKey when the trash button is clicked", () => {
    const onDeleteKey = vi.fn();
    render(
      <ApiKeysContent
        {...base}
        onDeleteKey={onDeleteKey}
        isLoading={false}
        error={null}
        keys={[makeKey({ id: "abc", name: "Row Key", lastUsedAt: new Date().toISOString() })]}
      />
    );
    expect(screen.getByText("Row Key")).toBeInTheDocument();

    const trash = screen
      .getAllByRole("button")
      .find((b) => b.querySelector("svg.lucide-trash-2"));
    expect(trash).toBeTruthy();
    fireEvent.click(trash!);
    expect(onDeleteKey).toHaveBeenCalledWith("abc");
  });

  it("hides the delete column when the user cannot create/delete", () => {
    render(
      <ApiKeysContent
        {...base}
        canCreateDelete={false}
        isLoading={false}
        error={null}
        keys={[makeKey()]}
      />
    );
    const trash = screen
      .queryAllByRole("button")
      .find((b) => b.querySelector("svg.lucide-trash-2"));
    expect(trash).toBeUndefined();
  });
});
