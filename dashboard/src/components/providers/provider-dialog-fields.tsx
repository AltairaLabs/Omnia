"use client";

import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { ChevronDown, Plus, Trash2 } from "lucide-react";
import { makeHeaderEntryId, type FormState } from "./provider-dialog";

// --- Capabilities ---

const ALL_CAPABILITIES = [
  "text",
  "streaming",
  "vision",
  "tools",
  "json",
  "audio",
  "video",
  "documents",
  "duplex",
] as const;

export function CapabilitiesFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(form.capabilities.length > 0);

  const toggleCapability = (cap: string) => {
    const current = form.capabilities;
    if (current.includes(cap)) {
      updateForm("capabilities", current.filter((c) => c !== cap));
    } else {
      updateForm("capabilities", [...current, cap]);
    }
  };

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          Capabilities
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-2">
        <div className="grid grid-cols-2 gap-2">
          {ALL_CAPABILITIES.map((cap) => (
            <div key={cap} className="flex items-center space-x-2">
              <Checkbox
                id={`cap-${cap}`}
                checked={form.capabilities.includes(cap)}
                onCheckedChange={() => toggleCapability(cap)}
              />
              <Label htmlFor={`cap-${cap}`} className="text-sm font-normal capitalize">
                {cap}
              </Label>
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

// --- Defaults ---

export function DefaultsFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(
    !!(form.temperature || form.topP || form.maxTokens || form.contextWindow)
  );

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          Defaults
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 pt-2">
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="temperature">Temperature</Label>
            <Input
              id="temperature"
              type="number"
              step="0.1"
              min="0"
              max="2"
              placeholder="0.7"
              value={form.temperature}
              onChange={(e) => updateForm("temperature", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="top-p">Top P</Label>
            <Input
              id="top-p"
              type="number"
              step="0.1"
              min="0"
              max="1"
              placeholder="0.9"
              value={form.topP}
              onChange={(e) => updateForm("topP", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="max-tokens">Max Tokens</Label>
            <Input
              id="max-tokens"
              type="number"
              min="1"
              placeholder="4096"
              value={form.maxTokens}
              onChange={(e) => updateForm("maxTokens", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="context-window">Context Window</Label>
            <Input
              id="context-window"
              type="number"
              min="1"
              placeholder="128000"
              value={form.contextWindow}
              onChange={(e) => updateForm("contextWindow", e.target.value)}
            />
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

// --- Pricing ---

export function PricingFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(
    !!(form.inputCostPer1K || form.outputCostPer1K || form.cachedCostPer1K)
  );

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          Pricing
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 pt-2">
        <div className="grid grid-cols-3 gap-4">
          <div className="space-y-2">
            <Label htmlFor="input-cost">Input / 1K tokens</Label>
            <Input
              id="input-cost"
              placeholder="0.003"
              value={form.inputCostPer1K}
              onChange={(e) => updateForm("inputCostPer1K", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="output-cost">Output / 1K tokens</Label>
            <Input
              id="output-cost"
              placeholder="0.015"
              value={form.outputCostPer1K}
              onChange={(e) => updateForm("outputCostPer1K", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="cached-cost">Cached / 1K tokens</Label>
            <Input
              id="cached-cost"
              placeholder="0.0003"
              value={form.cachedCostPer1K}
              onChange={(e) => updateForm("cachedCostPer1K", e.target.value)}
            />
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

// --- Headers ---

export function HeadersFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(form.headerEntries.length > 0);

  const updateEntry = (index: number, field: "key" | "value", next: string) => {
    const entries = form.headerEntries.map((entry, i) =>
      i === index ? { ...entry, [field]: next } : entry,
    );
    updateForm("headerEntries", entries);
  };

  const addEntry = () => {
    updateForm("headerEntries", [
      ...form.headerEntries,
      { id: makeHeaderEntryId(), key: "", value: "" },
    ]);
  };

  const removeEntry = (index: number) => {
    updateForm(
      "headerEntries",
      form.headerEntries.filter((_, i) => i !== index),
    );
  };

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          HTTP Headers
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3 pt-2">
        <p className="text-sm text-muted-foreground">
          Custom HTTP headers sent on every provider request. Used by gateway providers
          (e.g., OpenRouter&rsquo;s <code>HTTP-Referer</code> / <code>X-Title</code>) or tenant
          routing. Collisions with built-in provider headers are rejected by PromptKit.
        </p>
        {form.headerEntries.map((entry, index) => (
          <div key={entry.id} className="flex gap-2 items-start">
            <Input
              aria-label={`Header ${index + 1} name`}
              placeholder="HTTP-Referer"
              value={entry.key}
              onChange={(e) => updateEntry(index, "key", e.target.value)}
              className="flex-1"
            />
            <Input
              aria-label={`Header ${index + 1} value`}
              placeholder="https://my-app.example.com"
              value={entry.value}
              onChange={(e) => updateEntry(index, "value", e.target.value)}
              className="flex-1"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`Remove header ${index + 1}`}
              onClick={() => removeEntry(index)}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={addEntry}>
          <Plus className="h-4 w-4 mr-1" />
          Add header
        </Button>
      </CollapsibleContent>
    </Collapsible>
  );
}

