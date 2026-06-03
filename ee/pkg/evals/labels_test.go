package evals

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvalLabelsFor(t *testing.T) {
	got := evalLabelsFor("agent-x", "ns-y", "pack-z", "candidate", []string{"g1"})

	assert.Equal(t, "agent-x", got.Agent)
	assert.Equal(t, "ns-y", got.Namespace)
	assert.Equal(t, "pack-z", got.PromptPackName)
	assert.Equal(t, "candidate", got.Variant)
	assert.Equal(t, []string{"g1"}, got.Groups)
}

func TestEvalInstanceLabels(t *testing.T) {
	got := evalInstanceLabels(EvalLabels{
		Agent:          "a",
		Namespace:      "n",
		PromptPackName: "p",
		Variant:        "candidate",
	})

	assert.Equal(t, "a", got["agent"])
	assert.Equal(t, "n", got["namespace"])
	assert.Equal(t, "p", got["promptpack_name"])
	assert.Equal(t, "candidate", got["variant"])
}
