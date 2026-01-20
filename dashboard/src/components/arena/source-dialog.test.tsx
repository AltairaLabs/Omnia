/**
 * Tests for SourceDialog component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SourceDialog } from "./source-dialog";
import type { ArenaSource } from "@/types/arena";

const createMockSource = (overrides: Partial<ArenaSource> = {}): ArenaSource => ({
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ArenaSource",
  metadata: { name: "test-source" },
  spec: { type: "configmap", interval: "5m", configMapRef: { name: "test-cm" } },
  status: { phase: "Ready" },
  ...overrides,
});

// Mock hooks
const mockCreateSource = vi.fn();
const mockUpdateSource = vi.fn();
vi.mock("@/hooks", () => ({
  useArenaSourceMutations: vi.fn(() => ({
    createSource: mockCreateSource,
    updateSource: mockUpdateSource,
    loading: false,
    error: null,
  })),
}));

vi.mock("@/hooks/use-license", () => ({
  useLicense: vi.fn(() => ({
    license: { tier: "enterprise" },
    isEnterprise: true,
  })),
}));

describe("SourceDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
    mockUpdateSource.mockResolvedValue({});
  });

  it("renders create dialog when no source is provided", () => {
    render(<SourceDialog {...defaultProps} />);

    expect(screen.getByRole("heading", { name: "Create Source" })).toBeInTheDocument();
    expect(screen.getByText("Configure a new source for PromptKit bundles.")).toBeInTheDocument();
  });

  it("renders edit dialog when source is provided", () => {
    const source = createMockSource({
      metadata: { name: "existing-source" },
      spec: { type: "configmap", interval: "5m", configMapRef: { name: "my-configmap" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect(screen.getByText("Edit Source")).toBeInTheDocument();
    expect(screen.getByText("Update the configuration for this Arena source.")).toBeInTheDocument();
  });

  it("renders form fields correctly", () => {
    render(<SourceDialog {...defaultProps} />);

    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByText("Source Type")).toBeInTheDocument();
    expect(screen.getByText("Sync Interval")).toBeInTheDocument();
  });

  it("shows validation error when name is empty", async () => {
    render(<SourceDialog {...defaultProps} />);

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
  });

  it("shows validation error when configmap name is empty", async () => {
    render(<SourceDialog {...defaultProps} />);

    // Fill in source name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("ConfigMap name is required")).toBeInTheDocument();
    });
  });

  it("creates source successfully with valid data", async () => {
    render(<SourceDialog {...defaultProps} />);

    // Fill in source name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    // Fill in configmap name
    const configmapInput = screen.getByLabelText("ConfigMap Name");
    fireEvent.change(configmapInput, { target: { value: "my-configmap" } });

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(mockCreateSource).toHaveBeenCalledWith(
        "my-source",
        expect.objectContaining({
          type: "configmap",
          interval: "5m",
          configMapRef: { name: "my-configmap" },
        })
      );
    });
  });

  it("calls onSuccess after successful creation", async () => {
    render(<SourceDialog {...defaultProps} />);

    // Fill in source name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    // Fill in configmap name
    const configmapInput = screen.getByLabelText("ConfigMap Name");
    fireEvent.change(configmapInput, { target: { value: "my-configmap" } });

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(defaultProps.onSuccess).toHaveBeenCalled();
    });
  });

  it("disables name field when editing", () => {
    const source = createMockSource({
      metadata: { name: "existing-source" },
      spec: { type: "configmap", interval: "5m", configMapRef: { name: "my-configmap" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const nameInput = screen.getByLabelText("Name");
    expect(nameInput).toBeDisabled();
  });

  it("pre-fills form when editing", () => {
    const source = createMockSource({
      metadata: { name: "existing-source" },
      spec: { type: "configmap", interval: "10m", configMapRef: { name: "my-configmap" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const nameInput = screen.getByLabelText("Name") as HTMLInputElement;
    expect(nameInput.value).toBe("existing-source");

    const configmapInput = screen.getByLabelText("ConfigMap Name") as HTMLInputElement;
    expect(configmapInput.value).toBe("my-configmap");
  });

  it("calls updateSource when editing", async () => {
    const source = createMockSource({
      metadata: { name: "existing-source" },
      spec: { type: "configmap", interval: "5m", configMapRef: { name: "my-configmap" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    // Update configmap name
    const configmapInput = screen.getByLabelText("ConfigMap Name");
    fireEvent.change(configmapInput, { target: { value: "updated-configmap" } });

    const saveButton = screen.getByRole("button", { name: "Save Changes" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(mockUpdateSource).toHaveBeenCalledWith(
        "existing-source",
        expect.objectContaining({
          type: "configmap",
          configMapRef: { name: "updated-configmap" },
        })
      );
    });
  });

  it("calls onClose when cancel is clicked", () => {
    render(<SourceDialog {...defaultProps} />);

    const cancelButton = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancelButton);

    expect(defaultProps.onClose).toHaveBeenCalled();
  });

  it("shows error message when creation fails", async () => {
    mockCreateSource.mockRejectedValueOnce(new Error("Creation failed"));

    render(<SourceDialog {...defaultProps} />);

    // Fill in source name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    // Fill in configmap name
    const configmapInput = screen.getByLabelText("ConfigMap Name");
    fireEvent.change(configmapInput, { target: { value: "my-configmap" } });

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Creation failed")).toBeInTheDocument();
    });
  });

  it("does not render when dialog is closed", () => {
    render(<SourceDialog {...defaultProps} open={false} />);

    expect(screen.queryByText("Create Source")).not.toBeInTheDocument();
  });
});

describe("SourceDialog - Enterprise Features", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("renders source type selector", () => {
    render(<SourceDialog {...defaultProps} />);

    // Check that source type selector is rendered
    expect(screen.getByText("Source Type")).toBeInTheDocument();
    // ConfigMap should be shown as the default selected option
    expect(screen.getByText("ConfigMap")).toBeInTheDocument();
  });
});

describe("SourceDialog - Git Source Type", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("renders git source fields when editing a git source", () => {
    const source = createMockSource({
      metadata: { name: "git-source" },
      spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git", ref: { branch: "main" }, path: "prompts/" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect(screen.getByLabelText("Repository URL")).toBeInTheDocument();
    expect(screen.getByLabelText("Branch")).toBeInTheDocument();
    expect(screen.getByLabelText("Path")).toBeInTheDocument();
  });

  it("pre-fills git source fields when editing", () => {
    const source = createMockSource({
      metadata: { name: "git-source" },
      spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git", ref: { branch: "main" }, path: "prompts/" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect((screen.getByLabelText("Repository URL") as HTMLInputElement).value).toBe("https://github.com/org/repo.git");
    expect((screen.getByLabelText("Branch") as HTMLInputElement).value).toBe("main");
    expect((screen.getByLabelText("Path") as HTMLInputElement).value).toBe("prompts/");
  });

  it("allows editing git source fields", () => {
    const source = createMockSource({
      metadata: { name: "git-source" },
      spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const urlInput = screen.getByLabelText("Repository URL");
    fireEvent.change(urlInput, { target: { value: "https://github.com/new/repo.git" } });
    expect((urlInput as HTMLInputElement).value).toBe("https://github.com/new/repo.git");

    const branchInput = screen.getByLabelText("Branch");
    fireEvent.change(branchInput, { target: { value: "develop" } });
    expect((branchInput as HTMLInputElement).value).toBe("develop");

    const pathInput = screen.getByLabelText("Path");
    fireEvent.change(pathInput, { target: { value: "src/" } });
    expect((pathInput as HTMLInputElement).value).toBe("src/");
  });
});

describe("SourceDialog - OCI Source Type", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("renders OCI source fields when editing an OCI source", () => {
    const source = createMockSource({
      metadata: { name: "oci-source" },
      spec: { type: "oci", interval: "5m", oci: { url: "oci://ghcr.io/org/pkg", ref: { tag: "latest" } } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect(screen.getByLabelText("OCI Repository URL")).toBeInTheDocument();
    expect(screen.getByLabelText("Tag")).toBeInTheDocument();
    expect(screen.getByLabelText("SemVer Constraint")).toBeInTheDocument();
  });

  it("pre-fills OCI source fields when editing", () => {
    const source = createMockSource({
      metadata: { name: "oci-source" },
      spec: { type: "oci", interval: "5m", oci: { url: "oci://ghcr.io/org/pkg", ref: { tag: "v1.0.0", semver: ">=1.0.0" } } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect((screen.getByLabelText("OCI Repository URL") as HTMLInputElement).value).toBe("oci://ghcr.io/org/pkg");
    expect((screen.getByLabelText("Tag") as HTMLInputElement).value).toBe("v1.0.0");
    expect((screen.getByLabelText("SemVer Constraint") as HTMLInputElement).value).toBe(">=1.0.0");
  });

  it("allows editing OCI source fields", () => {
    const source = createMockSource({
      metadata: { name: "oci-source" },
      spec: { type: "oci", interval: "5m", oci: { url: "oci://ghcr.io/org/pkg" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const urlInput = screen.getByLabelText("OCI Repository URL");
    fireEvent.change(urlInput, { target: { value: "oci://docker.io/org/pkg" } });
    expect((urlInput as HTMLInputElement).value).toBe("oci://docker.io/org/pkg");

    const tagInput = screen.getByLabelText("Tag");
    fireEvent.change(tagInput, { target: { value: "v2.0.0" } });
    expect((tagInput as HTMLInputElement).value).toBe("v2.0.0");

    const semverInput = screen.getByLabelText("SemVer Constraint");
    fireEvent.change(semverInput, { target: { value: ">=2.0.0" } });
    expect((semverInput as HTMLInputElement).value).toBe(">=2.0.0");
  });
});

describe("SourceDialog - S3 Source Type", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("renders S3 source fields when editing an S3 source", () => {
    const source = createMockSource({
      metadata: { name: "s3-source" },
      spec: { type: "s3", interval: "5m", s3: { bucket: "my-bucket", prefix: "prompts/", region: "us-east-1" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect(screen.getByLabelText("Bucket Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Prefix")).toBeInTheDocument();
    expect(screen.getByLabelText("Region")).toBeInTheDocument();
    expect(screen.getByLabelText("Custom Endpoint (optional)")).toBeInTheDocument();
  });

  it("pre-fills S3 source fields when editing", () => {
    const source = createMockSource({
      metadata: { name: "s3-source" },
      spec: { type: "s3", interval: "5m", s3: { bucket: "my-bucket", prefix: "prompts/", region: "us-east-1", endpoint: "https://custom.s3.com" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    expect((screen.getByLabelText("Bucket Name") as HTMLInputElement).value).toBe("my-bucket");
    expect((screen.getByLabelText("Prefix") as HTMLInputElement).value).toBe("prompts/");
    expect((screen.getByLabelText("Region") as HTMLInputElement).value).toBe("us-east-1");
    expect((screen.getByLabelText("Custom Endpoint (optional)") as HTMLInputElement).value).toBe("https://custom.s3.com");
  });

  it("allows editing S3 source fields", () => {
    const source = createMockSource({
      metadata: { name: "s3-source" },
      spec: { type: "s3", interval: "5m", s3: { bucket: "my-bucket" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const bucketInput = screen.getByLabelText("Bucket Name");
    fireEvent.change(bucketInput, { target: { value: "new-bucket" } });
    expect((bucketInput as HTMLInputElement).value).toBe("new-bucket");

    const prefixInput = screen.getByLabelText("Prefix");
    fireEvent.change(prefixInput, { target: { value: "data/" } });
    expect((prefixInput as HTMLInputElement).value).toBe("data/");

    const regionInput = screen.getByLabelText("Region");
    fireEvent.change(regionInput, { target: { value: "eu-west-1" } });
    expect((regionInput as HTMLInputElement).value).toBe("eu-west-1");

    const endpointInput = screen.getByLabelText("Custom Endpoint (optional)");
    fireEvent.change(endpointInput, { target: { value: "https://minio.local" } });
    expect((endpointInput as HTMLInputElement).value).toBe("https://minio.local");
  });
});

describe("SourceDialog Helper Functions", () => {
  it("validateForm returns error for empty name", async () => {
    // Import the module to test the helper functions
    const mod = await import("./source-dialog");

    // The helper functions are not exported, but we test them through the component
    // This test verifies the component shows validation errors
    render(
      <mod.SourceDialog
        open={true}
        onOpenChange={vi.fn()}
        onSuccess={vi.fn()}
        onClose={vi.fn()}
      />
    );

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
  });
});

describe("SourceDialog - Validation Errors", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("shows validation error when git URL is empty", async () => {
    const source = createMockSource({
      metadata: { name: "git-source" },
      spec: { type: "git", interval: "5m", git: { url: "" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const saveButton = screen.getByRole("button", { name: "Save Changes" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(screen.getByText("Git repository URL is required")).toBeInTheDocument();
    });
  });

  it("shows validation error when OCI URL is empty", async () => {
    const source = createMockSource({
      metadata: { name: "oci-source" },
      spec: { type: "oci", interval: "5m", oci: { url: "" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const saveButton = screen.getByRole("button", { name: "Save Changes" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(screen.getByText("OCI repository URL is required")).toBeInTheDocument();
    });
  });

  it("shows validation error when S3 bucket is empty", async () => {
    const source = createMockSource({
      metadata: { name: "s3-source" },
      spec: { type: "s3", interval: "5m", s3: { bucket: "" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const saveButton = screen.getByRole("button", { name: "Save Changes" });
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(screen.getByText("S3 bucket name is required")).toBeInTheDocument();
    });
  });
});

describe("SourceDialog - SecretRef handling", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("includes secretRef when credentials secret is provided", async () => {
    render(<SourceDialog {...defaultProps} />);

    // Fill in required fields
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    const configmapInput = screen.getByLabelText("ConfigMap Name");
    fireEvent.change(configmapInput, { target: { value: "my-configmap" } });

    // Fill in optional credentials secret
    const secretInput = screen.getByLabelText("Credentials Secret (optional)");
    fireEvent.change(secretInput, { target: { value: "my-credentials" } });

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(mockCreateSource).toHaveBeenCalledWith(
        "my-source",
        expect.objectContaining({
          type: "configmap",
          interval: "5m",
          configMapRef: { name: "my-configmap" },
          secretRef: { name: "my-credentials" },
        })
      );
    });
  });

  it("pre-fills secretRef when editing source with credentials", () => {
    const source = createMockSource({
      metadata: { name: "test-source" },
      spec: {
        type: "git",
        interval: "5m",
        git: { url: "https://github.com/org/repo.git" },
        secretRef: { name: "git-credentials" },
      },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const secretInput = screen.getByLabelText("Credentials Secret (optional)") as HTMLInputElement;
    expect(secretInput.value).toBe("git-credentials");
  });
});


describe("SourceDialog - Error handling", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("displays generic error message when error is not an Error instance", async () => {
    mockCreateSource.mockRejectedValueOnce("Something went wrong");

    render(<SourceDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    const configmapInput = screen.getByLabelText("ConfigMap Name");
    fireEvent.change(configmapInput, { target: { value: "my-configmap" } });

    const createButton = screen.getByRole("button", { name: "Create Source" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Failed to save source")).toBeInTheDocument();
    });
  });
});

describe("SourceDialog - Interval selection", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
  });

  it("pre-fills interval from existing source", () => {
    const source = createMockSource({
      metadata: { name: "test-source" },
      spec: { type: "configmap", interval: "30m", configMapRef: { name: "cm" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    // The interval dropdown should display the value
    expect(screen.getByText("30 minutes")).toBeInTheDocument();
  });
});

describe("SourceDialog - Clear branch on empty input", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    onClose: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateSource.mockResolvedValue({});
    mockUpdateSource.mockResolvedValue({});
  });

  it("clears branch value when input is emptied", () => {
    const source = createMockSource({
      metadata: { name: "git-source" },
      spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git", ref: { branch: "main" } } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const branchInput = screen.getByLabelText("Branch");
    expect((branchInput as HTMLInputElement).value).toBe("main");

    fireEvent.change(branchInput, { target: { value: "" } });
    expect((branchInput as HTMLInputElement).value).toBe("");
  });

  it("clears path value when input is emptied", () => {
    const source = createMockSource({
      metadata: { name: "git-source" },
      spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git", path: "prompts/" } },
    });

    render(<SourceDialog {...defaultProps} source={source} />);

    const pathInput = screen.getByLabelText("Path");
    expect((pathInput as HTMLInputElement).value).toBe("prompts/");

    fireEvent.change(pathInput, { target: { value: "" } });
    expect((pathInput as HTMLInputElement).value).toBe("");
  });
});
