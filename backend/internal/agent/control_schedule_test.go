package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/governance"
)

// These are scheduling unit fixtures with real full governance chains, not
// claims that Go, GitLab or package execution occurred in this test.
type scheduleFixture struct {
	t     *testing.T
	s     *Service
	r     Run
	state CanonicalScheduleState
	clock time.Time
}

func newScheduleFixture(t *testing.T, mode string) *scheduleFixture {
	t.Helper()
	s := testService(t)
	s.Config.GitLabNamespace = "17"
	r := controlFixture(t, s, "schedule_"+mode)
	r.DeliveryMode = mode
	r.Control.Audit.Modes = runModes(r)
	f := &scheduleFixture{t: t, s: s, r: r, clock: time.Now().UTC(), state: CanonicalScheduleState{Workspace: contracts.WorkspaceReference{WorkspaceID: "workspace_schedule", RunID: r.ID, StorageKey: r.ID}, Results: []CanonicalActionResult{}}}
	command := Command{ID: "schedule_plan", RunID: r.ID, Content: "创建 schedule-project，Postgres GORM JWT JSON 日志"}
	message, err := appendCanonicalInput(&f.r, command, f.tick())
	if err != nil {
		t.Fatal(err)
	}
	p := ParsePlan(f.r.Plan, command.Content)
	p, err = s.normalizeFrozenPlan(f.r, p)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := projectPlanFor(f.r, p)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "gitlab" {
		body, _ := json.Marshal(contracts.GitLabTarget{Provider: "gitlab", InstanceURL: s.Config.GitLabURL, NamespaceID: "17", NamespacePath: "schedule-group", ProjectPath: plan.ProjectName, Ref: "main", Visibility: "private"})
		plan.DeliveryTarget = contracts.DeliveryTarget(body)
		plan.PlanHash = chainRehash(t, "ProjectPlan", plan)
	}
	ordered := []contracts.ContextMessage{{Role: "system", Content: f.r.FrozenDefinition.Definition.SystemPrompt.Content}, {Role: "user", Content: command.Content}}
	in, record, err := bindCanonicalDecision(f.r, command.ID, contracts.DecisionAdvice{Kind: "plan", Plan: &plan, Reason: "explicit deterministic test plan"}, contracts.ModelUsage{Status: "not_applicable"}, []contracts.MessageEnvelope{message}, ordered, f.tick())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := (governance.Validator{Definitions: s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "allowed" {
		t.Fatalf("plan: %v %+v", err, validation.Findings)
	}
	if err = appendCanonicalDecision(&f.r, in, record, validation); err != nil {
		t.Fatal(err)
	}
	f.r.Plan = p
	sourceChanged(&f.r)
	files, err := s.renderFrozenFiles(f.r)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.makeCanonicalProposal(&f.r, plan, files, f.tick()); err != nil {
		t.Fatal(err)
	}
	f.r.State = "PLAN"
	proposal := f.r.Control.Audit.Proposal
	accept := Command{ID: "schedule_approve", RunID: f.r.ID, Action: "accept", ProposalID: proposal.ProposalID, ProposalVersion: int(proposal.ProposalVersion), SourceVersion: f.r.SourceVersion, SnapshotHash: proposal.SnapshotHash}
	if err = s.acceptCanonical(&f.r, accept, f.tick()); err != nil {
		t.Fatal(err)
	}
	f.r.Control.Audit.Budget, err = governance.StartExecution(f.r.FrozenDefinition.Definition.BudgetPolicy, f.r.Control.Audit.Budget, f.tick())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *scheduleFixture) tick() time.Time { f.clock = f.clock.Add(time.Millisecond); return f.clock }
func (f *scheduleFixture) stamp() string   { return f.tick().Format(time.RFC3339Nano) }
func (f *scheduleFixture) next() ControlSchedule {
	f.t.Helper()
	out, err := NextControlAction(f.r, f.state)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *scheduleFixture) issue(out ControlSchedule) contracts.ExecutionRequest {
	f.t.Helper()
	if out.Action == nil {
		f.t.Fatalf("no writable action: %+v", out)
	}
	if out.Execution != nil {
		return *out.Execution
	}
	f.r.State = out.Stage
	f.r.StateSequence++
	command := fmt.Sprintf("schedule_action_%d", out.OperationSequence)
	ordered := f.r.Control.Audit.Contexts[0].Messages
	in, record, err := bindCanonicalDecision(f.r, command, contracts.DecisionAdvice{Kind: "action", Action: out.Action, Reason: "server scheduler candidate"}, contracts.ModelUsage{Status: "not_applicable"}, f.r.Control.Audit.Messages, ordered, f.tick())
	if err != nil {
		f.t.Fatal(err)
	}
	in.Facts.Workspace = &f.state.Workspace
	in.Facts.ExpectedAction = out.Action
	in.Facts.OperationSequence = out.OperationSequence
	deadline := f.clock.Add(time.Hour)
	in.Facts.Budget = governance.BudgetFacts{Available: true, RemainingAttempts: 8, ExecutionDeadline: &deadline, ToolDeadline: &deadline, CIDeadline: &deadline}
	for _, result := range f.state.Results {
		if result.ProjectID > 0 {
			in.Facts.RemoteProjectID = result.ProjectID
		}
		if result.CommitSHA != "" {
			in.Facts.CommitSHA = result.CommitSHA
			in.Facts.CIConfigSHA256 = result.CIConfigSHA256
		}
	}
	validation, err := (governance.Validator{Definitions: f.s.Definitions}).Validate(in)
	if err != nil || validation.Verdict != "allowed" {
		f.t.Fatalf("candidate rejected by actual governance: %v %+v", err, validation.Findings)
	}
	if err = appendCanonicalDecision(&f.r, in, record, validation); err != nil {
		f.t.Fatal(err)
	}
	return *validation.ExecutionRequest
}

func (f *scheduleFixture) operation(x contracts.ExecutionRequest, status string) int {
	f.t.Helper()
	kind, _, _ := scheduleAction(x.Action)
	target, err := governance.RequestTargetHash(x)
	if err != nil {
		f.t.Fatal(err)
	}
	sequence := int64(0)
	for _, in := range f.r.Control.ValidationInputs {
		if in.Facts.CommandID == x.CommandID {
			sequence = in.Facts.OperationSequence
		}
	}
	stamp := f.stamp()
	f.r.Control.Audit.Operations = append(f.r.Control.Audit.Operations, contracts.OperationRecord{OperationID: x.OperationID, RunID: x.RunID, SourceVersion: x.SourceVersion, ActionType: kind, OperationSequence: sequence, RequestHash: x.RequestHash, TargetHash: target, Status: status, EvidenceRefs: []contracts.ObjectReference{}, CreatedAt: stamp, UpdatedAt: stamp})
	return len(f.r.Control.Audit.Operations) - 1
}

func (f *scheduleFixture) reserve(x contracts.ExecutionRequest, attempt, kind, scope string, tool *contracts.PostCommand) contracts.BudgetReservation {
	f.t.Helper()
	ledger, reservation, err := governance.ReserveBudget(f.r.FrozenDefinition.Definition.BudgetPolicy, f.r.Control.Audit.Budget, governance.ReservationInput{SourceVersion: x.SourceVersion, CommandID: x.CommandID, AttemptID: attempt, Kind: kind, ScopeID: scope, Request: &x, Tool: tool, Now: f.tick()})
	if err != nil {
		f.t.Fatal(err)
	}
	f.r.Control.Audit.Budget = ledger
	return reservation
}
func (f *scheduleFixture) settle(reservation contracts.BudgetReservation) {
	f.t.Helper()
	elapsed := int64(0)
	ledger, _, err := governance.SettleBudget(f.r.FrozenDefinition.Definition.BudgetPolicy, f.r.Control.Audit.Budget, governance.SettlementInput{ReservationID: reservation.ReservationID, AttemptID: reservation.AttemptID, Outcome: "succeeded", MeasurementSource: "actual", ElapsedMS: &elapsed, Now: f.tick()})
	if err != nil {
		f.t.Fatal(err)
	}
	f.r.Control.Audit.Budget = ledger
}

func (f *scheduleFixture) finish(x contracts.ExecutionRequest) {
	f.t.Helper()
	kind, raw, _ := scheduleAction(x.Action)
	index := f.operation(x, "sent")
	attempt := "attempt_" + x.ExecutionID
	scope := kind
	if kind == "apply_skill" {
		var p contracts.ApplySkillParameters
		_ = json.Unmarshal(raw, &p)
		var sel struct {
			SkillID string `json:"skill_id"`
		}
		_ = json.Unmarshal(p.Selection, &sel)
		scope += ":" + sel.SkillID
	}
	reservation := f.reserve(x, attempt, "action", scope, nil)
	if kind == "scaffold" || kind == "apply_skill" {
		f.workspace(x, attempt, kind, raw, index)
	} else if kind == "verify" {
		f.tools(x, index)
	} else {
		f.result(x, attempt, kind, index)
	}
	f.settle(reservation)
	op := &f.r.Control.Audit.Operations[index]
	op.Status = "verified"
	op.UpdatedAt = f.stamp()
}

func (f *scheduleFixture) workspace(x contracts.ExecutionRequest, attempt, kind string, raw json.RawMessage, opIndex int) {
	selectedPaths := map[string]bool{}
	chosenPaths := map[string]bool{}
	chosenID := ""
	if kind == "apply_skill" {
		var p contracts.ApplySkillParameters
		_ = json.Unmarshal(raw, &p)
		var sel struct {
			SkillID string `json:"skill_id"`
		}
		_ = json.Unmarshal(p.Selection, &sel)
		chosenID = sel.SkillID
	}
	for _, selection := range f.r.Control.Audit.Plan.SelectedSkills {
		var sel struct {
			SkillID string `json:"skill_id"`
		}
		_ = json.Unmarshal(selection, &sel)
		for _, skill := range f.r.FrozenDefinition.Skills {
			if skill.SkillID == sel.SkillID {
				for _, path := range skill.WritePaths {
					selectedPaths[path] = true
					if sel.SkillID == chosenID {
						chosenPaths[path] = true
					}
				}
			}
		}
	}
	written := []string{}
	files := map[string]contracts.ArtifactEntry{}
	var input *contracts.WorkspaceReceiptInputManifestRef
	if previous := f.r.Control.Audit.Manifest; previous != nil {
		for _, file := range previous.Files {
			files[file.Path] = file
		}
		ref := contracts.WorkspaceReceiptInputManifestRef(scheduleManifestRef(*previous))
		input = &ref
	}
	for i, change := range f.r.Control.Audit.Proposal.Changes {
		if kind == "scaffold" && selectedPaths[change.Path] || kind == "apply_skill" && !chosenPaths[change.Path] {
			continue
		}
		written = append(written, change.Path)
		files[change.Path] = contracts.ArtifactEntry{ArtifactID: fmt.Sprintf("artifact_%d", i), Path: change.Path, Size: int64(len([]byte(*change.After))), SHA256: *change.AfterSHA256, MediaType: "text/plain"}
	}
	sort.Strings(written)
	p := f.r.Control.Audit.Proposal
	m := contracts.ArtifactManifest{ContractVersion: x.ContractVersion, ManifestID: "manifest_schedule", RunID: x.RunID, SourceVersion: x.SourceVersion, ProposalID: p.ProposalID, ProposalVersion: p.ProposalVersion, ProposalSnapshotHash: p.SnapshotHash, DefinitionSnapshotHash: x.DefinitionSnapshotHash, PlanHash: x.PlanHash, Files: []contracts.ArtifactEntry{}, CreatedAt: f.stamp(), ManifestVersion: int64(len(f.r.Control.Audit.ManifestHistory) + 1)}
	for _, file := range files {
		m.Files = append(m.Files, file)
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	m.Hash = chainRehash(f.t, "ArtifactManifest", m)
	f.r.Control.Audit.Manifest = &m
	f.r.Control.Audit.ManifestHistory = append(f.r.Control.Audit.ManifestHistory, m)
	output := contracts.WorkspaceReceiptOutputManifestRef(scheduleManifestRef(m))
	receipt := contracts.WorkspaceReceipt{ContractVersion: x.ContractVersion, ReceiptID: "workspace_" + x.ExecutionID, RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: attempt, RequestHash: x.RequestHash, ApprovalID: x.Approval.ApprovalID, ApprovalHash: f.r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: p.SnapshotHash, PlanHash: x.PlanHash, Workspace: x.Workspace, ActionType: kind, WrittenPaths: written, InputManifestRef: input, OutputManifestRef: &output, StartedAt: m.CreatedAt, CompletedAt: f.stamp(), Status: "succeeded"}
	receipt.ReceiptHash = chainRehash(f.t, "WorkspaceReceipt", receipt)
	f.r.Control.Audit.WorkspaceReceipts = append(f.r.Control.Audit.WorkspaceReceipts, receipt)
	f.r.Control.Audit.Operations[opIndex].EvidenceRefs = append(f.r.Control.Audit.Operations[opIndex].EvidenceRefs, contracts.ObjectReference{ObjectType: "workspace", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash})
}

func (f *scheduleFixture) tools(x contracts.ExecutionRequest, opIndex int) {
	commands, err := governance.MergePostCommands(*f.r.FrozenDefinition, *f.r.Control.Audit.Plan)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, command := range commands {
		identity, _ := contracts.HashValue([]string{x.RequestHash, "attempt_" + x.ExecutionID, command.CommandID})
		attempt := "tool_" + identity[:40]
		reservation := f.reserve(x, attempt, "tool", "tool:"+command.CommandID, &command)
		ref := scheduleManifestRef(*f.r.Control.Audit.Manifest)
		input := contracts.ToolExecutionReceiptInputManifestRef(ref)
		output := contracts.ToolExecutionReceiptOutputManifestRef(ref)
		zero := int64(0)
		var tool contracts.ToolIdentity
		for _, frozen := range f.r.FrozenDefinition.ToolBindings {
			if frozen.ToolID == command.Executable {
				tool = frozen
			}
		}
		if tool.ToolID == "" {
			f.t.Fatal("synthetic command has no exact frozen tool")
		}
		receipt := contracts.ToolExecutionReceipt{ContractVersion: x.ContractVersion, ReceiptID: "receipt_" + attempt, RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: attempt, RequestHash: x.RequestHash, ApprovalID: x.Approval.ApprovalID, ApprovalHash: f.r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, PlanHash: x.PlanHash, Workspace: x.Workspace, Tool: tool, CommandID: command.CommandID, Arguments: contracts.ToolExecutionReceiptArguments(command.Arguments), CwdScope: command.CwdScope, EnvironmentPolicy: command.EnvironmentPolicy, InputManifestRef: &input, OutputManifestRef: &output, StartedAt: f.stamp(), CompletedAt: f.stamp(), ExitCode: &zero, Status: "succeeded", Output: "synthetic scheduler fixture", OutputSHA256: contracts.BytesHash([]byte("synthetic scheduler fixture"))}
		receipt.ReceiptHash = chainRehash(f.t, "ToolExecutionReceipt", receipt)
		f.r.Control.Audit.ToolReceipts = append(f.r.Control.Audit.ToolReceipts, receipt)
		f.r.Control.Audit.Operations[opIndex].EvidenceRefs = append(f.r.Control.Audit.Operations[opIndex].EvidenceRefs, contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash})
		f.settle(reservation)
	}
}

func (f *scheduleFixture) result(x contracts.ExecutionRequest, attempt, kind string, index int) {
	ref := scheduleManifestRef(*f.r.Control.Audit.Manifest)
	op := &f.r.Control.Audit.Operations[index]
	result := CanonicalActionResult{RunID: x.RunID, SourceVersion: x.SourceVersion, DefinitionSnapshotHash: x.DefinitionSnapshotHash, PlanHash: x.PlanHash, ApprovalID: x.Approval.ApprovalID, ApprovalHash: f.r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: x.Approval.ProposalSnapshotHash, ExecutionID: x.ExecutionID, OperationID: x.OperationID, AttemptID: attempt, RequestHash: x.RequestHash, TargetHash: op.TargetHash, ActionType: kind, ManifestRef: &ref, CompletedAt: f.stamp()}
	external := ""
	if kind == "package" {
		result.PackageSHA256 = contracts.BytesHash([]byte("fixture package bytes"))
		result.PackageBytes = 21
		external = result.PackageSHA256
	} else {
		var target contracts.GitLabTarget
		_ = json.Unmarshal(f.r.Control.Audit.Plan.DeliveryTarget, &target)
		result.ObservedTarget = &target
		result.ProjectID = 191
		external = "191"
		if kind == "push_repository" || kind == "trigger_ci" {
			result.CommitSHA = strings.Repeat("a", 40)
			for _, file := range f.r.Control.Audit.Manifest.Files {
				if file.Path == ".gitlab-ci.yml" {
					result.CIConfigSHA256 = file.SHA256
				}
			}
			external = result.CommitSHA
		}
		if kind == "trigger_ci" {
			result.PipelineID = 291
			result.PipelineStatus = "success"
			external = strconv.FormatInt(result.PipelineID, 10)
		}
	}
	op.ExternalObjectID = &external
	var err error
	result.ResultHash, err = HashCanonicalActionResult(result)
	if err != nil {
		f.t.Fatal(err)
	}
	f.state.Results = append(f.state.Results, result)
}

func TestControlScheduleExactOrderAndLocalIsolation(t *testing.T) {
	for _, mode := range []string{"local", "gitlab"} {
		t.Run(mode, func(t *testing.T) {
			f := newScheduleFixture(t, mode)
			want := []string{"scaffold"}
			for range f.r.Control.Audit.Plan.SelectedSkills {
				want = append(want, "apply_skill")
			}
			want = append(want, "verify")
			if mode == "gitlab" {
				want = append(want, "create_repository", "push_repository", "trigger_ci")
			}
			want = append(want, "package")
			for i, kind := range want {
				before, _ := contracts.HashValue(f.r.Control)
				out := f.next()
				again := f.next()
				after, _ := contracts.HashValue(f.r.Control)
				if before != after || !chainEqual(out, again) {
					t.Fatal("scheduler mutated state or is nondeterministic")
				}
				actual, _, _ := scheduleAction(*out.Action)
				if actual != kind || out.Stage != scheduleStage(kind) || out.OperationSequence != int64(i+1) {
					t.Fatalf("next %+v, wanted %s", out, kind)
				}
				x := f.issue(out)
				replay := f.next()
				if replay.Execution == nil || replay.Execution.ExecutionID != x.ExecutionID || replay.Execution.RequestHash != x.RequestHash || replay.OperationSequence != out.OperationSequence {
					t.Fatal("existing request was replaced")
				}
				f.finish(x)
			}
			end := f.next()
			if !end.ActionsExhausted || end.Stage != "DELIVER" || end.Action != nil || f.r.State == "DONE" {
				t.Fatalf("invalid exhaustion semantics: %+v", end)
			}
		})
	}
}

func TestControlSchedulePartialSkillsAndMissingEvidence(t *testing.T) {
	f := newScheduleFixture(t, "local")
	f.finish(f.issue(f.next()))
	if len(f.r.Control.Audit.Plan.SelectedSkills) < 2 {
		t.Fatal("fixture needs multiple skills")
	}
	f.finish(f.issue(f.next()))
	out := f.next()
	var action contracts.CandidateActionVariant2
	if err := json.Unmarshal(*out.Action, &action); err != nil {
		t.Fatal(err)
	}
	if !chainEqual(action.Parameters.Selection, f.r.Control.Audit.Plan.SelectedSkills[1]) {
		t.Fatal("partial skills skipped or reordered")
	}
	x := f.issue(out)
	f.finish(x)
	f.r.Control.Audit.WorkspaceReceipts = f.r.Control.Audit.WorkspaceReceipts[:len(f.r.Control.Audit.WorkspaceReceipts)-1]
	blocked := f.next()
	if !blocked.AwaitingEvidence || blocked.Action != nil || blocked.Execution.ExecutionID != x.ExecutionID {
		t.Fatal("status label bypassed missing receipt")
	}
}

func TestControlScheduleUnknownNeverReissuesWrite(t *testing.T) {
	for _, status := range []string{"sent", "unknown"} {
		t.Run(status, func(t *testing.T) {
			f := newScheduleFixture(t, "local")
			x := f.issue(f.next())
			f.operation(x, status)
			for i := 0; i < 3; i++ {
				out := f.next()
				if !out.NeedsReconciliation || out.Action != nil || out.Operation.OperationID != x.OperationID || out.Execution.RequestHash != x.RequestHash {
					t.Fatal("unknown generated a new write")
				}
			}
			// A changed business source still cannot discard a possibly sent write.
			f.r.SourceVersion++
			out := f.next()
			if !out.NeedsReconciliation || out.Operation.SourceVersion != x.SourceVersion {
				t.Fatal("source reset outstanding write identity")
			}
		})
	}
}

func TestControlScheduleReadyReusesExactRequestAndReturnsIndependentCopy(t *testing.T) {
	f := newScheduleFixture(t, "local")
	x := f.issue(f.next())
	f.operation(x, "ready")
	out := f.next()
	if out.NeedsReconciliation || out.AwaitingEvidence || out.Execution == nil || out.Action == nil || out.Operation.OperationID != x.OperationID || out.Execution.RequestHash != x.RequestHash {
		t.Fatal("unstarted operation was not reused")
	}
	(*out.Action)[0] = 'x'
	out.Execution.Action[0] = 'x'
	if f.r.Control.Audit.Executions[0].Action[0] != '{' {
		t.Fatal("returned request aliases persisted history")
	}
	f.r.Control.Audit.Operations[0].Status = "rejected"
	out = f.next()
	if out.Action != nil || !out.AwaitingEvidence || out.Operation.OperationID != x.OperationID {
		t.Fatal("closed operation issued another write")
	}
}

func TestControlScheduleRejectsScopeAndHashTampering(t *testing.T) {
	f := newScheduleFixture(t, "local")
	f.finish(f.issue(f.next()))
	original := clone(f.r)
	cases := []struct {
		name   string
		mutate func(*Run)
	}{
		{"operation target", func(r *Run) { r.Control.Audit.Operations[0].TargetHash = strings.Repeat("0", 64) }},
		{"receipt hash", func(r *Run) { r.Control.Audit.WorkspaceReceipts[0].ReceiptHash = strings.Repeat("0", 64) }},
		{"receipt source", func(r *Run) {
			v := &r.Control.Audit.WorkspaceReceipts[0]
			v.SourceVersion++
			v.ReceiptHash = chainRehash(t, "WorkspaceReceipt", v)
		}},
		{"receipt request", func(r *Run) {
			v := &r.Control.Audit.WorkspaceReceipts[0]
			v.RequestHash = strings.Repeat("0", 64)
			v.ReceiptHash = chainRehash(t, "WorkspaceReceipt", v)
		}},
		{"receipt permission scope", func(r *Run) {
			v := &r.Control.Audit.WorkspaceReceipts[0]
			v.WrittenPaths = []string{"unauthorized.txt"}
			v.ReceiptHash = chainRehash(t, "WorkspaceReceipt", v)
		}},
		{"partial workspace success", func(r *Run) {
			v := &r.Control.Audit.WorkspaceReceipts[0]
			v.WrittenPaths = v.WrittenPaths[:1]
			v.ReceiptHash = chainRehash(t, "WorkspaceReceipt", v)
		}},
		{"current manifest absent", func(r *Run) { r.Control.Audit.Manifest = nil }},
		{"scope changed", func(r *Run) { r.Control.Audit.Approval.PermissionHash = strings.Repeat("0", 64) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := clone(original)
			tc.mutate(&r)
			if _, err := NextControlAction(r, f.state); err == nil {
				t.Fatal("tampering accepted")
			}
		})
	}
}

func TestControlScheduleVerificationNeedsEveryUnchangedToolReceipt(t *testing.T) {
	f := newScheduleFixture(t, "local")
	for {
		out := f.next()
		kind, _, _ := scheduleAction(*out.Action)
		x := f.issue(out)
		f.finish(x)
		if kind == "verify" {
			break
		}
	}
	if len(f.r.Control.Audit.ToolReceipts) < 3 {
		t.Fatal("expected fixed Go post commands")
	}
	original := clone(f.r)
	for _, test := range []string{"missing build", "candidate differs", "wrong argv", "wrong tool", "wrong manifest", "command order", "unbound attempt"} {
		t.Run(test, func(t *testing.T) {
			r := clone(original)
			receipt := &r.Control.Audit.ToolReceipts[len(r.Control.Audit.ToolReceipts)-1]
			switch test {
			case "missing build":
				r.Control.Audit.ToolReceipts = r.Control.Audit.ToolReceipts[:len(r.Control.Audit.ToolReceipts)-1]
			case "candidate differs":
				receipt.OutputManifestRef = nil
				receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
			case "wrong argv":
				receipt.Arguments = contracts.ToolExecutionReceiptArguments(`["test","./..."]`)
				receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
			case "wrong tool":
				receipt.Tool.Version = "different"
				receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
			case "wrong manifest":
				receipt.OutputManifestRef.Hash = strings.Repeat("0", 64)
				receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
			case "command order":
				receipt.StartedAt = r.Control.Audit.ToolReceipts[0].StartedAt
				receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
			case "unbound attempt":
				receipt.AttemptID = "unbound_attempt"
				receipt.ReceiptHash = chainRehash(t, "ToolExecutionReceipt", receipt)
			}
			out, err := NextControlAction(r, f.state)
			if test == "missing build" || test == "candidate differs" || test == "unbound attempt" {
				if err != nil || !out.AwaitingEvidence || out.Action != nil {
					t.Fatalf("incomplete verify advanced: %+v %v", out, err)
				}
			} else if err == nil {
				t.Fatal("tampered tool evidence accepted")
			}
		})
	}
}

func TestControlScheduleRemoteAndPackageNeedBoundActualResults(t *testing.T) {
	f := newScheduleFixture(t, "gitlab")
	for {
		out := f.next()
		kind, _, _ := scheduleAction(*out.Action)
		x := f.issue(out)
		f.finish(x)
		if kind == "trigger_ci" {
			break
		}
	}
	last := len(f.state.Results) - 1
	original := f.state.Results[last]
	f.state.Results[last].PipelineStatus = "running"
	f.state.Results[last].ResultHash, _ = HashCanonicalActionResult(f.state.Results[last])
	out := f.next()
	if !out.AwaitingEvidence || out.Action != nil {
		t.Fatal("pending CI advanced to package")
	}
	f.state.Results[last] = original
	f.state.Results[last].CommitSHA = strings.Repeat("b", 40)
	f.state.Results[last].ResultHash, _ = HashCanonicalActionResult(f.state.Results[last])
	if _, err := NextControlAction(f.r, f.state); err == nil {
		t.Fatal("CI for another commit accepted")
	}
	f.state.Results[last] = original
	packageRequest := f.issue(f.next())
	f.finish(packageRequest)
	f.state.Results = f.state.Results[:len(f.state.Results)-1]
	out = f.next()
	if out.ActionsExhausted || !out.AwaitingEvidence || out.Action != nil {
		t.Fatal("package label replaced package byte evidence")
	}
}
