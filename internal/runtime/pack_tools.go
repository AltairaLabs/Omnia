/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/go-logr/logr"
)

// surfaceRegistryToolsInPack rewrites the pack so every prompt's allowed-tools
// list (`tools` in pack.json) includes the tools provided by the attached
// ToolRegistry, writing the result to a temp file and returning its path.
//
// Why this exists: in Omnia the ToolRegistry CRD is the source of truth for
// which tools an agent can call — the PromptPack only carries the prompt. But
// PromptKit's ProviderStage only surfaces a tool to the LLM when the prompt
// template's allowed-tools list names it (pack-declared) or the tool is a
// system-namespaced capability (memory__, mcp__, ...). A plain `http`/`grpc`
// tool that Omnia registers into the conversation's registry but that the pack
// prompt never lists is therefore filtered out: buildProviderTools yields zero
// descriptors, the stage calls Predict instead of PredictWithTools, and the
// tool is never dispatched (with the mock provider this surfaces as the
// scripted tool_call being replaced by the text defaultResponse). See #734 and
// the policy-broker enforcement E2E.
//
// Bridging that here — unioning the registry tool names into each prompt's
// allowed list — makes attached ToolRegistry tools reachable without forcing
// every pack author to duplicate the tool names in the prompt. Per-call
// allow/deny governance stays with ToolPolicy CRDs (the policy broker), not the
// pack's `tools` list.
//
// On any read/parse/write failure it returns the original path unchanged
// (fail-open to current behaviour) rather than blocking startup.
func surfaceRegistryToolsInPack(packPath string, toolNames []string, log logr.Logger) string {
	if packPath == "" || len(toolNames) == 0 {
		return packPath
	}

	sort.Strings(toolNames)

	data, err := os.ReadFile(packPath)
	if err != nil {
		// Visible (not V(1)): a silent fall-back here means the model is never
		// offered the registry tools and the failure is invisible — exactly the
		// hard-to-debug trap this whole path exists to avoid.
		log.Error(err, "tool surfacing failed — registry tools will NOT reach the model",
			"reason", "readFailed", "packPath", packPath, "tools", toolNames)
		return packPath
	}

	rewritten, changed, err := injectToolsIntoPackJSON(data, toolNames)
	if err != nil {
		log.Error(err, "tool surfacing failed — registry tools will NOT reach the model",
			"reason", "parseFailed", "packPath", packPath, "tools", toolNames)
		return packPath
	}
	if !changed {
		// The pack already lists every registry tool — nothing to surface.
		log.Info("registry tools already surfaced in pack", "tools", toolNames)
		return packPath
	}

	// The runtime container runs with a read-only root filesystem, so the rewrite
	// MUST land on an explicitly-writable mount (packCacheDir, an emptyDir the
	// operator provides). Writing to os.TempDir() silently fails on that rootfs.
	outPath := filepath.Join(packCacheDir(), "omnia-pack-tools.promptpack")
	if err := os.WriteFile(outPath, rewritten, 0o600); err != nil {
		log.Error(err, "tool surfacing failed — registry tools will NOT reach the model",
			"reason", "writeFailed", "outPath", outPath, "tools", toolNames)
		return packPath
	}

	log.Info("registry tools surfaced into pack prompts — now offered to the model",
		"tools", toolNames, "packPath", outPath)
	return outPath
}

// packCacheDir is the writable directory the runtime uses to stage the
// tool-surfaced pack. The operator mounts an emptyDir here (writable even under
// a read-only root filesystem); OMNIA_PACK_CACHE_DIR overrides it, and it falls
// back to the OS temp dir for host-side unit tests.
func packCacheDir() string {
	if d := os.Getenv("OMNIA_PACK_CACHE_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

// injectToolsIntoPackJSON unions toolNames into every prompt's `tools`
// allowed-list, preserving all other pack/prompt fields. It reports whether any
// prompt was modified so callers can avoid a needless rewrite.
func injectToolsIntoPackJSON(data []byte, toolNames []string) (out []byte, changed bool, err error) {
	var pack map[string]json.RawMessage
	if err = json.Unmarshal(data, &pack); err != nil {
		return nil, false, fmt.Errorf("unmarshal pack: %w", err)
	}

	rawPrompts, ok := pack["prompts"]
	if !ok {
		return data, false, nil
	}
	var prompts map[string]json.RawMessage
	if err = json.Unmarshal(rawPrompts, &prompts); err != nil {
		return nil, false, fmt.Errorf("unmarshal prompts: %w", err)
	}

	for name, rawPrompt := range prompts {
		updated, promptChanged, perr := injectToolsIntoPrompt(rawPrompt, toolNames)
		if perr != nil {
			return nil, false, fmt.Errorf("prompt %q: %w", name, perr)
		}
		if promptChanged {
			prompts[name] = updated
			changed = true
		}
	}

	if !changed {
		return data, false, nil
	}

	promptsEncoded, merr := json.Marshal(prompts)
	if merr != nil {
		return nil, false, fmt.Errorf("marshal prompts: %w", merr)
	}
	pack["prompts"] = promptsEncoded
	out, merr = json.Marshal(pack)
	if merr != nil {
		return nil, false, fmt.Errorf("marshal pack: %w", merr)
	}
	return out, true, nil
}

// promptAllowedTools reads a pack file and returns the allowed-tools list for
// the named prompt (the tools that will actually be offered to the model). Used
// for diagnostics — a mismatch between this and the registered executor tools is
// the "registered but not offered to the model" trap. Returns nil on any error.
func promptAllowedTools(packPath, promptName string) []string {
	data, err := os.ReadFile(packPath)
	if err != nil {
		return nil
	}
	var pack struct {
		Prompts map[string]struct {
			Tools []string `json:"tools"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(data, &pack); err != nil {
		return nil
	}
	if p, ok := pack.Prompts[promptName]; ok {
		return p.Tools
	}
	return nil
}

// injectToolsIntoPrompt unions toolNames into a single prompt's `tools` list,
// returning the re-encoded prompt and whether it changed. On no change it
// returns the original bytes so the caller can skip re-marshalling.
func injectToolsIntoPrompt(rawPrompt json.RawMessage, toolNames []string) (json.RawMessage, bool, error) {
	var prompt map[string]json.RawMessage
	if err := json.Unmarshal(rawPrompt, &prompt); err != nil {
		return nil, false, fmt.Errorf("unmarshal: %w", err)
	}
	merged, changed, err := mergePromptTools(prompt["tools"], toolNames)
	if err != nil {
		return nil, false, fmt.Errorf("merge tools: %w", err)
	}
	if !changed {
		return rawPrompt, false, nil
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, false, fmt.Errorf("marshal tools: %w", err)
	}
	prompt["tools"] = encoded
	out, err := json.Marshal(prompt)
	if err != nil {
		return nil, false, fmt.Errorf("marshal prompt: %w", err)
	}
	return out, true, nil
}

// mergePromptTools returns the union of a prompt's existing `tools` list and
// toolNames (existing order first, then any new names sorted for determinism),
// and whether it added anything.
func mergePromptTools(rawTools json.RawMessage, toolNames []string) ([]string, bool, error) {
	var existing []string
	if len(rawTools) > 0 {
		if err := json.Unmarshal(rawTools, &existing); err != nil {
			return nil, false, err
		}
	}

	have := make(map[string]bool, len(existing))
	for _, t := range existing {
		have[t] = true
	}

	var added []string
	for _, name := range toolNames {
		if !have[name] {
			added = append(added, name)
			have[name] = true
		}
	}
	if len(added) == 0 {
		return existing, false, nil
	}
	sort.Strings(added)
	return slices.Concat(existing, added), true, nil
}
