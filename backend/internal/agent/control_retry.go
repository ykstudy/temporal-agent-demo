package agent

import (
	"context"
	"encoding/json"

	"agentstudio/internal/contracts"
)

// This private creation provenance is included in the immutable creation
// fingerprint and the first decision context. It carries content, no authority.
type controlRetryOrigin struct {
	RunID                  string                 `json:"run_id"`
	SourceVersion          int                    `json:"source_version"`
	DefinitionSnapshotHash string                 `json:"definition_snapshot_hash"`
	Command                Command                `json:"command"`
	Plan                   *contracts.ProjectPlan `json:"plan"`
}

func (s *Service) initializeControlRetry(r *Run, origin controlRetryOrigin) error {
	if r.Control == nil || r.RetryOf != origin.RunID || origin.Command.RunID != origin.RunID || origin.Command.Action != "retry" || origin.RunID == r.ID {
		return fail("retry_binding_mismatch", "重做来源与准确终态命令不符")
	}
	if origin.Plan != nil {
		if !hashMatches("ProjectPlan", *origin.Plan, origin.Plan.PlanHash) {
			return fail("retry_binding_mismatch", "重做缺少原完整计划")
		}
		plan := *origin.Plan
		plan.VerificationCandidateRef = nil
		var err error
		plan.PlanHash, err = contracts.HashObject("ProjectPlan", plan)
		if err != nil {
			return err
		}
		if err = s.canonicalTargetAllowed(*r, plan); err != nil {
			return err
		}
		r.Plan, err = s.compatibilityPlan(*r, plan, plan.DeliveryTarget)
		if err != nil {
			return err
		}
	}
	r.Control.RetryOrigin = &origin
	return nil
}

func (s *Service) createControlRetry(ctx context.Context, original Run, cmd Command) (Run, error) {
	if !terminal(original.State) || original.Control == nil || original.FrozenDefinition == nil || cmd.Action != "retry" || cmd.RunID != original.ID {
		return Run{}, fail("retry_binding_mismatch", "仅允许准确终态受控Run重做")
	}
	if err := verifyControlChain(original); err != nil {
		return Run{}, err
	}
	origin := controlRetryOrigin{RunID: original.ID, SourceVersion: original.SourceVersion, DefinitionSnapshotHash: original.FrozenDefinition.SnapshotHash, Command: cmd, Plan: original.Control.Audit.Plan}
	input, err := json.Marshal(cmd)
	if err != nil {
		return Run{}, err
	}
	// Preserve the actual normalized command rather than inventing user prose
	// or copying an old message under a new identity.
	opts := CreateOptions{ID: retryRunID(original.ID, cmd.ID), Title: original.Title + " · 重做", DecisionMode: original.DecisionMode, DeliveryMode: original.DeliveryMode,
		DefinitionID: original.FrozenDefinition.DefinitionID, DefinitionVersion: original.FrozenDefinition.DefinitionVersion, DefinitionHash: original.FrozenDefinition.DefinitionHash}
	if origin.Plan != nil {
		opts.Content = string(input)
	}
	return s.createRunWithControlRetry(ctx, opts, nil, &origin, original.ID)
}
