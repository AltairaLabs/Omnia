"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface PropertySchema {
  type?: string;
  description?: string;
  enum?: unknown[];
  items?: { type?: string };
  properties?: Record<string, PropertySchema>;
  required?: string[];
}

interface ObjectSchema {
  type: "object";
  properties: Record<string, PropertySchema>;
  required?: string[];
}

export interface SchemaFormProps {
  schema: unknown;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  idPrefix?: string;
}

// ---------------------------------------------------------------------------
// Public helpers
// ---------------------------------------------------------------------------

export function isRenderableObjectSchema(schema: unknown): boolean {
  if (!schema || typeof schema !== "object") return false;
  const s = schema as Record<string, unknown>;
  if (s.type !== "object") return false;
  const props = s.properties as Record<string, unknown> | undefined;
  return Boolean(props && Object.keys(props).length > 0);
}

// ---------------------------------------------------------------------------
// Field coercion helpers
// ---------------------------------------------------------------------------

function coerceArrayValue(raw: string, itemType: string | undefined): unknown[] {
  if (!raw.trim()) return [];
  return raw.split(",").map((item) => {
    const trimmed = item.trim();
    if (itemType === "number" || itemType === "integer") return Number(trimmed);
    return trimmed;
  });
}

function coerceNumberValue(raw: string): number | undefined {
  if (!raw.trim()) return undefined;
  return Number(raw);
}

// ---------------------------------------------------------------------------
// Individual field renderers
// ---------------------------------------------------------------------------

interface FieldProps {
  fieldKey: string;
  prop: PropertySchema;
  value: unknown;
  required: boolean;
  inputId: string;
  onChange: (val: unknown) => void;
}

function EnumField({ fieldKey, prop, value, required, inputId, onChange }: Readonly<FieldProps>) {
  const label = required ? `${fieldKey} *` : fieldKey;
  const current = value === undefined ? "" : String(value);
  return (
    <div className="space-y-1">
      <Label htmlFor={inputId}>{label}</Label>
      <Select value={current} onValueChange={(v) => onChange(v)}>
        <SelectTrigger id={inputId}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(prop.enum ?? []).map((opt) => {
            const s = String(opt);
            return <SelectItem key={s} value={s}>{s}</SelectItem>;
          })}
        </SelectContent>
      </Select>
      {prop.description && (
        <p className="text-xs text-muted-foreground">{prop.description}</p>
      )}
    </div>
  );
}

function BooleanField({ fieldKey, prop, value, required, inputId, onChange }: Readonly<FieldProps>) {
  const label = required ? `${fieldKey} *` : fieldKey;
  const checked = Boolean(value);
  return (
    <div className="flex items-center gap-2">
      <Checkbox
        id={inputId}
        checked={checked}
        onCheckedChange={(v) => onChange(Boolean(v))}
        aria-label={label}
      />
      <Label htmlFor={inputId}>{label}</Label>
      {prop.description && (
        <p className="text-xs text-muted-foreground ml-2">{prop.description}</p>
      )}
    </div>
  );
}

function ArrayField({ fieldKey, prop, value, required, inputId, onChange }: Readonly<FieldProps>) {
  const label = required ? `${fieldKey} *` : fieldKey;
  const raw = Array.isArray(value) ? value.join(", ") : String(value ?? "");
  return (
    <div className="space-y-1">
      <Label htmlFor={inputId}>{label}</Label>
      <Input
        id={inputId}
        value={raw}
        placeholder="comma-separated values"
        onChange={(e) => onChange(coerceArrayValue(e.target.value, prop.items?.type))}
      />
      {prop.description && (
        <p className="text-xs text-muted-foreground">{prop.description}</p>
      )}
    </div>
  );
}

function NumberField({ fieldKey, prop, value, required, inputId, onChange }: Readonly<FieldProps>) {
  const label = required ? `${fieldKey} *` : fieldKey;
  const raw = value !== undefined && value !== null ? String(value) : "";
  return (
    <div className="space-y-1">
      <Label htmlFor={inputId}>{label}</Label>
      <Input
        id={inputId}
        type="number"
        value={raw}
        onChange={(e) => {
          const coerced = coerceNumberValue(e.target.value);
          onChange(coerced);
        }}
      />
      {prop.description && (
        <p className="text-xs text-muted-foreground">{prop.description}</p>
      )}
    </div>
  );
}

function StringField({ fieldKey, prop, value, required, inputId, onChange }: Readonly<FieldProps>) {
  const label = required ? `${fieldKey} *` : fieldKey;
  const raw = value !== undefined && value !== null ? String(value) : "";
  return (
    <div className="space-y-1">
      <Label htmlFor={inputId}>{label}</Label>
      <Input
        id={inputId}
        value={raw}
        onChange={(e) => onChange(e.target.value)}
      />
      {prop.description && (
        <p className="text-xs text-muted-foreground">{prop.description}</p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Field dispatch
// ---------------------------------------------------------------------------

function renderField(props: FieldProps): React.ReactElement {
  const { prop } = props;
  if (prop.enum) return <EnumField {...props} />;
  if (prop.type === "boolean") return <BooleanField {...props} />;
  if (prop.type === "array") return <ArrayField {...props} />;
  if (prop.type === "number" || prop.type === "integer") return <NumberField {...props} />;
  return <StringField {...props} />;
}

// ---------------------------------------------------------------------------
// Nested object group
// ---------------------------------------------------------------------------

interface NestedGroupProps {
  parentKey: string;
  nestedSchema: PropertySchema;
  parentValue: Record<string, unknown>;
  required: boolean;
  idPrefix: string;
  onChange: (parentKey: string, next: Record<string, unknown>) => void;
}

function NestedObjectGroup({
  parentKey,
  nestedSchema,
  parentValue,
  required,
  idPrefix,
  onChange,
}: Readonly<NestedGroupProps>) {
  const nestedProps = nestedSchema.properties ?? {};
  const nestedRequired: string[] = nestedSchema.required ?? [];
  const label = required ? `${parentKey} *` : parentKey;

  return (
    <fieldset className="border rounded p-3 space-y-3">
      <legend className="text-sm font-medium px-1">{label}</legend>
      {Object.entries(nestedProps).map(([k, p]) => {
        const fieldId = `${idPrefix}-${parentKey}-${k}`;
        const isReq = nestedRequired.includes(k);
        return (
          <div key={k}>
            {renderField({
              fieldKey: k,
              prop: p,
              value: parentValue[k],
              required: isReq,
              inputId: fieldId,
              onChange: (val) => {
                const next = { ...parentValue, [k]: val };
                onChange(parentKey, next);
              },
            })}
          </div>
        );
      })}
    </fieldset>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function SchemaForm({
  schema,
  value,
  onChange,
  idPrefix = "sf",
}: Readonly<SchemaFormProps>): React.ReactElement {
  if (!isRenderableObjectSchema(schema)) {
    return (
      <p className="text-sm text-muted-foreground">
        No structured fields — use raw JSON
      </p>
    );
  }

  const s = schema as ObjectSchema;
  const required = s.required ?? [];

  const handleChange = (key: string, val: unknown) => {
    if (val === undefined) {
      const rest = Object.fromEntries(Object.entries(value).filter(([k]) => k !== key));
      onChange(rest);
    } else {
      onChange({ ...value, [key]: val });
    }
  };

  const handleNestedChange = (parentKey: string, next: Record<string, unknown>) => {
    onChange({ ...value, [parentKey]: next });
  };

  return (
    <div className="space-y-4">
      {Object.entries(s.properties).map(([k, p]) => {
        const isReq = required.includes(k);
        if (p.type === "object") {
          const nestedVal = (value[k] as Record<string, unknown>) ?? {};
          return (
            <NestedObjectGroup
              key={k}
              parentKey={k}
              nestedSchema={p}
              parentValue={nestedVal}
              required={isReq}
              idPrefix={idPrefix}
              onChange={handleNestedChange}
            />
          );
        }
        const fieldId = `${idPrefix}-${k}`;
        return (
          <div key={k}>
            {renderField({
              fieldKey: k,
              prop: p,
              value: value[k],
              required: isReq,
              inputId: fieldId,
              onChange: (val) => handleChange(k, val),
            })}
          </div>
        );
      })}
    </div>
  );
}
