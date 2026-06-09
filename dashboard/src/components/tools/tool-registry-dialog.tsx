"use client";

import { useState } from "react";
import { AlertCircle, Loader2, Plus, Trash2 } from "lucide-react";
import { useToolRegistryMutations } from "@/hooks/use-tool-registry-mutations";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription } from "@/components/ui/alert";
import type {
  HandlerDefinition,
  MCPClientConfig,
  OpenAPIConfig,
  ToolRegistrySpec,
} from "@/types/tool-registry";

// v1 supports the three common server-side tool sources. grpc/client handlers
// are creatable via kubectl/YAML and can be added to this form later.
export type HandlerKind = "http" | "mcp" | "openapi";
export type MCPTransport = "sse" | "stdio";

export interface HandlerForm {
  id: string;
  name: string;
  type: HandlerKind;
  // http
  httpEndpoint: string;
  httpMethod: string;
  httpToolName: string;
  httpToolDescription: string;
  // openapi
  openapiSpecURL: string;
  openapiBaseURL: string;
  // mcp
  mcpTransport: MCPTransport;
  mcpEndpoint: string;
  mcpCommand: string;
  mcpArgs: string;
}

export interface ToolRegistryFormState {
  name: string;
  handlers: HandlerForm[];
}

let nextHandlerId = 0;
function makeHandlerId(): string {
  nextHandlerId += 1;
  return `handler-${nextHandlerId}`;
}

export function emptyHandler(): HandlerForm {
  return {
    id: makeHandlerId(),
    name: "",
    type: "http",
    httpEndpoint: "",
    httpMethod: "",
    httpToolName: "",
    httpToolDescription: "",
    openapiSpecURL: "",
    openapiBaseURL: "",
    mcpTransport: "sse",
    mcpEndpoint: "",
    mcpCommand: "",
    mcpArgs: "",
  };
}

function initialFormState(): ToolRegistryFormState {
  return { name: "", handlers: [emptyHandler()] };
}

// --- Validation (exported for unit tests) ---

export function validateName(name: string): string | null {
  if (!name.trim()) return "Name is required";
  if (!/^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/.test(name)) {
    return "Name must be a valid DNS subdomain (lowercase alphanumeric, hyphens, dots)";
  }
  return null;
}

function validateHandler(h: HandlerForm, index: number): string | null {
  const where = `Handler ${index + 1}`;
  if (!h.name.trim()) return `${where}: name is required`;
  if (h.type === "http") {
    if (!h.httpEndpoint.trim()) return `${where}: endpoint is required`;
    if (!h.httpToolName.trim()) return `${where}: tool name is required`;
    return null;
  }
  if (h.type === "openapi") {
    if (!h.openapiSpecURL.trim()) return `${where}: spec URL is required`;
    return null;
  }
  // mcp
  if (h.mcpTransport === "sse" && !h.mcpEndpoint.trim()) {
    return `${where}: endpoint is required for SSE transport`;
  }
  if (h.mcpTransport === "stdio" && !h.mcpCommand.trim()) {
    return `${where}: command is required for stdio transport`;
  }
  return null;
}

export function validateToolRegistryForm(form: ToolRegistryFormState): string | null {
  const nameError = validateName(form.name);
  if (nameError) return nameError;
  if (form.handlers.length === 0) return "At least one handler is required";
  for (let i = 0; i < form.handlers.length; i++) {
    const err = validateHandler(form.handlers[i], i);
    if (err) return err;
  }
  return null;
}

// --- Spec building (exported for unit tests) ---

function buildHttpHandler(h: HandlerForm): HandlerDefinition {
  const def: HandlerDefinition = {
    name: h.name,
    type: "http",
    httpConfig: { endpoint: h.httpEndpoint },
  };
  if (h.httpMethod.trim()) def.httpConfig!.method = h.httpMethod.trim();
  if (h.httpToolName.trim()) {
    def.tool = { name: h.httpToolName.trim(), description: h.httpToolDescription };
  }
  return def;
}

function buildOpenAPIHandler(h: HandlerForm): HandlerDefinition {
  const cfg: OpenAPIConfig = { specURL: h.openapiSpecURL };
  if (h.openapiBaseURL.trim()) cfg.baseURL = h.openapiBaseURL.trim();
  return { name: h.name, type: "openapi", openAPIConfig: cfg };
}

function buildMcpHandler(h: HandlerForm): HandlerDefinition {
  const cfg: MCPClientConfig = { transport: h.mcpTransport };
  if (h.mcpTransport === "sse") {
    if (h.mcpEndpoint.trim()) cfg.endpoint = h.mcpEndpoint.trim();
  } else {
    if (h.mcpCommand.trim()) cfg.command = h.mcpCommand.trim();
    const args = h.mcpArgs.trim();
    if (args) cfg.args = args.split(/\s+/);
  }
  return { name: h.name, type: "mcp", mcpConfig: cfg };
}

const HANDLER_BUILDERS: Record<HandlerKind, (h: HandlerForm) => HandlerDefinition> = {
  http: buildHttpHandler,
  openapi: buildOpenAPIHandler,
  mcp: buildMcpHandler,
};

export function buildToolRegistrySpec(form: ToolRegistryFormState): ToolRegistrySpec {
  return { handlers: form.handlers.map((h) => HANDLER_BUILDERS[h.type](h)) };
}

// --- Sub-components ---

type UpdateHandler = (id: string, patch: Partial<HandlerForm>) => void;

function HttpFields({ h, update }: Readonly<{ h: HandlerForm; update: UpdateHandler }>) {
  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <Label htmlFor={`${h.id}-endpoint`}>Endpoint</Label>
        <Input
          id={`${h.id}-endpoint`}
          placeholder="https://my-svc.default.svc:8080/tool"
          value={h.httpEndpoint}
          onChange={(e) => update(h.id, { httpEndpoint: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label htmlFor={`${h.id}-tool-name`}>Tool name</Label>
          <Input
            id={`${h.id}-tool-name`}
            placeholder="get_weather"
            value={h.httpToolName}
            onChange={(e) => update(h.id, { httpToolName: e.target.value })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${h.id}-method`}>Method (optional)</Label>
          <Input
            id={`${h.id}-method`}
            placeholder="POST"
            value={h.httpMethod}
            onChange={(e) => update(h.id, { httpMethod: e.target.value })}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${h.id}-tool-desc`}>Tool description (optional)</Label>
        <Input
          id={`${h.id}-tool-desc`}
          placeholder="Get the weather for a city"
          value={h.httpToolDescription}
          onChange={(e) => update(h.id, { httpToolDescription: e.target.value })}
        />
      </div>
    </div>
  );
}

export function OpenAPIFields({ h, update }: Readonly<{ h: HandlerForm; update: UpdateHandler }>) {
  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <Label htmlFor={`${h.id}-spec-url`}>Spec URL</Label>
        <Input
          id={`${h.id}-spec-url`}
          placeholder="https://api.example.com/openapi.json"
          value={h.openapiSpecURL}
          onChange={(e) => update(h.id, { openapiSpecURL: e.target.value })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${h.id}-base-url`}>Base URL (optional)</Label>
        <Input
          id={`${h.id}-base-url`}
          placeholder="https://api.example.com"
          value={h.openapiBaseURL}
          onChange={(e) => update(h.id, { openapiBaseURL: e.target.value })}
        />
      </div>
    </div>
  );
}

export function McpFields({ h, update }: Readonly<{ h: HandlerForm; update: UpdateHandler }>) {
  return (
    <div className="space-y-3">
      <div className="space-y-2">
        <Label htmlFor={`${h.id}-transport`}>Transport</Label>
        <Select
          value={h.mcpTransport}
          onValueChange={(v) => update(h.id, { mcpTransport: v as MCPTransport })}
        >
          <SelectTrigger id={`${h.id}-transport`}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="sse">SSE (HTTP)</SelectItem>
            <SelectItem value="stdio">stdio (subprocess)</SelectItem>
          </SelectContent>
        </Select>
      </div>
      {h.mcpTransport === "sse" ? (
        <div className="space-y-2">
          <Label htmlFor={`${h.id}-mcp-endpoint`}>Endpoint</Label>
          <Input
            id={`${h.id}-mcp-endpoint`}
            placeholder="https://mcp-server.default.svc:8080/sse"
            value={h.mcpEndpoint}
            onChange={(e) => update(h.id, { mcpEndpoint: e.target.value })}
          />
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-2">
            <Label htmlFor={`${h.id}-mcp-command`}>Command</Label>
            <Input
              id={`${h.id}-mcp-command`}
              placeholder="npx"
              value={h.mcpCommand}
              onChange={(e) => update(h.id, { mcpCommand: e.target.value })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${h.id}-mcp-args`}>Args (space-separated)</Label>
            <Input
              id={`${h.id}-mcp-args`}
              placeholder="-y @modelcontextprotocol/server-everything"
              value={h.mcpArgs}
              onChange={(e) => update(h.id, { mcpArgs: e.target.value })}
            />
          </div>
        </div>
      )}
    </div>
  );
}

const HANDLER_FIELDS: Record<
  HandlerKind,
  (props: { h: HandlerForm; update: UpdateHandler }) => React.ReactElement
> = {
  http: HttpFields,
  openapi: OpenAPIFields,
  mcp: McpFields,
};

function HandlerCard({
  h,
  index,
  canRemove,
  update,
  remove,
}: Readonly<{
  h: HandlerForm;
  index: number;
  canRemove: boolean;
  update: UpdateHandler;
  remove: (id: string) => void;
}>) {
  const Fields = HANDLER_FIELDS[h.type];
  return (
    <div className="border rounded-lg p-4 space-y-3">
      <div className="flex items-center justify-between">
        <Label className="text-sm font-semibold">Handler {index + 1}</Label>
        {canRemove && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`Remove handler ${index + 1}`}
            onClick={() => remove(h.id)}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label htmlFor={`${h.id}-name`}>Handler name</Label>
          <Input
            id={`${h.id}-name`}
            placeholder="weather-api"
            value={h.name}
            onChange={(e) => update(h.id, { name: e.target.value })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${h.id}-type`}>Type</Label>
          <Select value={h.type} onValueChange={(v) => update(h.id, { type: v as HandlerKind })}>
            <SelectTrigger id={`${h.id}-type`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="http">HTTP</SelectItem>
              <SelectItem value="mcp">MCP server</SelectItem>
              <SelectItem value="openapi">OpenAPI</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <Fields h={h} update={update} />
    </div>
  );
}

// --- Main dialog ---

interface ToolRegistryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
}

export function ToolRegistryDialog({
  open,
  onOpenChange,
  onSuccess,
}: Readonly<ToolRegistryDialogProps>) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <ToolRegistryDialogForm
        key={`new-${open}`}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    </Dialog>
  );
}

function ToolRegistryDialogForm({
  onOpenChange,
  onSuccess,
}: Readonly<{ onOpenChange: (open: boolean) => void; onSuccess?: () => void }>) {
  const { createToolRegistry, loading } = useToolRegistryMutations();
  const [form, setForm] = useState<ToolRegistryFormState>(() => initialFormState());
  const [error, setError] = useState<string | null>(null);

  const update: UpdateHandler = (id, patch) => {
    setForm((prev) => ({
      ...prev,
      handlers: prev.handlers.map((h) => (h.id === id ? { ...h, ...patch } : h)),
    }));
  };

  const addHandler = () => {
    setForm((prev) => ({ ...prev, handlers: [...prev.handlers, emptyHandler()] }));
  };

  const removeHandler = (id: string) => {
    setForm((prev) => ({ ...prev, handlers: prev.handlers.filter((h) => h.id !== id) }));
  };

  const handleSubmit = async () => {
    setError(null);
    const validationError = validateToolRegistryForm(form);
    if (validationError) {
      setError(validationError);
      return;
    }
    try {
      await createToolRegistry(form.name, buildToolRegistrySpec(form));
      onSuccess?.();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create tool registry");
    }
  };

  return (
    <DialogContent className="sm:max-w-[600px] max-h-[90vh] flex flex-col overflow-hidden">
      <DialogHeader>
        <DialogTitle>Create ToolRegistry</DialogTitle>
        <DialogDescription>
          Register a source of tools for your workspace. Add one or more handlers (HTTP, MCP, or
          OpenAPI); the operator discovers tools from each.
        </DialogDescription>
      </DialogHeader>

      <div className="flex-1 min-h-0 overflow-y-auto -mx-6 px-6">
        <div className="space-y-6 py-4">
          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-2">
            <Label htmlFor="registry-name">Name</Label>
            <Input
              id="registry-name"
              placeholder="my-tools"
              value={form.name}
              onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
            />
          </div>

          {form.handlers.map((h, index) => (
            <HandlerCard
              key={h.id}
              h={h}
              index={index}
              canRemove={form.handlers.length > 1}
              update={update}
              remove={removeHandler}
            />
          ))}

          <Button type="button" variant="outline" size="sm" onClick={addHandler}>
            <Plus className="h-4 w-4 mr-1" />
            Add handler
          </Button>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={loading}>
          {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
          Create ToolRegistry
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
