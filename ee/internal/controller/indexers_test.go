/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestExtractSourceRef(t *testing.T) {
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: corev1alpha1.LocalObjectReference{Name: "my-source"},
		},
	}

	refs := extractSourceRef(job)
	assert.Equal(t, []string{"my-source"}, refs)
}

func TestExtractSourceRef_Empty(t *testing.T) {
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Spec:       omniav1alpha1.ArenaJobSpec{},
	}

	refs := extractSourceRef(job)
	assert.Empty(t, refs)
}
