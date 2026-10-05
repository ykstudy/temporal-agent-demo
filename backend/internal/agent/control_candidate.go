package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

func controlCandidateRef(c contracts.VerificationCandidate) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "verification_candidate", ObjectID: c.CandidateID, ObjectVersion: 1, SourceVersion: c.SourceVersion, Hash: c.CandidateHash}
}

func controlCandidateChanged(m contracts.ArtifactManifest, files map[string]string) []string {
	changed, before := map[string]bool{}, map[string]contracts.ArtifactEntry{}
	for _, entry := range m.Files {
		before[entry.Path] = entry
		value, ok := files[entry.Path]
		if !ok || int64(len(value)) != entry.Size || contracts.BytesHash([]byte(value)) != entry.SHA256 {
			changed[entry.Path] = true
		}
	}
	for name := range files {
		if _, ok := before[name]; !ok {
			changed[name] = true
		}
	}
	paths := make([]string, 0, len(changed))
	for name := range changed {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths
}

// buildControlCandidate freezes an actual sandbox output. The caller commits
// this body with the original parent settlement, never as a manifest or grant.
func (s *Service) buildControlCandidate(r Run, x contracts.ExecutionRequest, parent contracts.BudgetReservation, result ControlToolsResult, at time.Time) (contracts.VerificationCandidate, error) {
	var candidate contracts.VerificationCandidate
	if !contracts.DeliveryContract(x.ContractVersion) || !result.ReapprovalRequired || result.Candidate == nil || r.Control.Audit.Manifest == nil || len(result.Receipts) == 0 {
		return candidate, fail("candidate_not_available", "当前契约或真实工具结果缺少完整修订候选")
	}
	paths, err := s.controlToolPaths(r, x, parent.AttemptID)
	if err != nil {
		return candidate, err
	}
	if result.Candidate.Directory != paths.candidate {
		return candidate, fail("candidate_binding_mismatch", "候选目录不是原尝试的隔离工作区")
	}
	files, err := s.controlToolFiles(r, paths.candidate)
	if err != nil {
		return candidate, err
	}
	if !equalContractValue(files, result.Candidate.Files) || !equalContractValue(controlCandidateChanged(*r.Control.Audit.Manifest, files), result.Candidate.ChangedPaths) {
		return candidate, fail("candidate_binding_mismatch", "候选实际文件或差异在登记前变化")
	}
	identity, _ := contracts.HashValue([]string{x.RequestHash, parent.AttemptID})
	candidate = contracts.VerificationCandidate{ContractVersion: x.ContractVersion, CandidateID: "candidate_" + identity[:32], RunID: r.ID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: parent.AttemptID, RequestHash: x.RequestHash, ApprovalID: x.Approval.ApprovalID, ApprovalHash: r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, PlanHash: x.PlanHash, Workspace: x.Workspace, InputManifestRef: contracts.VerificationCandidateInputManifestRef(controlManifestRef(*r.Control.Audit.Manifest)), ToolReceiptRefs: []contracts.VerificationCandidateToolReceiptRefsItem{}, Files: []contracts.VerificationCandidateFile{}, ChangedPaths: append([]string{}, result.Candidate.ChangedPaths...), CreatedAt: at.Format(time.RFC3339Nano)}
	for _, receipt := range result.Receipts {
		candidate.ToolReceiptRefs = append(candidate.ToolReceiptRefs, contracts.VerificationCandidateToolReceiptRefsItem{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash})
	}
	for name, content := range files {
		candidate.Files = append(candidate.Files, contracts.VerificationCandidateFile{Path: name, Content: content, Size: int64(len(content)), SHA256: contracts.BytesHash([]byte(content))})
		candidate.TotalBytes += int64(len(content))
	}
	sort.Slice(candidate.Files, func(i, j int) bool { return candidate.Files[i].Path < candidate.Files[j].Path })
	candidate.CandidateHash, err = contracts.HashObject("VerificationCandidate", candidate)
	return candidate, err
}

// controlCandidateEvidence verifies the entire historical source closure. It
// can run after source advancement and never reconstructs facts from messages.
func controlCandidateEvidence(r Run, c contracts.VerificationCandidate) (contracts.ExecutionRequest, contracts.Proposal, map[string]string, error) {
	var x contracts.ExecutionRequest
	var proposal contracts.Proposal
	bad := func() (contracts.ExecutionRequest, contracts.Proposal, map[string]string, error) {
		return x, proposal, nil, fail("candidate_binding_mismatch", "候选缺少准确原执行、回执、文件或预算来源")
	}
	if r.Control == nil || r.FrozenDefinition == nil || !contracts.DeliveryContract(c.ContractVersion) || c.ContractVersion != r.FrozenDefinition.ContractVersion || !hashMatches("VerificationCandidate", c, c.CandidateHash) || c.RunID != r.ID || c.SourceVersion > int64(r.SourceVersion) || c.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash {
		return bad()
	}
	for _, request := range r.Control.Audit.Executions {
		if request.ExecutionID == c.ExecutionID {
			x = request
		}
	}
	kind, raw, err := scheduleAction(x.Action)
	identity, _ := contracts.HashValue([]string{x.RequestHash, c.AttemptID})
	if err != nil || kind != "verify" || !hashMatches("ExecutionRequest", x, x.RequestHash) || x.RunID != c.RunID || x.ContractVersion != c.ContractVersion || x.DefinitionSnapshotHash != c.DefinitionSnapshotHash || c.CandidateID != "candidate_"+identity[:32] || c.SourceVersion != x.SourceVersion || c.OperationID != x.OperationID || c.RequestHash != x.RequestHash || c.ApprovalID != x.Approval.ApprovalID || c.ProposalSnapshotHash != x.Approval.ProposalSnapshotHash || c.PlanHash != x.PlanHash || !equalContractValue(c.Workspace, x.Workspace) {
		return bad()
	}
	var parameters contracts.VerifyParameters
	if json.Unmarshal(raw, &parameters) != nil || !equalContractValue(parameters.ManifestRef, c.InputManifestRef) {
		return bad()
	}
	for _, p := range r.Control.Audit.ProposalHistory {
		if p.ProposalID == x.Approval.ProposalID && p.ProposalVersion == x.Approval.ProposalVersion {
			proposal = p
		}
	}
	approved := false
	for _, a := range r.Control.Audit.ApprovalHistory {
		if a.ApprovalID == c.ApprovalID && a.ApprovalHash == c.ApprovalHash && a.ProposalSnapshotHash == c.ProposalSnapshotHash && a.SourceVersion == c.SourceVersion && a.RunID == c.RunID && a.PlanHash == c.PlanHash && a.DefinitionSnapshotHash == c.DefinitionSnapshotHash && hashMatches("Approval", a, a.ApprovalHash) {
			approved = true
		}
	}
	var manifest contracts.ArtifactManifest
	for _, m := range r.Control.Audit.ManifestHistory {
		if equalContractValue(controlManifestRef(m), c.InputManifestRef) {
			manifest = m
		}
	}
	if !approved || !hashMatches("Proposal", proposal, proposal.SnapshotHash) || proposal.RunID != c.RunID || proposal.SourceVersion != c.SourceVersion || proposal.PlanHash != c.PlanHash || proposal.DefinitionSnapshotHash != c.DefinitionSnapshotHash || !hashMatches("ArtifactManifest", manifest, manifest.Hash) || proposal.SnapshotHash != c.ProposalSnapshotHash || manifest.RunID != c.RunID || manifest.PlanHash != c.PlanHash || manifest.DefinitionSnapshotHash != c.DefinitionSnapshotHash || manifest.SourceVersion != c.SourceVersion || manifest.ProposalID != proposal.ProposalID || manifest.ProposalVersion != proposal.ProposalVersion || manifest.ProposalSnapshotHash != c.ProposalSnapshotHash || !chainBefore(manifest.CreatedAt, x.CreatedAt, false) {
		return bad()
	}
	approvedFiles := map[string]string{}
	for _, change := range proposal.Changes {
		if change.After != nil {
			approvedFiles[change.Path] = *change.After
		}
	}
	historical := r
	control := *r.Control
	control.Audit = r.Control.Audit
	control.Audit.Manifest, control.Audit.Proposal = &manifest, &proposal
	historical.Control = &control
	if controlMatchApproved(historical, approvedFiles) != nil {
		return bad()
	}
	parent, parentSettlement, known := controlCandidateSettlement(r, x, c.AttemptID, "action", "verify")
	if !known || parentSettlement.Outcome != "failed" || !chainBefore(c.CreatedAt, parentSettlement.SettledAt, false) {
		return bad()
	}
	commands, err := governance.MergePostCommands(*r.FrozenDefinition, proposal.Plan)
	if err != nil || len(c.ToolReceiptRefs) == 0 || len(c.ToolReceiptRefs) > len(commands) {
		return bad()
	}
	previousCompletion := parent.ReservedAt
	for i, ref := range c.ToolReceiptRefs {
		found := false
		identity, _ := contracts.HashValue([]string{x.RequestHash, c.AttemptID, commands[i].CommandID})
		reservation, settlement, known := controlCandidateSettlement(r, x, "tool_"+identity[:40], "tool", "tool:"+commands[i].CommandID)
		if !known || !chainBefore(previousCompletion, reservation.ReservedAt, false) || !chainBefore(settlement.SettledAt, c.CreatedAt, false) {
			return bad()
		}
		for _, receipt := range r.Control.Audit.ToolReceipts {
			if receipt.ReceiptID != ref.ObjectID {
				continue
			}
			actual := contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}
			toolBound := false
			for _, tool := range r.FrozenDefinition.ToolBindings {
				if tool.ToolID == commands[i].Executable && equalContractValue(tool, receipt.Tool) {
					toolBound = true
				}
			}
			outcome := receipt.Status
			if outcome == "timed_out" {
				outcome = "failed"
			}
			if found || !toolBound || !equalContractValue(ref, actual) || !scheduleHasRef(r.Control.ObjectRefs, actual) || !hashMatches("ToolExecutionReceipt", receipt, receipt.ReceiptHash) || receipt.ContractVersion != c.ContractVersion || receipt.RunID != c.RunID || receipt.SourceVersion != c.SourceVersion || receipt.DefinitionSnapshotHash != c.DefinitionSnapshotHash || receipt.OperationID != c.OperationID || receipt.ApprovalID != c.ApprovalID || receipt.ProposalSnapshotHash != c.ProposalSnapshotHash || receipt.PlanHash != c.PlanHash || !equalContractValue(receipt.Workspace, c.Workspace) || receipt.ExecutionID != x.ExecutionID || receipt.RequestHash != c.RequestHash || receipt.ApprovalHash != c.ApprovalHash || receipt.AttemptID != reservation.AttemptID || receipt.CommandID != commands[i].CommandID || !equalContractValue(receipt.Arguments, commands[i].Arguments) || receipt.CwdScope != commands[i].CwdScope || receipt.EnvironmentPolicy != commands[i].EnvironmentPolicy || receipt.OutputSHA256 != contracts.BytesHash([]byte(receipt.Output)) || !equalContractValue(receipt.InputManifestRef, c.InputManifestRef) || !chainBefore(reservation.ReservedAt, receipt.StartedAt, false) || !chainBefore(receipt.StartedAt, receipt.CompletedAt, false) || !chainBefore(receipt.CompletedAt, settlement.SettledAt, false) || !chainContains([]string{"succeeded", "failed", "timed_out"}, receipt.Status) || settlement.Outcome != outcome {
				return bad()
			}
			if i == len(c.ToolReceiptRefs)-1 && receipt.OutputManifestRef != nil || i != len(c.ToolReceiptRefs)-1 && (receipt.Status != "succeeded" || !equalContractValue(receipt.OutputManifestRef, c.InputManifestRef)) {
				return bad()
			}
			found = true
			previousCompletion = receipt.CompletedAt
		}
		if !found {
			return bad()
		}
	}
	allowed := map[string]bool{}
	for _, change := range proposal.Changes {
		allowed[change.Path] = true
	}
	for _, selected := range proposal.Plan.SelectedSkills {
		var selection struct {
			SkillID string `json:"skill_id"`
		}
		if json.Unmarshal(selected, &selection) != nil {
			return bad()
		}
		for _, skill := range r.FrozenDefinition.Skills {
			if skill.SkillID == selection.SkillID {
				for _, name := range skill.WritePaths {
					allowed[name] = true
				}
			}
		}
	}
	files, previous, total := map[string]string{}, "", int64(0)
	for _, file := range c.Files {
		if file.Path <= previous || !allowed[file.Path] || int64(len(file.Content)) != file.Size || contracts.BytesHash([]byte(file.Content)) != file.SHA256 {
			return bad()
		}
		files[file.Path], previous = file.Content, file.Path
		total += file.Size
	}
	if total != c.TotalBytes || total > r.FrozenDefinition.Definition.BudgetPolicy.MaxArtifactBytes || int64(len(files)) > r.FrozenDefinition.Definition.BudgetPolicy.MaxFiles || !equalContractValue(controlCandidateChanged(manifest, files), c.ChangedPaths) {
		return bad()
	}
	return x, proposal, files, nil
}

// Every actual attempt has exactly one reservation and one known settlement;
// a matching receipt hash cannot stand in for this persisted accounting.
func controlCandidateSettlement(r Run, x contracts.ExecutionRequest, attempt, kind, scope string) (contracts.BudgetReservation, contracts.BudgetSettlement, bool) {
	var reservation contracts.BudgetReservation
	var settlement contracts.BudgetSettlement
	for _, value := range r.Control.Audit.Budget.Reservations {
		if value.AttemptID == attempt {
			if reservation.ReservationID != "" {
				return reservation, settlement, false
			}
			reservation = value
		}
	}
	if !hashMatches("BudgetReservation", reservation, reservation.ReservationHash) || reservation.RunID != x.RunID || reservation.SourceVersion != x.SourceVersion || reservation.ContractVersion != x.ContractVersion || reservation.DefinitionSnapshotHash != x.DefinitionSnapshotHash || reservation.PolicyHash != r.Control.Audit.Budget.PolicyHash || reservation.CommandID != x.CommandID || reservation.Kind != kind || reservation.ScopeID != scope || !equalContractValue(reservation.RequestHash, x.RequestHash) || !equalContractValue(reservation.ExecutionID, x.ExecutionID) || !equalContractValue(reservation.OperationID, x.OperationID) || !chainBefore(x.CreatedAt, reservation.ReservedAt, false) {
		return reservation, settlement, false
	}
	for _, value := range r.Control.Audit.Budget.Settlements {
		if value.ReservationID == reservation.ReservationID {
			if settlement.SettlementID != "" {
				return reservation, settlement, false
			}
			settlement = value
		}
	}
	known := hashMatches("BudgetSettlement", settlement, settlement.SettlementHash) && settlement.RunID == x.RunID && settlement.ContractVersion == x.ContractVersion && settlement.SourceVersion == x.SourceVersion && settlement.AttemptID == attempt && settlement.ReservationHash == reservation.ReservationHash && settlement.ConsumedUnits == reservation.Units && settlement.MeasurementSource == "actual" && settlement.ElapsedMS != nil && chainContains([]string{"succeeded", "failed"}, settlement.Outcome) && chainBefore(reservation.ReservedAt, settlement.SettledAt, false)
	return reservation, settlement, known
}

func (s *Service) ProposeControlCandidate(ctx context.Context, id string) (Run, error) {
	s.mu.Lock()
	lock := s.commandLocks[id]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[id] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	return s.proposeControlCandidate(ctx, id, nil)
}

// proposeControlCandidate requires the command lock; it is shared by automatic
// VERIFY repair and an explicit version-bound revise command after a failure.
func (s *Service) proposeControlCandidate(ctx context.Context, id string, finish func(*Run) error) (Run, error) {
	r, err := s.Store.Get(id)
	if err != nil {
		return r, err
	}
	if r.Control == nil || r.FrozenDefinition == nil || !contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) || !chainContains([]string{"VERIFY", "BLOCKED"}, r.State) || r.State == "BLOCKED" && r.ResumeTo != "VERIFY" || r.ActiveStep != "" || r.CancelRequested || r.PauseRequested || r.Control.Audit.Approval == nil {
		return r, fail("revision_not_allowed", "当前Run不处于已知验证修订边界")
	}
	if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
		return r, err
	}
	if err = s.controlCandidateApprovedFiles(r); err != nil {
		return r, err
	}
	for _, op := range r.Control.Audit.Operations {
		if op.Status == "sent" || op.Status == "unknown" {
			return r, fail("reconciliation_required", "存在未知操作时不能推进新修订")
		}
	}
	var candidate *contracts.VerificationCandidate
	for i := len(r.Control.Audit.VerificationCandidates) - 1; i >= 0; i-- {
		c := &r.Control.Audit.VerificationCandidates[i]
		if c.SourceVersion == int64(r.SourceVersion) && c.ApprovalID == r.Control.Audit.Approval.ApprovalID {
			candidate = c
			break
		}
	}
	if candidate == nil {
		return r, fail("candidate_not_available", "没有当前批准来源的真实候选差异")
	}
	_, _, files, err := controlCandidateEvidence(r, *candidate)
	if err != nil {
		return r, err
	}
	ref := controlCandidateRef(*candidate)
	if !scheduleHasRef(r.Control.ObjectRefs, ref) {
		return r, fail("candidate_not_available", "候选原体未登记")
	}
	plan := *r.Control.Audit.Plan
	candidateRef := contracts.ProjectPlanVerificationCandidateRef(ref)
	plan.VerificationCandidateRef = &candidateRef
	plan.PlanHash, err = contracts.HashObject("ProjectPlan", plan)
	if err != nil {
		return r, err
	}
	r, err = s.Store.Update(id, func(run *Run) error {
		if run.SourceVersion != r.SourceVersion || run.ActiveStep != "" || run.PauseRequested || run.CancelRequested || !chainContains([]string{"VERIFY", "BLOCKED"}, run.State) {
			return fail("revision_not_allowed", "修订前Run已改变")
		}
		run.State = "VERIFY"
		return nil
	})
	if err != nil {
		return r, err
	}
	failure := func(cause error) (Run, error) {
		updated, saveErr := s.Store.Update(id, func(run *Run) error {
			if run.SourceVersion == r.SourceVersion && run.State == "VERIFY" && run.ActiveStep == "" {
				run.State, run.ResumeTo = "BLOCKED", "VERIFY"
				run.Error = "修订建议未通过；保留原批准和候选，等待明确重试"
			}
			return nil
		})
		return updated, errors.Join(cause, saveErr)
	}
	rules := contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "实际工具产生候选差异；使用新来源和新批准重新生成工作区"}
	validation, err := s.controlSemanticAdvice(ctx, id, "plan", rules, map[string]any{"draft_plan": plan, "verification_candidate_ref": ref, "changed_paths": candidate.ChangedPaths, "instruction": "Propose exactly this candidate-bound plan; do not change its scope or target. It still requires a new explicit approval."}, []contracts.ObjectReference{ref})
	if err != nil {
		return failure(err)
	}
	if validation.NormalizedAdvice == nil || validation.NormalizedAdvice.Plan == nil {
		return failure(fail("advice_rejected", "未取得完整修订计划建议"))
	}
	proposed := *validation.NormalizedAdvice.Plan
	proposed.PlanHash, err = contracts.HashObject("ProjectPlan", proposed)
	if err != nil || !equalContractValue(proposed, plan) {
		return failure(fail("advice_rejected", "模型修订改变了已核对的候选、参数或交付范围"))
	}
	updated, err := s.Store.Update(id, func(run *Run) error {
		if run.State != "VERIFY" || run.ActiveStep != "" || run.PauseRequested || run.CancelRequested || run.SourceVersion != r.SourceVersion {
			return fail("revision_not_allowed", "建议期间来源或运行状态改变")
		}
		if e := s.recheckControlApproval(*run, time.Now().UTC()); e != nil {
			return e
		}
		if e := s.controlCandidateApprovedFiles(*run); e != nil {
			return e
		}
		if _, _, _, e := controlCandidateEvidence(*run, *candidate); e != nil {
			return e
		}
		sourceChanged(run)
		if e := s.makeCanonicalProposal(run, plan, files, time.Now().UTC()); e != nil {
			return e
		}
		run.Control.Audit.Manifest = nil
		run.Control.Audit.VerificationReport = nil
		run.Control.Schedule.Workspace = contracts.WorkspaceReference{}
		run.Workspace = ""
		run.Artifacts = []Artifact{}
		run.RequiredChecks = requiredChecksFor(*run)
		run.State, run.ResumeTo, run.Error = "PLAN", "", ""
		history(run, "verification_revision", "真实候选差异已形成新来源提案；旧目录和批准事实保留，等待重新确认")
		addMessage(run, "assistant", "验证产生了文件差异。请审查新提案；确认后将在新的工作区目录生成，旧文件保持可追溯。", "proposal", run.Proposal.ID)
		if finish != nil {
			return finish(run)
		}
		return nil
	})
	if err != nil {
		return failure(err)
	}
	return updated, nil
}

func (s *Service) controlCandidateApprovedFiles(r Run) error {
	files, err := s.controlToolFiles(r, r.Workspace)
	if err != nil {
		return err
	}
	return controlMatchApproved(r, files)
}
