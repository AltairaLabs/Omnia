package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPodOverridesDeepCopy(t *testing.T) {
	in := &PodOverrides{
		ServiceAccountName: "csi-sa",
		Labels:             map[string]string{"azure.workload.identity/use": "true"},
		Annotations:        map[string]string{"sidecar.istio.io/inject": "false"},
		NodeSelector:       map[string]string{"gpu": "nvidia-a100"},
		Tolerations: []corev1.Toleration{{
			Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists,
		}},
		Affinity:                  &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}},
		PriorityClassName:         "high-priority",
		TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{MaxSkew: 1}},
		ImagePullSecrets:          []corev1.LocalObjectReference{{Name: "regcred"}},
		ExtraEnv:                  []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
		ExtraEnvFrom:              []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "kv-secret"}}}},
		ExtraVolumes:              []corev1.Volume{{Name: "kv"}},
		ExtraVolumeMounts:         []corev1.VolumeMount{{Name: "kv", MountPath: "/mnt/kv"}},
	}

	out := in.DeepCopy()
	if out == in {
		t.Fatal("DeepCopy returned same pointer")
	}
	out.Labels["mutated"] = "yes"
	if _, ok := in.Labels["mutated"]; ok {
		t.Fatal("DeepCopy did not isolate Labels map")
	}
	out.Tolerations[0].Key = "mutated"
	if in.Tolerations[0].Key == "mutated" {
		t.Fatal("DeepCopy did not isolate Tolerations slice")
	}
}

func TestPodOverridesNilDeepCopy(t *testing.T) {
	var in *PodOverrides
	out := in.DeepCopy()
	if out != nil {
		t.Fatalf("nil.DeepCopy() should return nil, got %v", out)
	}
}
