"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { FieldError } from "@/components/ui/field-error";
import type {
  ConfigMapSourceRef,
  GitSourceRef,
  OCISourceRef,
} from "@/types/skill-source";

export interface FieldValidateProps {
  validate: (path: string, value: unknown) => void;
  errors: Record<string, string>;
}

export function GitFields({
  spec,
  onChange,
  validate,
  errors,
}: Readonly<{
  spec: GitSourceRef;
  onChange: (spec: GitSourceRef) => void;
} & FieldValidateProps>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="git-url">Repository URL</Label>
        <Input
          id="git-url"
          placeholder="https://github.com/org/skills.git"
          aria-invalid={!!errors["spec.git.url"]}
          aria-describedby={errors["spec.git.url"] ? "git-url-error" : undefined}
          value={spec.url}
          onChange={(e) => {
            onChange({ ...spec, url: e.target.value });
            validate("spec.git.url", e.target.value);
          }}
        />
        <FieldError id="git-url-error" message={errors["spec.git.url"]} />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="git-branch">Branch</Label>
          <Input
            id="git-branch"
            placeholder="main"
            value={spec.ref?.branch ?? ""}
            onChange={(e) =>
              onChange({
                ...spec,
                ref: { ...spec.ref, branch: e.target.value || undefined },
              })
            }
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="git-path">Path</Label>
          <Input
            id="git-path"
            placeholder="skills/"
            value={spec.path ?? ""}
            onChange={(e) =>
              onChange({ ...spec, path: e.target.value || undefined })
            }
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="git-secret">Secret (optional)</Label>
        <Input
          id="git-secret"
          placeholder="git-credentials"
          value={spec.secretRef?.name ?? ""}
          onChange={(e) =>
            onChange({
              ...spec,
              secretRef: e.target.value
                ? { name: e.target.value }
                : undefined,
            })
          }
        />
      </div>
    </div>
  );
}

export function OCIFields({
  spec,
  onChange,
  validate,
  errors,
}: Readonly<{
  spec: OCISourceRef;
  onChange: (spec: OCISourceRef) => void;
} & FieldValidateProps>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="oci-url">OCI URL</Label>
        <Input
          id="oci-url"
          placeholder="oci://ghcr.io/org/skills"
          aria-invalid={!!errors["spec.oci.url"]}
          aria-describedby={errors["spec.oci.url"] ? "oci-url-error" : undefined}
          value={spec.url}
          onChange={(e) => {
            onChange({ ...spec, url: e.target.value });
            validate("spec.oci.url", e.target.value);
          }}
        />
        <FieldError id="oci-url-error" message={errors["spec.oci.url"]} />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="oci-secret">Secret (optional)</Label>
          <Input
            id="oci-secret"
            placeholder="registry-creds"
            value={spec.secretRef?.name ?? ""}
            onChange={(e) =>
              onChange({
                ...spec,
                secretRef: e.target.value
                  ? { name: e.target.value }
                  : undefined,
              })
            }
          />
        </div>
        <div className="flex items-center gap-2 pt-8">
          <Switch
            id="oci-insecure"
            checked={spec.insecure ?? false}
            onCheckedChange={(checked) =>
              onChange({ ...spec, insecure: checked })
            }
          />
          <Label htmlFor="oci-insecure" className="cursor-pointer">
            Allow insecure (HTTP) pulls
          </Label>
        </div>
      </div>
    </div>
  );
}

export function ConfigMapFields({
  spec,
  onChange,
  validate,
  errors,
}: Readonly<{
  spec: ConfigMapSourceRef;
  onChange: (spec: ConfigMapSourceRef) => void;
} & FieldValidateProps>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="cm-name">ConfigMap name</Label>
        <Input
          id="cm-name"
          placeholder="my-skills"
          aria-invalid={!!errors["spec.configMap.name"]}
          aria-describedby={errors["spec.configMap.name"] ? "cm-name-error" : undefined}
          value={spec.name}
          onChange={(e) => {
            onChange({ ...spec, name: e.target.value });
            validate("spec.configMap.name", e.target.value);
          }}
        />
        <FieldError id="cm-name-error" message={errors["spec.configMap.name"]} />
      </div>
      <div className="space-y-2">
        <Label htmlFor="cm-key">Key (optional)</Label>
        <Input
          id="cm-key"
          placeholder="pack.json"
          value={spec.key ?? ""}
          onChange={(e) =>
            onChange({ ...spec, key: e.target.value || undefined })
          }
        />
      </div>
    </div>
  );
}
