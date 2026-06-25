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
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const entryFallback = "default"

// writePack writes pack JSON to a temp file and returns its path.
func writePack(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestResolvePackEntry(t *testing.T) {
	cases := []struct {
		name string
		pack string
		want string
	}{
		{
			name: "workflow entry wins",
			// workflow.entry is the entry even when prompts/agents also present.
			pack: `{"workflow":{"entry":"triage"},"agents":{"entry":"a"},"prompts":{"greeting":{},"default":{}}}`,
			want: "triage",
		},
		{
			name: "agents entry when no workflow",
			pack: `{"agents":{"entry":"router"},"prompts":{"greeting":{}}}`,
			want: "router",
		},
		{
			name: "sole prompt when single plain prompt not named default",
			pack: `{"prompts":{"greeting":{}}}`,
			want: "greeting",
		},
		{
			name: "fallback for multi-prompt plain pack",
			pack: `{"prompts":{"greeting":{},"farewell":{}}}`,
			want: entryFallback,
		},
		{
			name: "fallback when no prompts/workflow/agents",
			pack: `{}`,
			want: entryFallback,
		},
		{
			name: "empty workflow entry falls through to sole prompt",
			pack: `{"workflow":{"entry":""},"prompts":{"only":{}}}`,
			want: "only",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePackEntry(writePack(t, tc.pack), entryFallback, logr.Discard())
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolvePackEntry_BackCompatDefaultPack(t *testing.T) {
	// A pack that has a "default" prompt among several keeps working: the
	// multi-prompt fallback resolves to "default", which exists.
	p := writePack(t, `{"prompts":{"default":{},"other":{}}}`)
	assert.Equal(t, "default", ResolvePackEntry(p, entryFallback, logr.Discard()))
}

func TestResolvePackEntry_UnreadableReturnsFallback(t *testing.T) {
	got := ResolvePackEntry(filepath.Join(t.TempDir(), "missing.json"), entryFallback, logr.Discard())
	assert.Equal(t, entryFallback, got, "an unreadable pack must not change the entry; sdk.Open surfaces the real error")
}

func TestResolvePackEntry_UnparseableReturnsFallback(t *testing.T) {
	got := ResolvePackEntry(writePack(t, `{not json`), entryFallback, logr.Discard())
	assert.Equal(t, entryFallback, got)
}
