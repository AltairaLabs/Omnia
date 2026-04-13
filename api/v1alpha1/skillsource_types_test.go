/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestSkillSourceRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("add to scheme: %v", err)
	}

	gvk := GroupVersion.WithKind("SkillSource")
	obj, err := scheme.New(gvk)
	if err != nil {
		t.Fatalf("SkillSource not registered: %v", err)
	}
	if _, ok := obj.(*SkillSource); !ok {
		t.Fatalf("registered type is not *SkillSource: %T", obj)
	}

	gvkList := GroupVersion.WithKind("SkillSourceList")
	if _, err := scheme.New(gvkList); err != nil {
		t.Fatalf("SkillSourceList not registered: %v", err)
	}
}
