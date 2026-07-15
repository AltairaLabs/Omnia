package deploy

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

func TestPackToPromptPack(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0", Content: `{"id":"support"}`}
	want := omniav1alpha1.PromptPackObjectName("support", "1.2.0")

	pp := packToPromptPack("ns", pack, map[string]string{"env": "prod"})
	if pp.Name != want {
		t.Errorf("name = %q, want %q", pp.Name, want)
	}
	if pp.Namespace != "ns" {
		t.Errorf("namespace = %q, want ns", pp.Namespace)
	}
	if pp.Labels[packselect.Label] != "support" {
		t.Errorf("label %s = %q, want support", packselect.Label, pp.Labels[packselect.Label])
	}
	if pp.Labels["env"] != "prod" {
		t.Errorf("deploy label not propagated: %v", pp.Labels)
	}
	if pp.Spec.PackName != "support" || pp.Spec.Version != "1.2.0" {
		t.Errorf("spec = %+v", pp.Spec)
	}
	if pp.Spec.Source.Type != omniav1alpha1.PromptPackSourceTypeConfigMap {
		t.Errorf("source type = %q", pp.Spec.Source.Type)
	}
	if pp.Spec.Source.ConfigMapRef == nil || pp.Spec.Source.ConfigMapRef.Name != contentConfigMapName(want) {
		t.Errorf("configMapRef = %+v", pp.Spec.Source.ConfigMapRef)
	}
}

func TestPackContentConfigMap(t *testing.T) {
	pack := PackIntent{Name: "support", Version: "1.2.0", Content: `{"id":"support"}`}
	cm := packContentConfigMap("ns", pack, nil)
	if cm.Name != contentConfigMapName(omniav1alpha1.PromptPackObjectName("support", "1.2.0")) {
		t.Errorf("cm name = %q", cm.Name)
	}
	if cm.Data["pack.json"] != `{"id":"support"}` {
		t.Errorf("pack.json = %q", cm.Data["pack.json"])
	}
}

func TestPackToPromptPackWithSkills(t *testing.T) {
	maxActive := int32(3)
	pack := PackIntent{
		Name:    "support",
		Version: "1.2.0",
		Content: `{"id":"support"}`,
		Skills: []SkillRefIntent{
			{Source: "docs-source", Include: []string{"faq"}, MountAs: "docs"},
		},
		SkillsConfig: &SkillsConfigIntent{MaxActive: &maxActive, Selector: "tag"},
	}

	pp := packToPromptPack("ns", pack, nil)

	if len(pp.Spec.Skills) != 1 {
		t.Fatalf("skills = %+v, want 1 entry", pp.Spec.Skills)
	}
	got := pp.Spec.Skills[0]
	want := omniav1alpha1.SkillRef{Source: "docs-source", Include: []string{"faq"}, MountAs: "docs"}
	if got.Source != want.Source || got.MountAs != want.MountAs || len(got.Include) != 1 || got.Include[0] != "faq" {
		t.Errorf("skills[0] = %+v, want %+v", got, want)
	}

	if pp.Spec.SkillsConfig == nil {
		t.Fatal("skillsConfig = nil, want non-nil")
	}
	if pp.Spec.SkillsConfig.MaxActive == nil || *pp.Spec.SkillsConfig.MaxActive != maxActive {
		t.Errorf("skillsConfig.MaxActive = %v, want %d", pp.Spec.SkillsConfig.MaxActive, maxActive)
	}
	if pp.Spec.SkillsConfig.Selector != omniav1alpha1.SkillSelectorTag {
		t.Errorf("skillsConfig.Selector = %q, want %q", pp.Spec.SkillsConfig.Selector, omniav1alpha1.SkillSelectorTag)
	}

	// nil deployLabels must not panic and must still stamp the packselect label.
	if pp.Labels[packselect.Label] != "support" {
		t.Errorf("label %s = %q, want support", packselect.Label, pp.Labels[packselect.Label])
	}
}
