package agent

import (
	"testing"

	"github.com/altairalabs/omnia/internal/facade"
)

// The facade only probes the context store when its handler implements
// ResumeProber. If RuntimeHandler ever stops satisfying it, resume silently
// reverts to accepting any session id — the exact failure #1876 describes.
func TestRuntimeHandlerImplementsResumeProber(t *testing.T) {
	var h any = &RuntimeHandler{}
	if _, ok := h.(facade.ResumeProber); !ok {
		t.Fatal("*RuntimeHandler does not implement facade.ResumeProber")
	}
}
