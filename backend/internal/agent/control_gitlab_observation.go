package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

// A durable dispatch boundary cannot prove that a remote server committed the
// write. It only makes a lost result ineligible for another write attempt.
func (s *Service) controlGitLabDispatch(id string, x contracts.ExecutionRequest) error {
	_, err := s.Store.Update(id, func(r *Run) error {
		_, op, err := s.controlGitLabRequest(*r, x)
		if err != nil {
			return err
		}
		if op.Status != "sent" || r.ActiveStep != x.ExecutionID || r.PauseRequested || r.CancelRequested || terminal(r.State) || r.State == "PAUSED" || r.State == "BLOCKED" || r.Control.RemoteWriteStarted[x.ExecutionID] {
			return fail("execution_stopped", "原远程请求已发送或不再允许进入发送边界")
		}
		if err = s.recheckControlApproval(*r, time.Now().UTC()); err != nil {
			return err
		}
		if r.Control.RemoteWriteStarted == nil {
			r.Control.RemoteWriteStarted = map[string]bool{}
		}
		r.Control.RemoteWriteStarted[x.ExecutionID] = true
		return nil
	})
	return err
}

type controlRemoteFact struct {
	kind  string
	value any
}

func (s *Service) controlGitLabObservation(reservation contracts.BudgetReservation, path string, output any, status int, readErr error, started, completed time.Time) (contracts.RemoteReadObservation, error) {
	observation := contracts.RemoteReadObservation{
		ReservationRef: contracts.RemoteReadObservationReservationRef{ObjectType: "budget_reservation", ObjectID: reservation.ReservationID, ObjectVersion: 1, SourceVersion: reservation.SourceVersion, Hash: reservation.ReservationHash},
		Method:         http.MethodGet, Path: path, StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano), Kind: "unavailable", Payload: []byte("null"),
	}
	if status != 0 {
		code := int64(status)
		observation.HTTPStatus = &code
	}
	if readErr != nil {
		return observation, nil
	}
	var public any
	kind := "project"
	switch value := output.(type) {
	case *controlGitLabProject:
		if value.EmptyRepo == nil {
			return observation, fail("remote_observation_invalid", "项目读取缺少完整公开事实")
		}
		public = contracts.RemoteProjectObservation{ProjectID: value.ID, NamespaceID: strconv.FormatInt(value.Namespace.ID, 10), NamespacePath: value.Namespace.FullPath, ProjectPath: value.Path, PathWithNamespace: value.PathWithNamespace, Visibility: value.Visibility, OwnershipMarker: value.Description, EmptyRepository: *value.EmptyRepo, DefaultBranch: value.DefaultBranch}
	case *controlRemoteFact:
		kind, public = value.kind, value.value
		if kind == "unavailable" {
			return observation, nil
		}
	default:
		// Namespace/tree pages have no published payload branch. Do not relabel
		// them as a project or infer complete file facts from a partial listing.
		return observation, nil
	}
	if status != http.StatusOK {
		return observation, fail("remote_observation_invalid", "完整事实读取必须返回HTTP 200")
	}
	payload, err := json.Marshal(public)
	if err != nil {
		return observation, err
	}
	for _, secret := range []string{s.Config.GitLabToken, s.Config.AIKey, controlGitAuthorization(s.Config.GitLabToken), strings.TrimPrefix(controlGitAuthorization(s.Config.GitLabToken), "Basic ")} {
		if secret != "" && bytes.Contains(payload, []byte(secret)) {
			return observation, fail("secret_output", "远程观察包含执行器凭据，拒绝持久化正文")
		}
	}
	hash, err := contracts.HashValue(public)
	if err != nil {
		return observation, err
	}
	complete := observation
	complete.Kind, complete.Payload, complete.FactsHash = kind, payload, &hash
	if err = contracts.ValidateVersion("RemoteReadObservation", "1.3.0", complete); err != nil {
		return observation, fail("remote_observation_invalid", "远程事实不符合完整公开Schema")
	}
	return complete, nil
}

func controlRemoteReceiptRef(q contracts.RemoteExecutionReceipt) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "evidence", ObjectID: q.ReceiptID, ObjectVersion: 1, SourceVersion: q.SourceVersion, Hash: q.ReceiptHash}
}

func verifyControlRemoteObservations(r Run) error {
	if len(r.Control.RemoteObservations) == 0 && len(r.Control.RemoteWriteStarted) == 0 {
		return nil
	}
	if !contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) {
		return remoteReceiptError("remote observation metadata on an older publication")
	}
	requests := map[string]contracts.ExecutionRequest{}
	for _, x := range r.Control.Audit.Executions {
		requests[x.ExecutionID] = x
	}
	seen := map[string]bool{}
	for id, reads := range r.Control.RemoteObservations {
		x, ok := requests[id]
		kind, _, err := scheduleAction(x.Action)
		if !ok || err != nil || !chainContains([]string{"create_repository", "push_repository", "trigger_ci"}, kind) {
			return remoteReceiptError("read without original remote request")
		}
		for _, read := range reads {
			if contracts.ValidateVersion("RemoteReadObservation", "1.3.0", read) != nil || seen[read.ReservationRef.ObjectID] {
				return remoteReceiptError("invalid or duplicate persisted read")
			}
			seen[read.ReservationRef.ObjectID] = true
			var reserved *contracts.BudgetReservation
			for _, value := range r.Control.Audit.Budget.Reservations {
				if value.ReservationID == read.ReservationRef.ObjectID {
					copy := value
					reserved = &copy
				}
			}
			if reserved == nil || !remoteReservationBinds(*reserved, x) || read.ReservationRef.ObjectVersion != 1 || read.ReservationRef.SourceVersion != x.SourceVersion || read.ReservationRef.Hash != reserved.ReservationHash || (reserved.Kind != "reconciliation_read" && !(kind == "trigger_ci" && reserved.Kind == "ci_poll")) || reserved.ScopeID != reserved.Kind+":"+x.OperationID || reserved.DeadlineAt == nil || !chainBefore(x.CreatedAt, reserved.ReservedAt, false) || !chainBefore(reserved.ReservedAt, read.StartedAt, false) || !chainBefore(read.StartedAt, read.CompletedAt, false) {
				return remoteReceiptError("persisted read budget or time")
			}
			settled := controlGitLabSettled(r, *reserved)
			if settled == nil || settled.ConsumedUnits != 1 || !chainBefore(read.CompletedAt, settled.SettledAt, false) {
				return remoteReceiptError("persisted read has no consumed settlement")
			}
			if read.Kind != "unavailable" {
				hash, err := contracts.HashValue(read.Payload)
				if err != nil || read.FactsHash == nil || hash != *read.FactsHash || settled.Outcome != "succeeded" || !chainBefore(read.CompletedAt, *reserved.DeadlineAt, false) {
					return remoteReceiptError("persisted read facts or deadline")
				}
			}
		}
	}
	for id, dispatched := range r.Control.RemoteWriteStarted {
		x, ok := requests[id]
		kind, _, err := scheduleAction(x.Action)
		if !ok || !dispatched || err != nil || !chainContains([]string{"create_repository", "push_repository", "trigger_ci"}, kind) {
			return remoteReceiptError("dispatch without original remote request")
		}
		if _, err = controlGitLabAttempt(r, x); err != nil {
			return err
		}
	}
	return nil
}

// The public receipt is made solely from the original complete request and
// actual persisted observations. Reconciliation appends; it never rewrites an
// unknown receipt or its original unknown budget charge.
func appendControlRemoteReceipt(r *Run, x contracts.ExecutionRequest, status string, completed time.Time) error {
	kind, raw, err := scheduleAction(x.Action)
	var parameters struct {
		Target contracts.GitLabTarget `json:"target"`
	}
	if err != nil || json.Unmarshal(raw, &parameters) != nil {
		return remoteReceiptError("original remote action")
	}
	approval, manifest := controlGitLabProvenance(*r, x)
	attempt, err := controlGitLabAttempt(*r, x)
	if err != nil || approval == nil || manifest == nil {
		return remoteReceiptError("create provenance")
	}
	q := contracts.RemoteExecutionReceipt{
		ContractVersion: x.ContractVersion, RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash,
		ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: attempt.AttemptID, RequestHash: x.RequestHash,
		ApprovalID: approval.ApprovalID, ApprovalHash: approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, PlanHash: x.PlanHash, Workspace: x.Workspace,
		ReceiptID: newID("remote_receipt_"), ActionType: kind, Target: parameters.Target, ManifestRef: contracts.RemoteExecutionReceiptManifestRef(*manifest),
		Jobs: []contracts.CIJobEvidence{}, ToolReceiptRefs: []contracts.RemoteExecutionReceiptToolReceiptRefsItem{},
		Observations: append([]contracts.RemoteReadObservation{}, r.Control.RemoteObservations[x.ExecutionID]...),
		WriteSent:    r.Control.RemoteWriteStarted[x.ExecutionID], StartedAt: attempt.ReservedAt, CompletedAt: completed.Format(time.RFC3339Nano), Status: status,
	}
	for _, read := range q.Observations {
		if read.Kind == "project" {
			var project contracts.RemoteProjectObservation
			if err = contracts.DecodeVersion("RemoteProjectObservation", "1.3.0", read.Payload, &project); err != nil {
				return err
			}
			q.Project = &project
		}
		if read.Kind == "commit" {
			var commit contracts.RemoteCommitObservation
			if err = contracts.DecodeVersion("RemoteCommitObservation", "1.3.0", read.Payload, &commit); err != nil {
				return err
			}
			q.Commit = &commit
			for _, file := range commit.Files {
				if file.Path == ".gitlab-ci.yml" {
					value := file.SHA256
					q.CIConfigSHA256 = &value
				}
			}
		}
		if read.Kind == "pipeline" {
			var pipeline contracts.RemotePipelineObservation
			if err = contracts.Decode("RemotePipelineObservation", read.Payload, &pipeline); err != nil {
				return err
			}
			q.Pipeline = &pipeline
		}
		if read.Kind == "pipeline_variables" {
			var variables contracts.RemotePipelineVariables
			if err = contracts.Decode("RemotePipelineVariables", read.Payload, &variables); err != nil {
				return err
			}
			q.PipelineVariables = &variables
		}
		if read.Kind == "jobs" {
			var jobs contracts.RemoteJobListObservation
			if err = contracts.Decode("RemoteJobListObservation", read.Payload, &jobs); err != nil {
				return err
			}
			q.Jobs = jobs.Jobs
		}
	}
	if q.Pipeline != nil {
		for i := range q.Jobs {
			job := &q.Jobs[i]
			for _, read := range q.Observations {
				path := remoteProjectReadPath(q.Pipeline.ProjectID) + "/jobs/" + strconv.FormatInt(job.JobID, 10) + "/artifacts/"
				if read.Kind == "artifact" && strings.HasPrefix(read.Path, path) {
					var artifact contracts.RemoteArtifactEvidence
					if err = contracts.Decode("RemoteArtifactEvidence", read.Payload, &artifact); err != nil {
						return err
					}
					if read.Path != path+url.PathEscape(artifact.Path) {
						return remoteReceiptError("artifact observation path")
					}
					matched := false
					for j, old := range job.Artifacts {
						if old.Path == artifact.Path {
							job.Artifacts[j] = artifact
							matched = true
						}
					}
					if !matched {
						job.Artifacts = append(job.Artifacts, artifact)
					}
				}
			}
		}
	}
	if kind == "push_repository" {
		for _, command := range r.FrozenDefinition.Definition.RepositoryPolicy.GitExecution.Commands {
			for _, tool := range r.Control.Audit.ToolReceipts {
				if tool.ExecutionID == x.ExecutionID && tool.AttemptID == controlGitChild(x, attempt.AttemptID, command.CommandID) {
					q.ToolReceiptRefs = append(q.ToolReceiptRefs, contracts.RemoteExecutionReceiptToolReceiptRefsItem{ObjectType: "evidence", ObjectID: tool.ReceiptID, ObjectVersion: 1, SourceVersion: tool.SourceVersion, Hash: tool.ReceiptHash})
				}
			}
		}
	}
	for _, previous := range r.Control.Audit.RemoteReceipts {
		if previous.ExecutionID != x.ExecutionID {
			continue
		}
		if previous.Status == "succeeded" {
			return remoteReceiptError("successful operation cannot publish a second result")
		}
		if status == "unknown" && previous.Status == "unknown" && previous.WriteSent == q.WriteSent && chainEqual(previous.Observations, q.Observations) && chainEqual(previous.ToolReceiptRefs, q.ToolReceiptRefs) && chainEqual(previous.Commit, q.Commit) && chainEqual(previous.Project, q.Project) && chainEqual(previous.Pipeline, q.Pipeline) && chainEqual(previous.PipelineVariables, q.PipelineVariables) && chainEqual(previous.Jobs, q.Jobs) {
			return nil
		}
	}
	q.ReceiptHash, err = contracts.HashObject("RemoteExecutionReceipt", q)
	if err != nil {
		return err
	}
	if err = ValidateControlRemoteReceipt(*r, q); err != nil {
		return err
	}
	r.Control.Audit.RemoteReceipts = append(r.Control.Audit.RemoteReceipts, q)
	for i := range r.Control.Audit.Operations {
		if r.Control.Audit.Operations[i].OperationID == x.OperationID {
			r.Control.Audit.Operations[i].EvidenceRefs = append(r.Control.Audit.Operations[i].EvidenceRefs, controlRemoteReceiptRef(q))
		}
	}
	return nil
}

// A legacy/private scheduler projection is never sufficient for 1.3. Require
// one complete public successful receipt with exactly the projected facts.
func controlRemoteResultReceipt(r Run, x contracts.ExecutionRequest, result CanonicalActionResult) error {
	count := 0
	for _, q := range r.Control.Audit.RemoteReceipts {
		if q.ExecutionID != x.ExecutionID || q.Status != "succeeded" {
			continue
		}
		count++
		if err := ValidateControlRemoteReceipt(r, q); err != nil {
			return err
		}
		if q.OperationID != result.OperationID || q.AttemptID != result.AttemptID || q.RequestHash != result.RequestHash || q.ActionType != result.ActionType || q.CompletedAt != result.CompletedAt || q.Project == nil || q.Project.ProjectID != result.ProjectID || !chainEqual(q.ManifestRef, result.ManifestRef) || !chainEqual(q.Target, result.ObservedTarget) {
			return remoteReceiptError("private result differs from complete public receipt")
		}
		commit, ciHash, pipeline, pipelineStatus := "", "", int64(0), ""
		if q.Commit != nil {
			commit = q.Commit.CommitSha
		}
		if q.CIConfigSHA256 != nil {
			ciHash = *q.CIConfigSHA256
		}
		if q.Pipeline != nil {
			pipeline, pipelineStatus = q.Pipeline.PipelineID, q.Pipeline.Status
		}
		if result.CommitSHA != commit || result.CIConfigSHA256 != ciHash || result.PipelineID != pipeline || result.PipelineStatus != pipelineStatus || result.PackageBytes != 0 || result.PackageSHA256 != "" {
			return remoteReceiptError("private remote result claims unobserved delivery")
		}
	}
	if count != 1 {
		return remoteReceiptError("one complete successful public receipt required")
	}
	return nil
}
