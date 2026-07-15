package deploy

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

const packJSONKey = "pack.json"

// contentConfigMapName is the deterministic name of the ConfigMap holding a
// PromptPack's compiled pack.json, derived from the pack's object name.
func contentConfigMapName(packObjectName string) string {
	return packObjectName + "-content"
}

// mergeLabels returns base with the deploy-wide labels overlaid (deploy labels
// never override the reserved keys base sets).
func mergeLabels(base, deploy map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range deploy {
		out[k] = v
	}
	for k, v := range base {
		out[k] = v
	}
	return out
}

// packToPromptPack builds the immutable PromptPack object for {pack.Name,
// pack.Version}. The name is deterministic so a duplicate coordinate is rejected
// natively by the apiserver (AlreadyExists).
func packToPromptPack(namespace string, pack PackIntent, deployLabels map[string]string) *omniav1alpha1.PromptPack {
	name := omniav1alpha1.PromptPackObjectName(pack.Name, pack.Version)
	return &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: pack.Name,
			Version:  pack.Version,
			Source: omniav1alpha1.PromptPackContentSource{
				Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{Name: contentConfigMapName(name)},
			},
			Skills:       skillRefs(pack.Skills),
			SkillsConfig: skillsConfig(pack.SkillsConfig),
		},
	}
}

// packContentConfigMap builds the ConfigMap holding the raw pack.json.
func packContentConfigMap(namespace string, pack PackIntent, deployLabels map[string]string) *corev1.ConfigMap {
	name := omniav1alpha1.PromptPackObjectName(pack.Name, pack.Version)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      contentConfigMapName(name),
			Namespace: namespace,
			Labels:    mergeLabels(map[string]string{packselect.Label: pack.Name}, deployLabels),
		},
		Data: map[string]string{packJSONKey: pack.Content},
	}
}

func skillRefs(in []SkillRefIntent) []omniav1alpha1.SkillRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]omniav1alpha1.SkillRef, 0, len(in))
	for _, s := range in {
		out = append(out, omniav1alpha1.SkillRef{Source: s.Source, Include: s.Include, MountAs: s.MountAs})
	}
	return out
}

func skillsConfig(in *SkillsConfigIntent) *omniav1alpha1.SkillsConfig {
	if in == nil {
		return nil
	}
	return &omniav1alpha1.SkillsConfig{MaxActive: in.MaxActive, Selector: omniav1alpha1.SkillSelector(in.Selector)}
}
