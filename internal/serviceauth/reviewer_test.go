/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"context"
	"errors"
	"testing"

	authnv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

const testSubject = "system:serviceaccount:omnia-system:omnia-session-api"

func TestNewK8sTokenReviewer(t *testing.T) {
	r, err := NewK8sTokenReviewer(&rest.Config{Host: "https://example.invalid"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil reviewer")
	}
}

func TestK8sTokenReviewer_ReviewToken(t *testing.T) {
	t.Run("authenticated returns username", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
			tr := action.(k8stesting.CreateAction).GetObject().(*authnv1.TokenReview)
			tr.Status = authnv1.TokenReviewStatus{Authenticated: true, User: authnv1.UserInfo{Username: testSubject}}
			return true, tr, nil
		})
		r := &k8sTokenReviewer{client: cs}
		ok, user, err := r.ReviewToken(context.Background(), "tok")
		if err != nil || !ok || user != testSubject {
			t.Fatalf("got ok=%v user=%q err=%v", ok, user, err)
		}
	})

	t.Run("audiences are forwarded on the spec", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		var gotAudiences []string
		cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
			tr := action.(k8stesting.CreateAction).GetObject().(*authnv1.TokenReview)
			gotAudiences = tr.Spec.Audiences
			tr.Status = authnv1.TokenReviewStatus{Authenticated: true, User: authnv1.UserInfo{Username: testSubject}}
			return true, tr, nil
		})
		r := &k8sTokenReviewer{client: cs, audiences: []string{"omnia-session-api"}}
		if _, _, err := r.ReviewToken(context.Background(), "tok"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gotAudiences) != 1 || gotAudiences[0] != "omnia-session-api" {
			t.Fatalf("audiences not forwarded: %v", gotAudiences)
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
			tr := action.(k8stesting.CreateAction).GetObject().(*authnv1.TokenReview)
			tr.Status = authnv1.TokenReviewStatus{Authenticated: false}
			return true, tr, nil
		})
		r := &k8sTokenReviewer{client: cs}
		ok, _, err := r.ReviewToken(context.Background(), "tok")
		if err != nil || ok {
			t.Fatalf("got ok=%v err=%v, want ok=false err=nil", ok, err)
		}
	})

	t.Run("status error surfaces as error", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
			tr := action.(k8stesting.CreateAction).GetObject().(*authnv1.TokenReview)
			tr.Status = authnv1.TokenReviewStatus{Error: "token expired"}
			return true, tr, nil
		})
		r := &k8sTokenReviewer{client: cs}
		if _, _, err := r.ReviewToken(context.Background(), "tok"); err == nil {
			t.Fatal("expected error from status.Error")
		}
	})

	t.Run("api error surfaces as error", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("create", "tokenreviews", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("boom")
		})
		r := &k8sTokenReviewer{client: cs}
		if _, _, err := r.ReviewToken(context.Background(), "tok"); err == nil {
			t.Fatal("expected error from API failure")
		}
	})
}
