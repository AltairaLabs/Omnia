/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"
)

func TestNewWorkerMetrics(t *testing.T) {
	m := testWorkerMetrics()
	require.NotNil(t, m.WorkItemsTotal)
	require.NotNil(t, m.WorkItemDuration)
}

func TestRecordWorkItem_Pass(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegisterer(reg)

	m.RecordWorkItem("job-1", statusPass, 1.5)
	m.RecordWorkItem("job-1", statusPass, 2.0)
	m.RecordWorkItem("job-1", statusFail, 0.5)

	families, err := reg.Gather()
	require.NoError(t, err)

	var itemsFamily *dto.MetricFamily
	var durationFamily *dto.MetricFamily
	for _, f := range families {
		switch f.GetName() {
		case "omnia_arena_work_items_total":
			itemsFamily = f
		case "omnia_arena_work_item_duration_seconds":
			durationFamily = f
		}
	}

	require.NotNil(t, itemsFamily, "work_items_total metric should exist")
	// Should have 2 label combos: pass and fail
	assert.Len(t, itemsFamily.GetMetric(), 2)

	// Find pass counter
	for _, metric := range itemsFamily.GetMetric() {
		for _, label := range metric.GetLabel() {
			if label.GetName() == "status" && label.GetValue() == statusPass {
				assert.Equal(t, float64(2), metric.GetCounter().GetValue())
			}
			if label.GetName() == "status" && label.GetValue() == statusFail {
				assert.Equal(t, float64(1), metric.GetCounter().GetValue())
			}
		}
	}

	require.NotNil(t, durationFamily, "work_item_duration metric should exist")
	// Should have 1 label combo (job_name=job-1)
	assert.Len(t, durationFamily.GetMetric(), 1)
	assert.Equal(t, uint64(3), durationFamily.GetMetric()[0].GetHistogram().GetSampleCount())
}

func TestStartMetricsServer(t *testing.T) {
	// Use a random available port by starting a listener first.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	// Close the listener so startMetricsServer can bind to the same port.
	_ = listener.Close()

	go startMetricsServer(addr, testLog())

	// Give the server time to start.
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/healthz")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNewMetricsMux(t *testing.T) {
	mux := newMetricsMux()

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/healthz", http.StatusOK, "ok"},
		{"/readyz", http.StatusOK, "ok"},
		{"/metrics", http.StatusOK, "go_goroutines"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantBody)
		})
	}
}
