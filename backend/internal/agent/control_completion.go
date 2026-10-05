package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"agentstudio/internal/contracts"
)

type controlCompletionObservation struct {
	files       map[string]string
	packageData []byte
	readErr     error
	ciArtifacts map[string][]byte
	ciReadErr   error
}

func (s *Service) observeControlCompletion(r Run) controlCompletionObservation {
	facts := controlCompletionObservation{}
	facts.files, facts.readErr = s.controlToolFiles(r, r.Workspace)
	if facts.readErr != nil {
		return facts
	}
	for _, receipt := range r.Control.Audit.PackageReceipts {
		if receipt.Status == "succeeded" && receipt.SourceVersion == int64(r.SourceVersion) && receipt.PackageRef != nil && equalContractValue(receipt.ManifestRef, scheduleManifestRef(*r.Control.Audit.Manifest)) {
			facts.packageData, facts.readErr = s.readControlPackage(r, *scheduleRef(receipt.PackageRef))
			break
		}
	}
	if len(facts.packageData) == 0 && facts.readErr == nil {
		facts.readErr = fail("package_not_ready", "缺少当前来源的完整成功包回执")
	}
	facts.ciArtifacts = map[string][]byte{}
	if r.DeliveryMode == "gitlab" {
		for _, receipt := range r.Control.Audit.RemoteReceipts {
			if receipt.ActionType != "trigger_ci" || receipt.Status != "succeeded" || receipt.SourceVersion != int64(r.SourceVersion) {
				continue
			}
			for _, job := range receipt.Jobs {
				for _, artifact := range job.Artifacts {
					ref := contracts.ObjectReference(artifact.ArtifactRef)
					data, err := s.readControlCIArtifact(r, ref)
					if err != nil {
						facts.ciReadErr = errors.Join(facts.ciReadErr, err)
						continue
					}
					facts.ciArtifacts[ref.ObjectID] = data
				}
			}
		}
	}
	return facts
}

func controlReportRef(report contracts.VerificationReport) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "verification_report", ObjectID: report.ReportID, ObjectVersion: report.ReportVersion, SourceVersion: report.SourceVersion, Hash: report.ReportHash}
}

// buildControlVerificationReport consumes complete persisted contracts and
// caller-observed actual bytes. It does not accept legacy checks or LLM verdicts.
func buildControlVerificationReport(r Run, observed controlCompletionObservation, reportID string, version int64, at time.Time) (contracts.VerificationReport, error) {
	var report contracts.VerificationReport
	if r.Control == nil || r.FrozenDefinition == nil || r.Control.Audit.Plan == nil || r.Control.Audit.Proposal == nil || r.Control.Audit.Manifest == nil {
		return report, fail("completion_not_ready", "完整报告缺少当前方案、批准或manifest")
	}
	a := r.Control.Audit
	stamp := at.UTC().Format(time.RFC3339Nano)
	report = contracts.VerificationReport{ContractVersion: r.FrozenDefinition.ContractVersion, ReportID: reportID, ReportVersion: version, RunID: r.ID, SourceVersion: int64(r.SourceVersion), DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, ProposalSnapshotHash: a.Proposal.SnapshotHash, ManifestRef: scheduleManifestRef(*a.Manifest), Checks: []contracts.RequiredCheck{}, Outcome: "passed", CreatedAt: stamp}
	schedule, chainErr := NextControlAction(r, r.Control.Schedule)
	chainReady := chainErr == nil && schedule.ActionsExhausted && !schedule.NeedsReconciliation && !schedule.AwaitingEvidence && r.ActiveStep == "" && !r.PauseRequested && !r.CancelRequested
	policyReady := true
	for _, check := range requiredChecksFor(r) {
		if strings.HasPrefix(check.Name, "unsupported_") || strings.HasPrefix(check.Name, "duplicate_") || strings.HasPrefix(check.Name, "policy_missing:") {
			policyReady = false
		}
	}
	policy := r.FrozenDefinition.Definition.CompletionPolicy
	if policy.CompletionAuthority != "deterministic_validator" || policy.ExternalValidationAuthority || policy.AllowUnverifiedDone {
		policyReady = false
	}
	manifestRef := scheduleManifestRef(*a.Manifest)
	proposalRef := contracts.ObjectReference{ObjectType: "proposal", ObjectID: a.Proposal.ProposalID, ObjectVersion: a.Proposal.ProposalVersion, SourceVersion: a.Proposal.SourceVersion, Hash: a.Proposal.SnapshotHash}
	sourceReady := observed.readErr == nil && controlMatchApproved(r, observed.files) == nil && hashMatches("ArtifactManifest", *a.Manifest, a.Manifest.Hash)
	var packageReceipt *contracts.PackageReceipt
	for _, receipt := range a.PackageReceipts {
		if receipt.Status != "succeeded" || receipt.SourceVersion != int64(r.SourceVersion) || !equalContractValue(receipt.ManifestRef, manifestRef) {
			continue
		}
		for _, request := range a.Executions {
			if request.ExecutionID == receipt.ExecutionID && controlPackageReceiptBinding(r, request, receipt) == nil {
				if packageReceipt != nil {
					return report, fail("package_receipt_mismatch", "同一完成来源出现多个成功包")
				}
				copy := receipt
				packageReceipt = &copy
			}
		}
	}
	packageReady := packageReceipt != nil && sourceReady && packageReceipt.FileCount == int64(len(a.Manifest.Files)) && packageReceipt.PackageBytes == int64(len(observed.packageData)) && packageReceipt.PackageSHA256 != nil && *packageReceipt.PackageSHA256 == contracts.BytesHash(observed.packageData) && verifyControlPackageBytes(r, observed.files, observed.packageData) == nil
	toolReceipts := map[string]contracts.ToolExecutionReceipt{}
	for _, receipt := range a.ToolReceipts {
		if receipt.SourceVersion != int64(r.SourceVersion) || receipt.Status != "succeeded" || receipt.OutputManifestRef == nil || !equalContractValue(receipt.InputManifestRef, manifestRef) || !equalContractValue(receipt.OutputManifestRef, manifestRef) || !hashMatches("ToolExecutionReceipt", receipt, receipt.ReceiptHash) {
			continue
		}
		for _, op := range a.Operations {
			if op.OperationID == receipt.OperationID && op.ActionType == "verify" && op.Status == "verified" && op.RequestHash == receipt.RequestHash && scheduleHasRef(op.EvidenceRefs, contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}) {
				toolReceipts[receipt.CommandID] = receipt
			}
		}
	}
	remoteReceipts := map[string]contracts.RemoteExecutionReceipt{}
	for _, receipt := range a.RemoteReceipts {
		if receipt.Status != "succeeded" || receipt.SourceVersion != int64(r.SourceVersion) || receipt.ProposalSnapshotHash != a.Proposal.SnapshotHash || !chainEqual(receipt.ManifestRef, manifestRef) || ValidateControlRemoteReceipt(r, receipt) != nil {
			continue
		}
		verified := false
		for _, op := range a.Operations {
			if op.OperationID == receipt.OperationID && op.Status == "verified" && scheduleHasRef(op.EvidenceRefs, controlRemoteReceiptRef(receipt)) {
				verified = true
			}
		}
		if !verified {
			continue
		}
		if _, duplicate := remoteReceipts[receipt.ActionType]; duplicate {
			return report, remoteReceiptError("ambiguous completion receipt")
		}
		if receipt.ActionType == "trigger_ci" {
			ready := observed.ciReadErr == nil
			for _, job := range receipt.Jobs {
				for _, artifact := range job.Artifacts {
					data, ok := observed.ciArtifacts[artifact.ArtifactRef.ObjectID]
					format, err := controlCIArtifactFormat(artifact.Path, data)
					if !ok || err != nil || format != artifact.Format || int64(len(data)) != artifact.ByteCount || contracts.BytesHash(data) != artifact.SHA256 {
						ready = false
					}
				}
			}
			if !ready {
				continue
			}
		}
		remoteReceipts[receipt.ActionType] = receipt
	}
	for _, definition := range policy.RequiredChecks {
		if !chainContains(definition.DeliveryModes, r.DeliveryMode) {
			continue
		}
		check := contracts.RequiredCheck{CheckID: definition.CheckID, RunID: r.ID, SourceVersion: int64(r.SourceVersion), DefinitionSnapshotHash: r.FrozenDefinition.SnapshotHash, ProposalSnapshotHash: &a.Proposal.SnapshotHash, Status: "not_ready", Detail: "缺少同源完整受控证据", EvidenceRefs: []contracts.ObjectReference{}, StartedAt: &stamp, CompletedAt: &stamp}
		passed := false
		switch definition.CheckID {
		case "check.approved_sources", "check.artifact_manifest":
			passed = sourceReady
			check.EvidenceRefs = []contracts.ObjectReference{proposalRef, manifestRef}
		case "check.go_mod_tidy", "check.go_test", "check.go_build":
			command := map[string]string{"check.go_mod_tidy": "go.mod_tidy", "check.go_test": "go.test", "check.go_build": "go.build"}[definition.CheckID]
			if receipt, exists := toolReceipts[command]; exists {
				passed = true
				check.EvidenceRefs = []contracts.ObjectReference{{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}, manifestRef}
			}
		case "check.go_mod_file", "check.gitignore_file", "check.ci_config_file", "check.readme_file":
			name := map[string]string{"check.go_mod_file": "go.mod", "check.gitignore_file": ".gitignore", "check.ci_config_file": ".gitlab-ci.yml", "check.readme_file": "README.md"}[definition.CheckID]
			_, exists := observed.files[name]
			passed = sourceReady && exists
			check.EvidenceRefs = []contracts.ObjectReference{manifestRef}
		case "check.secret_boundary":
			passed = sourceReady && packageReady
			check.EvidenceRefs = []contracts.ObjectReference{manifestRef}
		case "check.zip_integrity":
			passed = packageReady
			check.EvidenceRefs = []contracts.ObjectReference{manifestRef}
		case "check.remote_repository", "check.remote_commit", "check.gitlab_ci":
			kind := map[string]string{"check.remote_repository": "create_repository", "check.remote_commit": "push_repository", "check.gitlab_ci": "trigger_ci"}[definition.CheckID]
			if receipt, exists := remoteReceipts[kind]; exists {
				passed = true
				check.EvidenceRefs = []contracts.ObjectReference{{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}, manifestRef}
				if kind == "trigger_ci" {
					for _, job := range receipt.Jobs {
						for _, artifact := range job.Artifacts {
							check.EvidenceRefs = append(check.EvidenceRefs, contracts.ObjectReference(artifact.ArtifactRef))
						}
					}
				}
			}
		default:
			check.Detail = "冻结策略要求的检查尚无受控执行器"
		}
		if packageReceipt != nil && (definition.Kind == "source" || definition.Kind == "file" || definition.Kind == "manifest" || definition.Kind == "secret_boundary" || definition.Kind == "package") {
			check.EvidenceRefs = append(check.EvidenceRefs, controlPackageReceiptRef(*packageReceipt))
		}
		if passed && chainReady && policyReady {
			check.Status, check.Detail = "passed", "完整同源证据与当前实际字节通过确定性核对"
		} else if !policyReady {
			check.Detail = "冻结完成策略削弱安全基线或包含无受控实现的检查"
		} else if !chainReady {
			check.Detail = "完整动作链尚未耗尽、存在未知写或治理证据不完整"
		} else if observed.readErr != nil && (definition.Kind == "source" || definition.Kind == "file" || definition.Kind == "manifest" || definition.Kind == "secret_boundary" || definition.Kind == "package") {
			check.Status, check.Detail = "failed", "实际批准目录或交付包读取/完整性核对失败"
		}
		if check.Status == "failed" {
			report.Outcome = "failed"
		} else if check.Status != "passed" && report.Outcome != "failed" {
			report.Outcome = "not_ready"
		}
		report.Checks = append(report.Checks, check)
	}
	var err error
	report.ReportHash, err = contracts.HashObject("VerificationReport", report)
	if err == nil {
		err = contracts.Validate("VerificationReport", report)
	}
	return report, err
}

// FinalizeControlCompletion keeps model advice and deterministic completion
// separate. The completion lock permits the semantic helper's command lock and
// serializes concurrent callers without blocking user pause/cancel commands.
func (s *Service) FinalizeControlCompletion(ctx context.Context, id string) (Run, contracts.VerificationReport, error) {
	var empty contracts.VerificationReport
	s.mu.Lock()
	key := "control-completion:" + id
	lock := s.commandLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.commandLocks[key] = lock
	}
	s.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	r, err := s.Store.Get(id)
	if err != nil {
		return r, empty, err
	}
	if r.Control == nil || !contracts.DeliveryContract(r.Control.Audit.ContractVersion) || r.Control.Audit.Manifest == nil || r.Control.Audit.Proposal == nil {
		return r, empty, fail("completion_not_ready", "完成需要完整1.3公开证据")
	}
	if r.State == "DONE" && r.Control.Audit.VerificationReport != nil && r.Control.Audit.VerificationReport.Outcome == "passed" {
		return r, *r.Control.Audit.VerificationReport, nil
	}
	if r.ActiveStep != "" || r.State != "DELIVER" || r.PauseRequested || r.CancelRequested {
		return r, empty, fail("completion_not_ready", "当前Run尚未到可完成边界")
	}
	if err = s.recheckControlApproval(r, time.Now().UTC()); err != nil {
		return r, empty, err
	}
	if err = ctx.Err(); err != nil {
		return r, empty, err
	}
	at := time.Now().UTC()
	report, err := buildControlVerificationReport(r, s.observeControlCompletion(r), newID("report_"), 1, at)
	if err != nil {
		return r, report, err
	}
	r, err = s.Store.Update(id, func(run *Run) error {
		if run.SourceVersion != r.SourceVersion || run.ActiveStep != "" || run.State != "DELIVER" || run.PauseRequested || run.CancelRequested || !equalContractValue(run.Control.Audit.Manifest, r.Control.Audit.Manifest) || run.Control.Audit.Proposal.SnapshotHash != report.ProposalSnapshotHash {
			return fail("completion_changed", "报告签发前来源或控制边界已改变")
		}
		run.Control.Audit.VerificationReport = &report
		run.Control.Audit.VerificationReports = append(run.Control.Audit.VerificationReports, report)
		if report.Outcome != "passed" {
			run.State, run.ResumeTo, run.Error = "BLOCKED", "DELIVER", "完整固定检查尚未通过，不能完成"
		}
		return nil
	})
	if err != nil || report.Outcome != "passed" {
		return r, report, errors.Join(err, fail("completion_not_ready", "完整固定检查未全部通过"))
	}
	ref := controlReportRef(report)
	hint := "完整固定检查已通过；仅建议控制平面重新核对证据后完成。"
	rules := contracts.DecisionAdvice{Kind: "completion", CompletionHint: &hint, Reason: "依据已归档的完整固定检查报告提出完成建议"}
	validation, err := s.prepareControlSemanticAdvice(ctx, id, "completion", rules, map[string]any{"verification_report": ref, "outcome": report.Outcome, "check_count": len(report.Checks)}, []contracts.ObjectReference{ref})
	if err != nil {
		latest, getErr := s.Store.Get(id)
		return latest, report, errors.Join(err, getErr)
	}
	r, err = s.Store.Update(id, func(run *Run) error {
		if run.SourceVersion != r.SourceVersion || run.ActiveStep != "" || run.State != "DELIVER" || run.PauseRequested || run.CancelRequested || run.Control.Audit.VerificationReport == nil || !equalContractValue(*run.Control.Audit.VerificationReport, report) {
			return fail("completion_changed", "建议返回后来源、报告或用户控制已改变")
		}
		if err := s.recheckControlApproval(*run, time.Now().UTC()); err != nil {
			return err
		}
		found := false
		for _, stored := range run.Control.Audit.Validations {
			if equalContractValue(stored, validation) && stored.DecisionKind == "completion" && stored.Verdict == "allowed" && stored.ExecutionRequest == nil {
				found = true
			}
		}
		if !found {
			return fail("completion_not_ready", "缺少真实持久化完成建议校验")
		}
		checked, err := buildControlVerificationReport(*run, s.observeControlCompletion(*run), report.ReportID, report.ReportVersion, at)
		if err != nil {
			return err
		}
		if checked.Outcome != "passed" || !equalContractValue(checked, report) {
			return fail("completion_changed", "建议之后实际字节或完整证据已变化")
		}
		run.State, run.Error = "DONE", ""
		return nil
	})
	return r, report, err
}
