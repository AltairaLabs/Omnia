/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// defaultRetentionBatchSize mirrors the CRD's default BatchSize.
const defaultRetentionBatchSize int32 = 1000

// RetentionWorker composites TTL and LRU pruning across the three
// memory tiers, driven by a MemoryPolicy CRD. Each run is
// one pass per (tier, branch); rows are soft-deleted first then
// hard-deleted in a separate pass once the policy's grace window
// has elapsed.
//
// Phase 3 ships TTL + LRU. Decay is recognised by the policy but
// logged as not-yet-implemented until a follow-up wires the score
// formula.
type RetentionWorker struct {
	store  *PostgresMemoryStore
	loader PolicyLoader
	log    logr.Logger

	// testHook fires at the end of every run so tests can synchronise
	// without sleeping.
	testHook func()
}

// NewRetentionWorker wires the composite worker. The loader is
// typically a K8sPolicyLoader in production and a StaticPolicyLoader
// in tests.
func NewRetentionWorker(store *PostgresMemoryStore, loader PolicyLoader, log logr.Logger) *RetentionWorker {
	return &RetentionWorker{
		store:  store,
		loader: loader,
		log:    log,
	}
}

// Run blocks until ctx is cancelled, firing a pass on every tick of
// the policy's cron schedule. The schedule is re-read from the loader
// at startup and cached; policy changes land on the next worker
// restart.
func (w *RetentionWorker) Run(ctx context.Context) {
	policy, err := w.loader.Load(ctx)
	if err != nil || policy == nil {
		w.log.Info("retention worker not started",
			"reason", "no active MemoryPolicy",
			"error", errString(err))
		return
	}
	schedule := policy.Spec.Default.Schedule
	if schedule == "" {
		w.log.Info("retention worker not started", "reason", "policy has no schedule")
		return
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	sched, err := parser.Parse(schedule)
	if err != nil {
		w.log.Error(err, "retention worker not started", "reason", "invalid cron", "schedule", schedule)
		return
	}

	w.log.Info("retention worker started", "schedule", schedule)
	next := sched.Next(time.Now())
	for {
		wait := time.Until(next)
		if wait <= 0 {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			w.log.Info("retention worker stopped")
			return
		case <-timer.C:
			w.runOnce(ctx)
			next = sched.Next(time.Now())
		}
	}
}

// runOnce executes a full pass — load policy, iterate tiers, run
// applicable branches, then hard-delete past grace.
func (w *RetentionWorker) runOnce(ctx context.Context) {
	start := time.Now()
	metrics := defaultRetentionMetrics.Load()
	defer func() {
		if w.testHook != nil {
			w.testHook()
		}
	}()

	policy, err := w.loader.Load(ctx)
	if err != nil || policy == nil {
		metrics.observeRun(time.Since(start), false)
		w.log.V(1).Info("retention pass skipped",
			"reason", "no policy available",
			"error", errString(err))
		return
	}

	batchSize := resolveBatchSize(policy)
	anyErr := false

	for _, tier := range retentionTiers() {
		cfg := tierConfig(policy, tier)
		if cfg == nil {
			continue
		}
		branches := branchesForMode(cfg.Mode)
		for _, branch := range branches {
			if err := w.runBranch(ctx, tier, branch, cfg, batchSize); err != nil {
				anyErr = true
				metrics.observeBranchError(tier, branch)
				w.log.Error(err, "retention branch failed",
					"tier", string(tier),
					"branch", string(branch))
			}
		}
	}

	cascadeSoft, cascadeHard, cascadeErr := w.runConsentRevocation(ctx,
		policy.Spec.Default.ConsentRevocation, batchSize)
	if cascadeErr != nil {
		anyErr = true
	}

	superseded, supersessionErr := w.runSupersessionCleanup(ctx,
		policy.Spec.Default.Supersession, batchSize)
	if supersessionErr != nil {
		anyErr = true
	}

	hard, err := w.store.HardDeleteForgottenOlderThan(ctx,
		resolveGraceDays(policy.Spec.Default.ConsentRevocation), int(batchSize))
	if err != nil {
		anyErr = true
		w.log.Error(err, "retention hard-delete failed")
	} else {
		metrics.observeHardDelete(hard)
	}

	metrics.observeRun(time.Since(start), !anyErr)
	w.log.V(1).Info("retention pass finished",
		"duration", time.Since(start).String(),
		"hardDeleted", hard,
		"consentSoftDeleted", cascadeSoft,
		"consentHardDeleted", cascadeHard,
		"supersededObservations", superseded,
		"ok", !anyErr)
}

// runSupersessionCleanup hard-deletes observations that have been
// superseded by a temporal summary once the policy's grace window
// has elapsed. The branch is a no-op when the policy's
// supersession.enabled flag is false (the default) — operators opt
// in after confirming their summarizer agent is producing summaries
// they're willing to commit to.
//
// Superseded observations are already hidden from retrieval via the
// `superseded_by IS NULL` filter in retrieve_multi_tier.go, so this
// branch only reclaims storage and does not change API behaviour.
func (w *RetentionWorker) runSupersessionCleanup(
	ctx context.Context,
	cfg *omniav1alpha1.MemorySupersessionConfig,
	batchSize int32,
) (int64, error) {
	if cfg == nil || !cfg.Enabled {
		return 0, nil
	}
	metrics := defaultRetentionMetrics.Load()
	grace := resolveSupersessionGraceDays(cfg)
	n, err := w.store.HardDeleteSupersededObservations(ctx, grace, int(batchSize))
	if err != nil {
		metrics.observeBranchError(TierInstitutional, BranchSupersession)
		w.log.Error(err, "supersession cleanup failed")
		return 0, err
	}
	metrics.observeHardDelete(n)
	if n > 0 {
		w.log.Info("memory supersession cleanup",
			"observationsDeleted", n,
			"graceDays", grace,
		)
	}
	return n, nil
}

// resolveSupersessionGraceDays pulls the grace window from the
// policy, defaulting to the CRD's documented default (14 days).
func resolveSupersessionGraceDays(cfg *omniav1alpha1.MemorySupersessionConfig) int32 {
	if cfg != nil && cfg.GraceDays != nil {
		return *cfg.GraceDays
	}
	return 14
}

// runConsentRevocation cascades user consent revocations to memory
// rows. For action=SoftDelete it flips forgotten=true with
// forgotten_at=now so the grace-period pass later hard-deletes. For
// action=HardDelete it removes rows immediately. action=Stop is a
// no-op — operators chose it explicitly to keep existing rows.
//
// Returns (softCount, hardCount, err) so callers can log totals
// without re-querying.
func (w *RetentionWorker) runConsentRevocation(
	ctx context.Context,
	cfg *omniav1alpha1.MemoryConsentRevocationConfig,
	batchSize int32,
) (int64, int64, error) {
	metrics := defaultRetentionMetrics.Load()
	action := resolveConsentAction(cfg)
	switch action {
	case omniav1alpha1.ConsentRevocationStop:
		return 0, 0, nil
	case omniav1alpha1.ConsentRevocationHardDelete:
		n, err := w.store.HardDeleteRevokedConsent(ctx, int(batchSize))
		if err != nil {
			metrics.observeBranchError(TierUser, BranchConsentRevoke)
			w.log.Error(err, "consent revocation hard-delete failed")
			return 0, 0, err
		}
		metrics.observeHardDelete(n)
		if n > 0 {
			w.emitConsentAudit(ctx, action, 0, n)
		}
		return 0, n, nil
	}

	// SoftDelete path (default).
	soft, err := w.store.SoftDeleteRevokedConsent(ctx, int(batchSize))
	if err != nil {
		metrics.observeBranchError(TierUser, BranchConsentRevoke)
		w.log.Error(err, "consent revocation soft-delete failed")
		return 0, 0, err
	}
	metrics.observeSoftDelete(TierUser, BranchConsentRevoke, soft)

	// Hard-delete rows whose consent-driven soft-delete grace has
	// elapsed. Keyed on forgotten_at so we don't double-count rows
	// whose forgotten=true came from TTL/LRU elsewhere.
	hard, err := w.store.HardDeleteForgottenByConsentOlderThan(ctx,
		resolveGraceDays(cfg), int(batchSize))
	if err != nil {
		metrics.observeBranchError(TierUser, BranchConsentHardClean)
		w.log.Error(err, "consent revocation grace hard-delete failed")
		return soft, 0, err
	}
	metrics.observeHardDelete(hard)

	if soft > 0 || hard > 0 {
		w.emitConsentAudit(ctx, action, soft, hard)
	}
	return soft, hard, nil
}

// resolveConsentAction returns the policy's action, defaulting to
// SoftDelete so absent config doesn't silently skip the cascade.
func resolveConsentAction(cfg *omniav1alpha1.MemoryConsentRevocationConfig) omniav1alpha1.ConsentRevocationAction {
	if cfg != nil && cfg.Action != "" {
		return cfg.Action
	}
	return omniav1alpha1.ConsentRevocationSoftDelete
}

// emitConsentAudit records a consent-cascade event. Audit output is
// best-effort — failure here must not abort the retention cycle, so
// we stick to structured log output. The operator's Prometheus
// omnia_memory_retention_soft_deleted_total / _hard_deleted_total
// counters carry the same signal and drive alerting.
func (w *RetentionWorker) emitConsentAudit(
	_ context.Context,
	action omniav1alpha1.ConsentRevocationAction,
	soft, hard int64,
) {
	w.log.Info("memory retention consent cascade",
		"action", string(action),
		"softDeleted", soft,
		"hardDeleted", hard,
	)
}

// runBranch dispatches one (tier, branch) pair to the appropriate
// store call.
func (w *RetentionWorker) runBranch(
	ctx context.Context,
	tier Tier,
	branch RetentionBranch,
	cfg *omniav1alpha1.MemoryTierConfig,
	batchSize int32,
) error {
	metrics := defaultRetentionMetrics.Load()
	switch branch {
	case BranchTTL:
		n, err := w.store.SoftDeleteExpiredTTL(ctx, tier, int(batchSize))
		if err != nil {
			return err
		}
		metrics.observeSoftDelete(tier, branch, n)
		return nil
	case BranchLRU:
		stale, err := resolveStaleAfter(cfg.LRU)
		if err != nil || stale <= 0 {
			return err
		}
		n, err := w.store.SoftDeleteLRU(ctx, tier, stale, int(batchSize))
		if err != nil {
			return err
		}
		metrics.observeSoftDelete(tier, branch, n)
		return nil
	case BranchDecay:
		// Phase 3 defers the Decay formula to a follow-up. Log once
		// per run so operators notice configured Decay policies aren't
		// yet enforced.
		w.log.V(1).Info("decay branch not yet implemented", "tier", string(tier))
		return nil
	}
	return fmt.Errorf("unknown branch %q", branch)
}

// resolveBatchSize pulls the policy's BatchSize, falling back to the
// CRD default when nil.
func resolveBatchSize(policy *omniav1alpha1.MemoryPolicy) int32 {
	if b := policy.Spec.Default.BatchSize; b != nil && *b > 0 {
		return *b
	}
	return defaultRetentionBatchSize
}

// resolveStaleAfter parses the LRU staleAfter duration, returning zero
// and no error when unset so the caller can skip the branch.
func resolveStaleAfter(cfg *omniav1alpha1.MemoryLRUConfig) (time.Duration, error) {
	if cfg == nil {
		return 0, nil
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		return 0, nil
	}
	if cfg.StaleAfter == "" {
		return 0, nil
	}
	d, err := parseRetentionDuration(cfg.StaleAfter)
	if err != nil {
		return 0, fmt.Errorf("lru.staleAfter %q: %w", cfg.StaleAfter, err)
	}
	return d, nil
}

// resolveGraceDays returns the soft→hard delete grace window from the
// consent revocation config, defaulting to 7 days. The CRD keeps this
// on a consentRevocation sub-struct because Phase 4 reuses it; in
// Phase 3 we use it as the general soft-delete grace.
func resolveGraceDays(cfg *omniav1alpha1.MemoryConsentRevocationConfig) int32 {
	if cfg != nil && cfg.GraceDays != nil {
		return *cfg.GraceDays
	}
	return 7
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
