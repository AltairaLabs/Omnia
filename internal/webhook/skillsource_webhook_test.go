/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestSkillSourceValidator_Create_Valid(t *testing.T) {
	v := &SkillSourceValidator{}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{
			Type:      corev1alpha1.SkillSourceTypeConfigMap,
			Interval:  "1h",
			ConfigMap: &corev1alpha1.ConfigMapSource{Name: "cm"},
		},
	}
	warnings, err := v.ValidateCreate(context.Background(), src)
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestSkillSourceValidator_Create_GitMissingBlock(t *testing.T) {
	v := &SkillSourceValidator{}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeGit, Interval: "1h"},
	}
	_, err := v.ValidateCreate(context.Background(), src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.git")
}

func TestSkillSourceValidator_Create_OCIMissingBlock(t *testing.T) {
	v := &SkillSourceValidator{}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeOCI, Interval: "1h"},
	}
	_, err := v.ValidateCreate(context.Background(), src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.oci")
}

func TestSkillSourceValidator_Create_ConfigMapMissingBlock(t *testing.T) {
	v := &SkillSourceValidator{}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeConfigMap, Interval: "1h"},
	}
	_, err := v.ValidateCreate(context.Background(), src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.configMap")
}

func TestSkillSourceValidator_Update_AppliesSameRules(t *testing.T) {
	v := &SkillSourceValidator{}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeGit, Interval: "1h"},
	}
	_, err := v.ValidateUpdate(context.Background(), src, src)
	require.Error(t, err)
}

func TestSkillSourceValidator_Delete_AlwaysAllowed(t *testing.T) {
	v := &SkillSourceValidator{}
	src := &corev1alpha1.SkillSource{
		Spec: corev1alpha1.SkillSourceSpec{Type: corev1alpha1.SkillSourceTypeGit},
	}
	_, err := v.ValidateDelete(context.Background(), src)
	require.NoError(t, err)
}
