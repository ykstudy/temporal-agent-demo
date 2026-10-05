package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

func remoteReceiptError(detail string) error {
	return fail("remote_receipt_mismatch", "远程回执不完整或与原执行不一致: "+detail)
}

// ValidateControlRemoteReceipt checks the complete public semantic chain. It
// never uses a private successful scheduler result as proof. Artifact bytes are
// a separate boundary: the completion reader must fetch the exact raw refs and
// independently validate coverage/executable content before granting DONE.
func ValidateControlRemoteReceipt(r Run, receipt contracts.RemoteExecutionReceipt) error {
	return validateControlRemoteReceipt(r, receipt, map[string]bool{})
}

func validateControlRemoteReceipt(r Run, q contracts.RemoteExecutionReceipt, visiting map[string]bool) error {
	if r.Control == nil || r.FrozenDefinition == nil || !contracts.DeliveryContract(q.ContractVersion) || r.FrozenDefinition.ContractVersion != q.ContractVersion || q.RunID != r.ID || q.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash || contracts.Validate("RemoteExecutionReceipt", q) != nil || !hashMatches("RemoteExecutionReceipt", q, q.ReceiptHash) {
		return remoteReceiptError("schema, frozen source or receipt hash")
	}
	if visiting[q.ReceiptID] {
		return remoteReceiptError("cyclic predecessor receipt")
	}
	visiting[q.ReceiptID] = true
	defer delete(visiting, q.ReceiptID)
	a := &r.Control.Audit
	if err := governance.ValidateBudgetLedger(r.FrozenDefinition.Definition.BudgetPolicy, a.Budget); err != nil {
		return remoteReceiptError("budget ledger")
	}
	var x *contracts.ExecutionRequest
	for _, candidate := range a.Executions {
		if candidate.ExecutionID == q.ExecutionID {
			if x != nil {
				return remoteReceiptError("duplicate execution identity")
			}
			copy := candidate
			x = &copy
		}
	}
	if x == nil || contracts.Validate("ExecutionRequest", *x) != nil || !hashMatches("ExecutionRequest", *x, x.RequestHash) || x.RunID != q.RunID || x.SourceVersion != q.SourceVersion || x.ContractVersion != q.ContractVersion || x.DefinitionSnapshotHash != q.DefinitionSnapshotHash || x.OperationID != q.OperationID || x.RequestHash != q.RequestHash || x.PlanHash != q.PlanHash || x.Approval.ApprovalID != q.ApprovalID || !chainEqual(x.Workspace, q.Workspace) || !chainBefore(x.CreatedAt, q.StartedAt, false) || !chainBefore(q.StartedAt, q.CompletedAt, false) {
		return remoteReceiptError("original execution and time")
	}
	kind, raw, err := scheduleAction(x.Action)
	var parameters struct {
		Target contracts.GitLabTarget `json:"target"`
	}
	if err != nil || kind != q.ActionType || json.Unmarshal(raw, &parameters) != nil || !chainEqual(parameters.Target, q.Target) {
		return remoteReceiptError("action and target")
	}
	targetHash, err := governance.RequestTargetHash(*x)
	operationFound := false
	for _, op := range a.Operations {
		if op.OperationID == q.OperationID {
			if operationFound || op.RunID != q.RunID || op.SourceVersion != q.SourceVersion || op.ActionType != kind || op.RequestHash != q.RequestHash || op.TargetHash != targetHash || err != nil {
				return remoteReceiptError("operation identity")
			}
			operationFound = true
		}
	}
	if !operationFound {
		return remoteReceiptError("missing original operation")
	}
	var approval *contracts.Approval
	for _, value := range a.ApprovalHistory {
		if value.ApprovalID == q.ApprovalID {
			if approval != nil {
				return remoteReceiptError("duplicate approval")
			}
			copy := value
			approval = &copy
		}
	}
	if approval == nil || contracts.Validate("Approval", *approval) != nil || !hashMatches("Approval", *approval, approval.ApprovalHash) || approval.ApprovalHash != q.ApprovalHash || x.Approval.ProposalID != approval.ProposalID || x.Approval.ProposalVersion != approval.ProposalVersion || x.Approval.SourceVersion != approval.SourceVersion || x.Approval.ProposalSnapshotHash != approval.ProposalSnapshotHash || x.Approval.DefinitionSnapshotHash != approval.DefinitionSnapshotHash || x.Approval.PermissionHash != approval.PermissionHash || x.Approval.IssuedAt != approval.IssuedAt || x.Approval.ExpiresAt != approval.ExpiresAt || approval.ProposalSnapshotHash != q.ProposalSnapshotHash || approval.RunID != q.RunID || approval.SourceVersion != q.SourceVersion || approval.DefinitionSnapshotHash != q.DefinitionSnapshotHash || approval.PlanHash != q.PlanHash || !chainEqual(approval.DeliveryTarget, q.Target) || !chainContains(approval.AllowedActions, kind) || !chainBefore(approval.AcceptedAt, q.StartedAt, false) {
		return remoteReceiptError("historical approval")
	}
	var proposal *contracts.Proposal
	for _, value := range a.ProposalHistory {
		if value.ProposalID == approval.ProposalID && value.ProposalVersion == approval.ProposalVersion {
			if proposal != nil {
				return remoteReceiptError("duplicate proposal")
			}
			copy := value
			proposal = &copy
		}
	}
	if proposal == nil || contracts.Validate("Proposal", *proposal) != nil || !hashMatches("Proposal", *proposal, proposal.SnapshotHash) || proposal.SnapshotHash != q.ProposalSnapshotHash || proposal.RunID != q.RunID || proposal.SourceVersion != q.SourceVersion || proposal.PlanHash != q.PlanHash || proposal.DefinitionSnapshotHash != q.DefinitionSnapshotHash || proposal.PermissionHash != approval.PermissionHash {
		return remoteReceiptError("historical proposal")
	}
	var manifest *contracts.ArtifactManifest
	for _, value := range a.ManifestHistory {
		if chainEqual(q.ManifestRef, scheduleManifestRef(value)) {
			if manifest != nil {
				return remoteReceiptError("duplicate manifest")
			}
			copy := value
			manifest = &copy
		}
	}
	if manifest == nil || contracts.Validate("ArtifactManifest", *manifest) != nil || !hashMatches("ArtifactManifest", *manifest, manifest.Hash) || manifest.RunID != q.RunID || manifest.SourceVersion != q.SourceVersion || manifest.DefinitionSnapshotHash != q.DefinitionSnapshotHash || manifest.ProposalSnapshotHash != q.ProposalSnapshotHash || manifest.PlanHash != q.PlanHash || manifest.ProposalID != proposal.ProposalID || manifest.ProposalVersion != proposal.ProposalVersion || !chainBefore(manifest.CreatedAt, q.StartedAt, false) {
		return remoteReceiptError("original complete manifest")
	}
	var action *contracts.BudgetReservation
	for _, value := range a.Budget.Reservations {
		if value.AttemptID == q.AttemptID {
			if action != nil {
				return remoteReceiptError("duplicate action attempt")
			}
			copy := value
			action = &copy
		}
	}
	if action == nil || action.Kind != "action" || action.ScopeID != kind || !remoteReservationBinds(*action, *x) || action.ReservedAt != q.StartedAt {
		return remoteReceiptError("original action reservation")
	}
	settlement := controlGitLabSettled(r, *action)
	if settlement == nil || settlement.ConsumedUnits != 1 || !chainBefore(action.ReservedAt, settlement.SettledAt, false) || !chainBefore(settlement.SettledAt, q.CompletedAt, false) || q.Status == "succeeded" && (settlement.Outcome != "succeeded" && settlement.Outcome != "unknown") {
		return remoteReceiptError("original action charge")
	}
	if q.Status == "succeeded" && !q.WriteSent {
		return remoteReceiptError("successful write without original send")
	}
	seenReads := map[string]bool{}
	for _, read := range q.Observations {
		if seenReads[read.ReservationRef.ObjectID] {
			return remoteReceiptError("reused read reservation")
		}
		seenReads[read.ReservationRef.ObjectID] = true
		var reservation *contracts.BudgetReservation
		for _, value := range a.Budget.Reservations {
			ref := contracts.ObjectReference{ObjectType: "budget_reservation", ObjectID: value.ReservationID, ObjectVersion: 1, SourceVersion: value.SourceVersion, Hash: value.ReservationHash}
			if chainEqual(ref, read.ReservationRef) {
				copy := value
				reservation = &copy
			}
		}
		if reservation == nil || !remoteReservationBinds(*reservation, *x) || (reservation.Kind != "reconciliation_read" && !(kind == "trigger_ci" && reservation.Kind == "ci_poll")) || reservation.ScopeID != reservation.Kind+":"+q.OperationID || reservation.DeadlineAt == nil || !chainBefore(x.CreatedAt, reservation.ReservedAt, false) || !chainBefore(reservation.ReservedAt, read.StartedAt, false) || !chainBefore(read.StartedAt, read.CompletedAt, false) || !chainBefore(read.CompletedAt, q.CompletedAt, false) {
			return remoteReceiptError("read reservation and timeline")
		}
		settled := controlGitLabSettled(r, *reservation)
		if settled == nil || settled.ConsumedUnits != 1 || !chainBefore(read.CompletedAt, settled.SettledAt, false) || !chainBefore(settled.SettledAt, q.CompletedAt, false) {
			return remoteReceiptError("read settlement")
		}
		if read.Kind != "unavailable" {
			hash, e := contracts.HashValue(read.Payload)
			if e != nil || read.FactsHash == nil || hash != *read.FactsHash || settled.Outcome != "succeeded" || !chainBefore(read.CompletedAt, *reservation.DeadlineAt, false) {
				return remoteReceiptError("read facts and deadline")
			}
		}
	}
	// A receipt cannot hide a completed GET by dropping its observation while
	// retaining its budget charge. Later reconciliation reads belong to later
	// immutable receipts; a genuinely interrupted read has unknown settlement.
	for _, reservation := range a.Budget.Reservations {
		if !remoteReservationBinds(reservation, *x) || (reservation.Kind != "reconciliation_read" && reservation.Kind != "ci_poll") {
			continue
		}
		settled := controlGitLabSettled(r, reservation)
		if settled != nil && settled.Outcome != "unknown" && chainBefore(settled.SettledAt, q.CompletedAt, false) && !seenReads[reservation.ReservationID] {
			return remoteReceiptError("completed read omitted from receipt")
		}
	}
	// Even an unknown/failed receipt must not claim nullable facts that were
	// never observed. Wrong-target facts may be retained as failure evidence;
	// only the successful branch below requires them to match approval.
	for _, fact := range []struct {
		kind  string
		value any
		set   bool
	}{{"project", q.Project, q.Project != nil}, {"commit", q.Commit, q.Commit != nil}, {"pipeline", q.Pipeline, q.Pipeline != nil}, {"pipeline_variables", q.PipelineVariables, q.PipelineVariables != nil}} {
		if !fact.set {
			continue
		}
		found := false
		for _, read := range q.Observations {
			if read.Kind == fact.kind && chainEqual(read.Payload, fact.value) {
				found = true
			}
		}
		if !found {
			return remoteReceiptError("unobserved nullable remote fact")
		}
	}
	if len(q.Jobs) > 0 {
		if err := remoteCIJobFacts(q); err != nil {
			return err
		}
	}
	if q.Status != "succeeded" {
		return nil
	}
	if q.Project == nil || !remoteObserved(q, "project", q.Project, remoteProjectReadPath(q.Project.ProjectID), controlGitLabProjectPath(q.Target)) || !remoteProjectTarget(*q.Project, q.Target) {
		return remoteReceiptError("actual project observation")
	}
	if kind == "create_repository" {
		if q.Project.OwnershipMarker != controlGitLabMarker(*x) || !q.Project.EmptyRepository {
			return remoteReceiptError("created empty project ownership")
		}
		return nil
	}
	predecessorKind := "create_repository"
	if kind == "trigger_ci" {
		predecessorKind = "push_repository"
	}
	var predecessor *contracts.RemoteExecutionReceipt
	for _, value := range a.RemoteReceipts {
		if value.ActionType == predecessorKind && value.Status == "succeeded" && value.SourceVersion == q.SourceVersion && value.PlanHash == q.PlanHash && value.ApprovalHash == q.ApprovalHash && chainEqual(value.Target, q.Target) && chainEqual(value.ManifestRef, q.ManifestRef) {
			if predecessor != nil {
				return remoteReceiptError("ambiguous predecessor")
			}
			copy := value
			predecessor = &copy
		}
	}
	if predecessor == nil || validateControlRemoteReceipt(r, *predecessor, visiting) != nil || !chainBefore(predecessor.CompletedAt, x.CreatedAt, false) || predecessor.Project == nil || predecessor.Project.ProjectID != q.Project.ProjectID || predecessor.Project.OwnershipMarker != q.Project.OwnershipMarker || q.Project.EmptyRepository || q.Project.DefaultBranch == nil || *q.Project.DefaultBranch != q.Target.Ref {
		return remoteReceiptError("complete predecessor and project ownership")
	}
	if q.Commit == nil || q.Commit.ProjectID != q.Project.ProjectID || !remoteObserved(q, "commit", q.Commit, remoteProjectReadPath(q.Project.ProjectID)+"/repository/commits/"+q.Commit.CommitSha) || !remoteObserved(q, "branch", contracts.RemoteBranchObservation{ProjectID: q.Project.ProjectID, Ref: q.Target.Ref, CommitSha: q.Commit.CommitSha}, remoteProjectReadPath(q.Project.ProjectID)+"/repository/branches/"+url.PathEscape(q.Target.Ref)) {
		return remoteReceiptError("exact remote commit and branch")
	}
	if err := remoteManifestFiles(*manifest, *proposal, q); err != nil {
		return err
	}
	if kind == "push_repository" {
		var p contracts.PushRepositoryParameters
		if contracts.DecodeVersion("PushRepositoryParameters", x.ContractVersion, raw, &p) != nil || p.ProjectID != q.Project.ProjectID || p.ExpectedBranchSha != nil || !chainEqual(p.ManifestRef, q.ManifestRef) || len(q.Commit.ParentShas) != 0 || q.Commit.Message != controlGitLabMarker(*x) {
			return remoteReceiptError("approved initial commit")
		}
		return remoteGitToolChain(r, *x, q)
	}
	var p contracts.TriggerCIParameters
	if contracts.DecodeVersion("TriggerCIParameters", x.ContractVersion, raw, &p) != nil || p.ProjectID != q.Project.ProjectID || p.CommitSha != q.Commit.CommitSha || q.CIConfigSHA256 == nil || p.CIConfigSHA256 != *q.CIConfigSHA256 || !chainEqual(predecessor.Commit, q.Commit) || !chainEqual(predecessor.CIConfigSHA256, q.CIConfigSHA256) {
		return remoteReceiptError("CI original push binding")
	}
	return remoteCIObservations(q, *x)
}

func remoteReservationBinds(v contracts.BudgetReservation, x contracts.ExecutionRequest) bool {
	return v.RunID == x.RunID && v.ContractVersion == x.ContractVersion && v.SourceVersion == x.SourceVersion && v.DefinitionSnapshotHash == x.DefinitionSnapshotHash && v.CommandID == x.CommandID && v.OperationID != nil && *v.OperationID == x.OperationID && v.ExecutionID != nil && *v.ExecutionID == x.ExecutionID && v.RequestHash != nil && *v.RequestHash == x.RequestHash
}

func remoteProjectReadPath(id int64) string { return "/projects/" + strconv.FormatInt(id, 10) }
func remoteProjectTarget(p contracts.RemoteProjectObservation, t contracts.GitLabTarget) bool {
	return p.NamespaceID == t.NamespaceID && p.NamespacePath == t.NamespacePath && p.ProjectPath == t.ProjectPath && p.PathWithNamespace == t.NamespacePath+"/"+t.ProjectPath && p.Visibility == t.Visibility && (p.DefaultBranch == nil || *p.DefaultBranch == "" || *p.DefaultBranch == t.Ref)
}
func remoteObserved(q contracts.RemoteExecutionReceipt, kind string, value any, paths ...string) bool {
	for _, read := range q.Observations {
		if read.Kind == kind && chainEqual(read.Payload, value) && chainContains(paths, read.Path) {
			return true
		}
	}
	return false
}

func remoteManifestFiles(m contracts.ArtifactManifest, p contracts.Proposal, q contracts.RemoteExecutionReceipt) error {
	if q.Commit == nil || len(q.Commit.Files) != len(m.Files) || q.CIConfigSHA256 == nil {
		return remoteReceiptError("complete remote file tree")
	}
	contents := map[string]string{}
	for _, change := range p.Changes {
		if change.After != nil {
			contents[change.Path] = *change.After
		}
	}
	seen := map[string]bool{}
	ciFound := false
	for _, file := range q.Commit.Files {
		body, ok := contents[file.Path]
		if seen[file.Path] || !ok {
			return remoteReceiptError("duplicate or unapproved remote file")
		}
		seen[file.Path] = true
		blob := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len([]byte(body)), body)))
		if file.Size != int64(len([]byte(body))) || file.SHA256 != contracts.BytesHash([]byte(body)) || file.GitBlobSha1 != hex.EncodeToString(blob[:]) {
			return remoteReceiptError("remote file bytes")
		}
		found := false
		for _, entry := range m.Files {
			if entry.Path == file.Path && entry.Size == file.Size && entry.SHA256 == file.SHA256 {
				found = true
			}
		}
		if !found || !remoteObserved(q, "file", file, remoteProjectReadPath(q.Project.ProjectID)+"/repository/files/"+url.PathEscape(file.Path)+"?ref="+url.QueryEscape(q.Commit.CommitSha)) {
			return remoteReceiptError("manifest and per-file observation")
		}
		if file.Path == ".gitlab-ci.yml" {
			ciFound = file.SHA256 == *q.CIConfigSHA256
		}
	}
	if !ciFound {
		return remoteReceiptError("exact CI configuration")
	}
	return nil
}

func remoteGitToolChain(r Run, x contracts.ExecutionRequest, q contracts.RemoteExecutionReceipt) error {
	order := []string{"git.init", "git.add", "git.commit", "git.head", "git.tree", "git.push"}
	if len(q.ToolReceiptRefs) != len(order) {
		return remoteReceiptError("six fixed Git commands")
	}
	last := q.StartedAt
	for i, ref := range q.ToolReceiptRefs {
		var tool *contracts.ToolExecutionReceipt
		for _, v := range r.Control.Audit.ToolReceipts {
			if chainEqual(ref, contracts.ObjectReference{ObjectType: "evidence", ObjectID: v.ReceiptID, ObjectVersion: 1, SourceVersion: v.SourceVersion, Hash: v.ReceiptHash}) {
				copy := v
				tool = &copy
			}
		}
		if tool == nil || contracts.Validate("ToolExecutionReceipt", *tool) != nil || !hashMatches("ToolExecutionReceipt", *tool, tool.ReceiptHash) || tool.CommandID != order[i] || tool.AttemptID != controlGitChild(x, q.AttemptID, order[i]) || tool.ContractVersion != q.ContractVersion || tool.RunID != q.RunID || tool.SourceVersion != q.SourceVersion || tool.DefinitionSnapshotHash != q.DefinitionSnapshotHash || tool.ExecutionID != q.ExecutionID || tool.OperationID != q.OperationID || tool.RequestHash != q.RequestHash || tool.ApprovalID != q.ApprovalID || tool.ApprovalHash != q.ApprovalHash || tool.ProposalSnapshotHash != q.ProposalSnapshotHash || tool.PlanHash != q.PlanHash || !chainEqual(tool.Workspace, q.Workspace) || !chainEqual(tool.InputManifestRef, q.ManifestRef) || !chainBefore(last, tool.StartedAt, false) || !chainBefore(tool.StartedAt, tool.CompletedAt, false) || !chainBefore(tool.CompletedAt, q.CompletedAt, false) || tool.OutputSHA256 != contracts.BytesHash([]byte(tool.Output)) {
			return remoteReceiptError("Git tool identity, provenance or sequence")
		}
		// Only the final push may have lost its outcome. The original successful
		// head/tree plus exact remote reads resolve it without changing history.
		if i < 5 || tool.Status == "succeeded" {
			if tool.Status != "succeeded" || tool.ExitCode == nil || *tool.ExitCode != 0 || !chainEqual(tool.OutputManifestRef, q.ManifestRef) {
				return remoteReceiptError("successful local Git prerequisite")
			}
		} else if !chainContains([]string{"failed", "interrupted", "unknown", "timed_out"}, tool.Status) {
			return remoteReceiptError("unresolved push tool outcome")
		}
		identity := false
		for _, v := range r.FrozenDefinition.ToolBindings {
			if v.ToolID == "git" && chainEqual(v, tool.Tool) {
				identity = true
			}
		}
		if !identity {
			return remoteReceiptError("frozen Git identity")
		}
		var policy *contracts.GitCommandPolicy
		for _, v := range r.FrozenDefinition.Definition.RepositoryPolicy.GitExecution.Commands {
			if v.CommandID == tool.CommandID {
				copy := v
				policy = &copy
			}
		}
		if policy == nil {
			return remoteReceiptError("frozen Git command")
		}
		argv := controlGitArguments(*policy, x, q.Target)
		if !chainEqual(tool.Arguments, argv) || tool.EnvironmentPolicy != "isolated_git" || tool.CwdScope != "run_workspace" {
			return remoteReceiptError("fixed Git argv and environment")
		}
		var reservation *contracts.BudgetReservation
		for _, v := range r.Control.Audit.Budget.Reservations {
			if v.AttemptID == tool.AttemptID {
				copy := v
				reservation = &copy
			}
		}
		if reservation == nil || !remoteReservationBinds(*reservation, x) || reservation.Kind != "tool" || reservation.ScopeID != "tool:"+tool.CommandID || reservation.DeadlineAt == nil || !chainBefore(reservation.ReservedAt, tool.StartedAt, false) || (i < 5 || tool.Status == "succeeded") && !chainBefore(tool.CompletedAt, *reservation.DeadlineAt, false) {
			return remoteReceiptError("Git tool budget")
		}
		settled := controlGitLabSettled(r, *reservation)
		outcome := tool.Status
		if outcome == "timed_out" {
			outcome = "failed"
		}
		if settled == nil || settled.Outcome != outcome || settled.ConsumedUnits != 1 || !chainBefore(tool.CompletedAt, settled.SettledAt, false) || !chainBefore(settled.SettledAt, q.CompletedAt, false) {
			return remoteReceiptError("Git tool settlement")
		}
		if tool.CommandID == "git.head" && (tool.OutputTruncated || strings.TrimSpace(tool.Output) != q.Commit.CommitSha) {
			return remoteReceiptError("original local Git SHA")
		}
		if tool.CommandID == "git.tree" {
			files := map[string]string{}
			for _, p := range r.Control.Audit.ProposalHistory {
				if p.SnapshotHash == q.ProposalSnapshotHash {
					for _, change := range p.Changes {
						if change.After != nil {
							files[change.Path] = *change.After
						}
					}
				}
			}
			if tool.OutputTruncated || controlGitTreeMatches(tool.Output, files) != nil {
				return remoteReceiptError("original full local Git tree")
			}
		}
		last = tool.CompletedAt
	}
	return nil
}

func remoteCIObservations(q contracts.RemoteExecutionReceipt, x contracts.ExecutionRequest) error {
	p, v := q.Pipeline, q.PipelineVariables
	if p == nil || v == nil || p.ProjectID != q.Project.ProjectID || p.Ref != q.Target.Ref || p.CommitSha != q.Commit.CommitSha || p.Status != "success" || p.Source != "api" || v.ProjectID != p.ProjectID || v.PipelineID != p.PipelineID || v.OperationID != x.OperationID || v.RequestHash != x.RequestHash {
		return remoteReceiptError("original explicit pipeline and variables")
	}
	base := remoteProjectReadPath(p.ProjectID) + "/pipelines/" + strconv.FormatInt(p.PipelineID, 10)
	if !remoteObserved(q, "pipeline", p, base) || !remoteObserved(q, "pipeline_variables", v, base+"/variables") || remoteCIJobFacts(q) != nil {
		return remoteReceiptError("complete pipeline observations")
	}
	names, ids := map[string]bool{}, map[int64]bool{}
	coverage, executable := false, false
	for _, job := range q.Jobs {
		if names[job.Name] || ids[job.JobID] || job.ProjectID != p.ProjectID || job.PipelineID != p.PipelineID || job.Ref != p.Ref || job.CommitSha != p.CommitSha || job.Status != "success" || job.AllowFailure {
			return remoteReceiptError("unique successful exact CI jobs")
		}
		names[job.Name], ids[job.JobID] = true, true
		paths := map[string]bool{}
		for _, artifact := range job.Artifacts {
			if paths[artifact.Path] || !artifact.Validated || artifact.ByteCount <= 0 || artifact.ArtifactRef.SourceVersion != q.SourceVersion || artifact.ArtifactRef.ObjectVersion != 1 || artifact.ArtifactRef.Hash != artifact.SHA256 || !remoteObserved(q, "artifact", artifact, remoteProjectReadPath(p.ProjectID)+"/jobs/"+strconv.FormatInt(job.JobID, 10)+"/artifacts/"+url.PathEscape(artifact.Path)) {
				return remoteReceiptError("actual CI artifact observation")
			}
			paths[artifact.Path] = true
			if job.Name == "test" && artifact.Path == "coverage.out" && artifact.Format == "go_coverage" {
				coverage = true
			}
			if job.Name == "build" && artifact.Path == "bin/server" && (artifact.Format == "elf_executable" || artifact.Format == "mach_o_executable") {
				executable = true
			}
		}
	}
	if !names["test"] || !names["build"] || !coverage || !executable {
		return remoteReceiptError("required test, coverage, build and executable")
	}
	return nil
}

func remoteCIJobFacts(q contracts.RemoteExecutionReceipt) error {
	if q.Pipeline == nil {
		return remoteReceiptError("jobs without observed pipeline")
	}
	p := q.Pipeline
	base := controlCIPipelinePath(p.ProjectID, p.PipelineID) + "/jobs?per_page=100&page="
	jobs := append([]contracts.CIJobEvidence{}, q.Jobs...)
	for i := range jobs {
		jobs[i].Artifacts = []contracts.RemoteArtifactEvidence{}
	}
	found := false
	for _, read := range q.Observations {
		if read.Kind == "jobs" && strings.HasPrefix(read.Path, base) && chainEqual(read.Payload, contracts.RemoteJobListObservation{ProjectID: p.ProjectID, PipelineID: p.PipelineID, Jobs: jobs}) {
			found = true
		}
	}
	if !found {
		return remoteReceiptError("unobserved complete job list")
	}
	for _, job := range q.Jobs {
		for _, artifact := range job.Artifacts {
			if !remoteObserved(q, "artifact", artifact, remoteProjectReadPath(p.ProjectID)+"/jobs/"+strconv.FormatInt(job.JobID, 10)+"/artifacts/"+url.PathEscape(artifact.Path)) {
				return remoteReceiptError("unobserved artifact")
			}
		}
	}
	return nil
}
