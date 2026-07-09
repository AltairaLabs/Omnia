/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/altairalabs/omnia/ee/pkg/license"
)

func TestRecordActivationFailure_ExponentialBackoffAndCap(t *testing.T) {
	r := &LicenseActivationReconciler{}
	want := []time.Duration{1 * time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}
	for i, w := range want {
		delay, first, gaveUp := r.recordActivationFailure("lic")
		assert.Equal(t, w, delay, "attempt %d", i+1)
		assert.Equal(t, i == 0, first, "first flag on attempt %d", i+1)
		assert.False(t, gaveUp)
	}
	// Keep failing; the delay must be capped, never unbounded.
	var delay time.Duration
	for i := 0; i < 20; i++ {
		delay, _, _ = r.recordActivationFailure("lic")
	}
	assert.Equal(t, activationBackoffCap, delay, "delay should be capped")
}

func TestRecordActivationFailure_GivesUpToSlowInterval(t *testing.T) {
	r := &LicenseActivationReconciler{
		activationFailures: map[string]*activationBackoff{
			"lic": {firstFailure: time.Now().Add(-(activationGiveUpAfter + time.Hour)), attempts: 30},
		},
	}
	delay, first, gaveUp := r.recordActivationFailure("lic")
	assert.Equal(t, activationSlowInterval, delay)
	assert.False(t, first)
	assert.True(t, gaveUp, "the first call past the give-up window reports the transition")

	delay, _, gaveUp = r.recordActivationFailure("lic")
	assert.Equal(t, activationSlowInterval, delay)
	assert.False(t, gaveUp, "later slow retries must not re-report give-up")
}

func TestResetActivationBackoff(t *testing.T) {
	r := &LicenseActivationReconciler{}
	r.recordActivationFailure("lic")
	r.recordActivationFailure("lic")
	r.resetActivationBackoff("lic")
	_, first, _ := r.recordActivationFailure("lic")
	assert.True(t, first, "after reset the streak starts over")
}

func TestHandleActivationFailure_EmitsEventOnceAndBacksOff(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: license.LicenseSecretName, Namespace: license.LicenseSecretNamespace,
	}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	rec := record.NewFakeRecorder(10)
	r := &LicenseActivationReconciler{Client: c, Recorder: rec}

	res := r.handleActivationFailure(context.Background(), "lic", "ActivationFailed", "boom")
	assert.Equal(t, activationBackoffBase, res.RequeueAfter)

	res = r.handleActivationFailure(context.Background(), "lic", "ActivationFailed", "boom")
	assert.Equal(t, 2*time.Minute, res.RequeueAfter, "second failure backs off further")

	// A single Warning event across the two failures — not one per cycle.
	assert.Len(t, rec.Events, 1)
}
