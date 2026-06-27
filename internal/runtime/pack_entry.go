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
	"os"

	"github.com/go-logr/logr"
)

// packEntryFields is the subset of a compiled pack.json needed to resolve the
// runtime's entry point. Read with a local struct so this does not depend on the
// PromptKit pack type (internal/runtime must compile against the published SDK).
type packEntryFields struct {
	Workflow *packGraphEntry            `json:"workflow"`
	Agents   *packGraphEntry            `json:"agents"`
	Prompts  map[string]json.RawMessage `json:"prompts"`
}

// packGraphEntry is the shared shape of a workflow / multi-agent pack's declared
// entry point.
type packGraphEntry struct {
	Entry string `json:"entry"`
}

// ResolvePackEntry returns the prompt/state the runtime should open the pack at,
// read from the pack the PromptPack references rather than a hardcoded name
// (#1595). Resolution order:
//
//  1. workflow pack    → workflow.entry
//  2. multi-agent pack → agents.entry
//  3. plain pack with exactly one prompt → that prompt
//  4. otherwise        → fallback (the configured/"default" name)
//
// The fallback only applies to multi-prompt plain packs, which must name a
// prompt accordingly (or declare an entry in the pack format). On an
// unreadable/unparseable pack it returns the fallback unchanged and lets the
// subsequent sdk.Open surface the real error.
func ResolvePackEntry(packPath, fallback string, log logr.Logger) string {
	data, err := os.ReadFile(packPath)
	if err != nil {
		log.V(1).Info("pack entry resolution skipped", "reason", "readFailed",
			"packPath", packPath, "fallback", fallback)
		return fallback
	}
	var pack packEntryFields
	if err := json.Unmarshal(data, &pack); err != nil {
		log.V(1).Info("pack entry resolution skipped", "reason", "parseFailed",
			"fallback", fallback)
		return fallback
	}

	if pack.Workflow != nil && pack.Workflow.Entry != "" {
		log.V(1).Info("pack entry resolved", "source", "workflow.entry", "entry", pack.Workflow.Entry)
		return pack.Workflow.Entry
	}
	if pack.Agents != nil && pack.Agents.Entry != "" {
		log.V(1).Info("pack entry resolved", "source", "agents.entry", "entry", pack.Agents.Entry)
		return pack.Agents.Entry
	}
	if len(pack.Prompts) == 1 {
		for name := range pack.Prompts {
			log.V(1).Info("pack entry resolved", "source", "solePrompt", "entry", name)
			return name
		}
	}

	log.V(1).Info("pack entry resolved", "source", "fallback", "entry", fallback,
		"promptCount", len(pack.Prompts))
	return fallback
}
