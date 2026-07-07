package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func promptTools(t *testing.T, packJSON []byte, prompt string) []string {
	t.Helper()
	var pack struct {
		Prompts map[string]struct {
			Tools []string `json:"tools"`
		} `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal(packJSON, &pack))
	return pack.Prompts[prompt].Tools
}

func TestInjectToolsIntoPackJSON_AddsToPromptWithNoTools(t *testing.T) {
	in := []byte(`{"id":"p","prompts":{"default":{"id":"default","system_template":"hi"}}}`)
	out, changed, err := injectToolsIntoPackJSON(in, []string{"echo"})
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []string{"echo"}, promptTools(t, out, "default"))
}

func TestInjectToolsIntoPackJSON_UnionsWithExistingTools(t *testing.T) {
	in := []byte(`{"id":"p","prompts":{"default":{"id":"default","tools":["foo"]}}}`)
	out, changed, err := injectToolsIntoPackJSON(in, []string{"echo", "foo"})
	require.NoError(t, err)
	assert.True(t, changed)
	// Existing order first, new names appended (sorted); "foo" already present.
	assert.Equal(t, []string{"foo", "echo"}, promptTools(t, out, "default"))
}

func TestInjectToolsIntoPackJSON_NoChangeWhenAllPresent(t *testing.T) {
	in := []byte(`{"id":"p","prompts":{"default":{"id":"default","tools":["echo"]}}}`)
	out, changed, err := injectToolsIntoPackJSON(in, []string{"echo"})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, in, out)
}

func TestInjectToolsIntoPackJSON_MultiPromptAndPreservesFields(t *testing.T) {
	in := []byte(`{"id":"p","name":"pack","prompts":{` +
		`"a":{"id":"a","system_template":"A","tools":["x"]},` +
		`"b":{"id":"b","system_template":"B"}}}`)
	out, changed, err := injectToolsIntoPackJSON(in, []string{"echo"})
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []string{"x", "echo"}, promptTools(t, out, "a"))
	assert.Equal(t, []string{"echo"}, promptTools(t, out, "b"))

	// Unrelated fields survive.
	var pack map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &pack))
	assert.JSONEq(t, `"pack"`, string(pack["name"]))
}

func TestInjectToolsIntoPackJSON_NoPromptsIsNoop(t *testing.T) {
	in := []byte(`{"id":"p"}`)
	out, changed, err := injectToolsIntoPackJSON(in, []string{"echo"})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, in, out)
}

func TestInjectToolsIntoPackJSON_InvalidJSON(t *testing.T) {
	_, _, err := injectToolsIntoPackJSON([]byte(`not json`), []string{"echo"})
	assert.Error(t, err)
}

func TestInjectToolsIntoPackJSON_PromptToolsWrongType(t *testing.T) {
	in := []byte(`{"prompts":{"default":{"tools":"not-an-array"}}}`)
	_, _, err := injectToolsIntoPackJSON(in, []string{"echo"})
	assert.Error(t, err)
}

func TestSurfaceRegistryToolsInPack_EmptyInputs(t *testing.T) {
	assert.Equal(t, "", surfaceRegistryToolsInPack("", []string{"echo"}, logr.Discard()))
	assert.Equal(t, "/p", surfaceRegistryToolsInPack("/p", nil, logr.Discard()))
}

func TestSurfaceRegistryToolsInPack_MissingFileReturnsOriginal(t *testing.T) {
	assert.Equal(t, "/nope/pack.json",
		surfaceRegistryToolsInPack("/nope/pack.json", []string{"echo"}, logr.Discard()))
}

func TestSurfaceRegistryToolsInPack_WritesRewrittenPack(t *testing.T) {
	dir := t.TempDir()
	packPath := filepath.Join(dir, "pack.promptpack")
	require.NoError(t, os.WriteFile(packPath,
		[]byte(`{"id":"p","prompts":{"default":{"id":"default","system_template":"hi"}}}`), 0o600))

	out := surfaceRegistryToolsInPack(packPath, []string{"echo"}, logr.Discard())
	require.NotEqual(t, packPath, out)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, []string{"echo"}, promptTools(t, data, "default"))
}

func TestSurfaceRegistryToolsInPack_WriteFailureReturnsOriginal(t *testing.T) {
	dir := t.TempDir()
	packPath := filepath.Join(dir, "pack.promptpack")
	require.NoError(t, os.WriteFile(packPath,
		[]byte(`{"id":"p","prompts":{"default":{"id":"default","system_template":"hi"}}}`), 0o600))

	// Occupy the rewrite output path with a directory so os.WriteFile fails.
	outPath := filepath.Join(os.TempDir(), "omnia-pack-tools.promptpack")
	require.NoError(t, os.RemoveAll(outPath))
	require.NoError(t, os.Mkdir(outPath, 0o700))
	t.Cleanup(func() { _ = os.RemoveAll(outPath) })

	got := surfaceRegistryToolsInPack(packPath, []string{"echo"}, logr.Discard())
	assert.Equal(t, packPath, got)
}

func TestSurfaceRegistryToolsInPack_NoChangeReturnsOriginal(t *testing.T) {
	dir := t.TempDir()
	packPath := filepath.Join(dir, "pack.promptpack")
	require.NoError(t, os.WriteFile(packPath,
		[]byte(`{"id":"p","prompts":{"default":{"id":"default","tools":["echo"]}}}`), 0o600))

	out := surfaceRegistryToolsInPack(packPath, []string{"echo"}, logr.Discard())
	assert.Equal(t, packPath, out)
}
