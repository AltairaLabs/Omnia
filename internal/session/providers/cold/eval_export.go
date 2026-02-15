/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cold

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/altairalabs/omnia/internal/session/api"
)

// JSON lines content type constant.
const jsonLinesContentType = "application/x-jsonlines"

// EvalExportRecord is a flat record for eval results exported as JSON lines.
type EvalExportRecord struct {
	ID                string           `json:"id"`
	SessionID         string           `json:"sessionId"`
	MessageID         string           `json:"messageId,omitempty"`
	AgentName         string           `json:"agentName"`
	Namespace         string           `json:"namespace"`
	PromptPackName    string           `json:"promptpackName"`
	PromptPackVersion string           `json:"promptpackVersion,omitempty"`
	EvalID            string           `json:"evalId"`
	EvalType          string           `json:"evalType"`
	Trigger           string           `json:"trigger"`
	Passed            bool             `json:"passed"`
	Score             *float64         `json:"score,omitempty"`
	Details           *json.RawMessage `json:"details,omitempty"`
	DurationMs        *int             `json:"durationMs,omitempty"`
	JudgeTokens       *int             `json:"judgeTokens,omitempty"`
	JudgeCostUSD      *float64         `json:"judgeCostUsd,omitempty"`
	Source            string           `json:"source"`
	CreatedAt         time.Time        `json:"createdAt"`
}

// ExportEvalResults retrieves eval results for the given session IDs
// from the EvalStore and returns them as export records.
func ExportEvalResults(
	ctx context.Context, sessionIDs []string, store api.EvalStore,
) ([]EvalExportRecord, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}

	var records []EvalExportRecord
	for _, sid := range sessionIDs {
		batch, err := exportSessionEvals(ctx, sid, store)
		if err != nil {
			return nil, err
		}
		records = append(records, batch...)
	}
	return records, nil
}

// exportSessionEvals fetches and converts eval results for a single session.
func exportSessionEvals(
	ctx context.Context, sessionID string, store api.EvalStore,
) ([]EvalExportRecord, error) {
	results, err := store.GetSessionEvalResults(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get eval results for session %s: %w", sessionID, err)
	}

	records := make([]EvalExportRecord, len(results))
	for i, r := range results {
		records[i] = evalResultToRecord(r)
	}
	return records, nil
}

// evalResultToRecord converts an EvalResult to an EvalExportRecord.
func evalResultToRecord(r *api.EvalResult) EvalExportRecord {
	rec := EvalExportRecord{
		ID:                r.ID,
		SessionID:         r.SessionID,
		MessageID:         r.MessageID,
		AgentName:         r.AgentName,
		Namespace:         r.Namespace,
		PromptPackName:    r.PromptPackName,
		PromptPackVersion: r.PromptPackVersion,
		EvalID:            r.EvalID,
		EvalType:          r.EvalType,
		Trigger:           r.Trigger,
		Passed:            r.Passed,
		Score:             r.Score,
		DurationMs:        r.DurationMs,
		JudgeTokens:       r.JudgeTokens,
		JudgeCostUSD:      r.JudgeCostUSD,
		Source:            r.Source,
		CreatedAt:         r.CreatedAt,
	}
	if len(r.Details) > 0 {
		d := r.Details
		rec.Details = &d
	}
	return rec
}

// WriteEvalExport writes eval export records as JSON lines to the object store.
func (p *Provider) WriteEvalExport(
	ctx context.Context, records []EvalExportRecord, opts EvalExportOpts,
) error {
	if len(records) == 0 {
		return nil
	}

	prefix := p.prefix
	if opts.BasePath != "" {
		prefix = opts.BasePath
	}

	data, err := marshalJSONLines(records)
	if err != nil {
		return fmt.Errorf("marshal eval records: %w", err)
	}

	key := evalExportKey(prefix, records[0].CreatedAt)
	return p.store.Put(ctx, key, data, jsonLinesContentType)
}

// EvalExportOpts configures eval result export.
type EvalExportOpts struct {
	// BasePath overrides the default object key prefix.
	BasePath string
}

// evalExportKey returns the object key for an eval export file.
func evalExportKey(prefix string, t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf(
		"%sevals/year=%04d/month=%02d/day=%02d/eval_results.jsonl",
		prefix, t.Year(), int(t.Month()), t.Day(),
	)
}

// marshalJSONLines serializes records into newline-delimited JSON.
func marshalJSONLines(records []EvalExportRecord) ([]byte, error) {
	// Pre-allocate assuming ~256 bytes per JSON record + newline.
	const estimatedRecordSize = 256
	buf := make([]byte, 0, len(records)*estimatedRecordSize)
	for i := range records {
		line, err := json.Marshal(&records[i])
		if err != nil {
			return nil, fmt.Errorf("marshal record %s: %w", records[i].ID, err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	return buf, nil
}
