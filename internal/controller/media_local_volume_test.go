package controller

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/media"
	corev1 "k8s.io/api/core/v1"
)

func localStorageAR(basePath, claim string) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Media: &omniav1alpha1.MediaConfig{
				Storage: &omniav1alpha1.MediaStorageConfig{
					Type:  string(media.BackendTypeLocal),
					Local: &omniav1alpha1.LocalMediaBackend{BasePath: basePath, VolumeClaim: claim},
				},
			},
		},
	}
}

func hasMediaMount(mounts []corev1.VolumeMount, name, path string) bool {
	for _, m := range mounts {
		if m.Name == name && m.MountPath == path {
			return true
		}
	}
	return false
}

func TestMediaLocalVolume_MountedIntoBothContainers(t *testing.T) {
	ar := localStorageAR("/data/media", "media-rwx")
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}

	vol := mediaLocalVolume(ar)
	if vol == nil {
		t.Fatal("mediaLocalVolume is nil, want a PVC volume")
	}
	if vol.PersistentVolumeClaim == nil || vol.PersistentVolumeClaim.ClaimName != "media-rwx" {
		t.Fatalf("volume PVC claim = %+v, want media-rwx", vol.PersistentVolumeClaim)
	}
	if !hasMediaMount(r.buildFacadeVolumeMounts(ar, pp), mediaLocalVolumeName, "/data/media") {
		t.Error("facade container missing media volume mount at /data/media")
	}
	if !hasMediaMount(r.buildRuntimeVolumeMounts(ar, pp, nil), mediaLocalVolumeName, "/data/media") {
		t.Error("runtime container missing media volume mount at /data/media")
	}
}

func TestMediaLocalVolume_NoneWhenUnset(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	cases := map[string]*omniav1alpha1.AgentRuntime{
		"nil media":      {},
		"local no claim": localStorageAR("/data/media", ""),
		"s3 backend":     {Spec: omniav1alpha1.AgentRuntimeSpec{Media: &omniav1alpha1.MediaConfig{Storage: &omniav1alpha1.MediaStorageConfig{Type: "s3"}}}},
	}
	for name, ar := range cases {
		if mediaLocalVolume(ar) != nil {
			t.Errorf("%s: expected no media volume", name)
		}
		if hasMediaMount(r.buildFacadeVolumeMounts(ar, pp), mediaLocalVolumeName, "/data/media") {
			t.Errorf("%s: facade should have no media mount", name)
		}
	}
}
