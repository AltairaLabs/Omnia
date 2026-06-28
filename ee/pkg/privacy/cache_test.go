/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

type fakeKV struct {
	mu                           sync.Mutex
	data                         map[string]string
	getErr                       error
	delErr                       error
	getCalls, setCalls, delCalls int
}

func newFakeKV() *fakeKV { return &fakeKV{data: map[string]string{}} }

func (f *fakeKV) Get(_ context.Context, k string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.data[k]
	return v, ok, nil
}
func (f *fakeKV) Set(_ context.Context, k, v string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	f.data[k] = v
	return nil
}
func (f *fakeKV) Del(_ context.Context, k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delCalls++
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.data, k)
	return nil
}

type countingStore struct {
	prefs           *Preferences
	err             error
	setOptOutErr    error
	removeOptOutErr error
	getCalls        int
}

func (c *countingStore) GetPreferences(_ context.Context, _ string) (*Preferences, error) {
	c.getCalls++
	if c.err != nil {
		return nil, c.err
	}
	return c.prefs, nil
}
func (c *countingStore) SetOptOut(_ context.Context, _, _, _ string) error {
	return c.setOptOutErr
}
func (c *countingStore) RemoveOptOut(_ context.Context, _, _, _ string) error {
	return c.removeOptOutErr
}
func (c *countingStore) GetConsentGrants(_ context.Context, _ string) ([]ConsentCategory, error) {
	p, err := c.GetPreferences(context.Background(), "")
	if err != nil {
		return nil, err
	}
	return p.ConsentGrants, nil
}

func TestCachedStore_MissThenHit(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1", ConsentGrants: []ConsentCategory{ConsentMemoryIdentity}}}
	kv := newFakeKV()
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())

	if _, err := s.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatal(err)
	}
	if inner.getCalls != 1 {
		t.Errorf("inner called %d times, want 1 (second served from cache)", inner.getCalls)
	}
}

func TestCachedStore_ConsentFromCache(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1", ConsentGrants: []ConsentCategory{ConsentMemoryHealth}}}
	s := NewCachedPreferencesStore(inner, newFakeKV(), time.Minute, logr.Discard())
	_, _ = s.GetPreferences(context.Background(), "u1") // warm
	grants, err := s.GetConsentGrants(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 || grants[0] != ConsentMemoryHealth {
		t.Errorf("got %v", grants)
	}
	if inner.getCalls != 1 {
		t.Errorf("consent read should reuse the cached prefs, inner=%d", inner.getCalls)
	}
}

func TestCachedStore_SetOptOutBusts(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}}
	kv := newFakeKV()
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())
	_, _ = s.GetPreferences(context.Background(), "u1") // populate
	if err := s.SetOptOut(context.Background(), "u1", ScopeAll, ""); err != nil {
		t.Fatal(err)
	}
	if kv.delCalls != 1 {
		t.Errorf("SetOptOut must bust cache, delCalls=%d", kv.delCalls)
	}
}

func TestCachedStore_KVErrorFallsThrough(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}}
	kv := newFakeKV()
	kv.getErr = errors.New("redis down")
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())
	if _, err := s.GetPreferences(context.Background(), "u1"); err != nil {
		t.Fatalf("cache error must not fail the read: %v", err)
	}
	if inner.getCalls != 1 {
		t.Error("must fall through to inner on cache error")
	}
}

func TestCachedStore_NotFoundNotCached(t *testing.T) {
	inner := &countingStore{err: ErrPreferencesNotFound}
	kv := newFakeKV()
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())
	if _, err := s.GetPreferences(context.Background(), "u1"); !errors.Is(err, ErrPreferencesNotFound) {
		t.Fatalf("want ErrPreferencesNotFound, got %v", err)
	}
	if kv.setCalls != 0 {
		t.Error("not-found must not be cached")
	}
}

func TestCachedStore_RemoveOptOutBusts(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}}
	kv := newFakeKV()
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())
	_, _ = s.GetPreferences(context.Background(), "u1") // populate
	if err := s.RemoveOptOut(context.Background(), "u1", ScopeAll, ""); err != nil {
		t.Fatal(err)
	}
	if kv.delCalls != 1 {
		t.Errorf("RemoveOptOut must bust cache, delCalls=%d", kv.delCalls)
	}
}

func TestCachedStore_ConsentNotFound(t *testing.T) {
	inner := &countingStore{err: ErrPreferencesNotFound}
	s := NewCachedPreferencesStore(inner, newFakeKV(), time.Minute, logr.Discard())
	grants, err := s.GetConsentGrants(context.Background(), "u1")
	if err != nil {
		t.Fatalf("not-found must return empty slice, got error: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("expected empty grants, got %v", grants)
	}
}

func TestCachedStore_SetOptOutInnerError(t *testing.T) {
	innerErr := errors.New("db error")
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}, setOptOutErr: innerErr}
	s := NewCachedPreferencesStore(inner, newFakeKV(), time.Minute, logr.Discard())
	if err := s.SetOptOut(context.Background(), "u1", ScopeAll, ""); !errors.Is(err, innerErr) {
		t.Errorf("want inner error, got %v", err)
	}
}

func TestCachedStore_RemoveOptOutInnerError(t *testing.T) {
	innerErr := errors.New("db error")
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}, removeOptOutErr: innerErr}
	s := NewCachedPreferencesStore(inner, newFakeKV(), time.Minute, logr.Discard())
	if err := s.RemoveOptOut(context.Background(), "u1", ScopeAll, ""); !errors.Is(err, innerErr) {
		t.Errorf("want inner error, got %v", err)
	}
}

func TestCachedStore_CorruptCacheFallsThrough(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}}
	kv := newFakeKV()
	// Seed corrupt JSON directly so the unmarshal path is exercised.
	kv.data[cacheKeyPrefix+"u1"] = "not-json{"
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())
	p, err := s.GetPreferences(context.Background(), "u1")
	if err != nil {
		t.Fatalf("corrupt cache must fall through to inner: %v", err)
	}
	if p.UserID != "u1" {
		t.Errorf("expected inner prefs, got %+v", p)
	}
	if inner.getCalls != 1 {
		t.Errorf("inner must be called on corrupt cache, getCalls=%d", inner.getCalls)
	}
}

func TestCachedStore_BustDelError(t *testing.T) {
	inner := &countingStore{prefs: &Preferences{UserID: "u1"}}
	kv := newFakeKV()
	kv.delErr = errors.New("redis timeout")
	s := NewCachedPreferencesStore(inner, kv, time.Minute, logr.Discard())
	// SetOptOut should still succeed even when bust (Del) fails.
	if err := s.SetOptOut(context.Background(), "u1", ScopeAll, ""); err != nil {
		t.Fatalf("bust Del error must not propagate: %v", err)
	}
	if kv.delCalls != 1 {
		t.Errorf("Del must have been attempted, delCalls=%d", kv.delCalls)
	}
}
