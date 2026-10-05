// Package governance owns deterministic advice and execution authorization.
// It does not write files, advance Runs, reserve budgets, or execute tools.
package governance

import (
	"fmt"
	"time"

	"agentstudio/internal/contracts"
)

type DefinitionVerifier interface {
	VerifySnapshot(contracts.DefinitionSnapshot) error
	NormalizeParameters(contracts.DefinitionSnapshot, string, map[string]any) (map[string]any, error)
	ValidateSelection(contracts.DefinitionSnapshot, []string) error
}

type Validator struct{ Definitions DefinitionVerifier }

// ContextMessage is the exact ordered context hashing vocabulary. The caller
// supplies what the model/rules decider actually received, never a reconstruction.
type ContextMessage = contracts.ContextMessage

// BudgetFacts must come from the trusted, persisted reservation ledger. Available
// cannot be set by advice. Deadlines are absolute and survive resume/restart.
type BudgetFacts struct {
	Available               bool
	Reason                  string
	RemainingAttempts       int64
	ExecutionDeadline       *time.Time
	ToolDeadline            *time.Time
	CIDeadline              *time.Time
	ReconciliationDeadline  *time.Time
	ReconciliationAvailable bool
}

// Facts are internal trusted control-plane inputs, not a public wire DTO. Their
// complete public objects retain the published contracts without projection.
type Facts struct {
	RunID, State, ActorID, CommandID string
	SourceVersion, StateSequence     int64
	OperationSequence                int64
	Modes                            contracts.Modes
	FrozenDefinition                 contracts.DefinitionSnapshot
	Plan                             *contracts.ProjectPlan
	Proposal                         *contracts.Proposal
	Approval                         *contracts.Approval
	Permission                       *contracts.PermissionSnapshot
	Workspace                        *contracts.WorkspaceReference
	Manifest                         *contracts.ArtifactManifest
	RemoteProjectID                  int64
	CommitSHA, CIConfigSHA256        string
	Now                              time.Time
	Budget                           BudgetFacts
	Operation                        *contracts.OperationRecord
	ExpectedAction                   *contracts.CandidateAction
	ObservedModelUsage               *contracts.ModelUsage
	AvailableTools                   []contracts.ToolIdentity
	AvailableEvidence                []contracts.ObjectReference
	RevisionAllowed                  bool
}

type Input struct {
	Facts          Facts
	Decision       contracts.DecisionProposal
	InputMessages  []contracts.MessageEnvelope
	OrderedContext []ContextMessage
}

type Error struct{ Code, Detail string }

func (e *Error) Error() string { return e.Code + ": " + e.Detail }

// A caller may route this result to its bounded read-only reconciliation path.
// It never authorizes a write, even when the original approval remains valid.
type ReconciliationRequiredError struct{ OperationID string }

func (e *ReconciliationRequiredError) Error() string {
	return fmt.Sprintf("reconciliation_required: operation %s requires read-only reconciliation", e.OperationID)
}

func reject(code, detail string) *Error { return &Error{Code: code, Detail: detail} }

func MessageRef(message contracts.MessageEnvelope) (contracts.ObjectReference, error) {
	if err := contracts.Validate("MessageEnvelope", message); err != nil {
		return contracts.ObjectReference{}, reject("invalid_input_message", "input envelope violates its published schema")
	}
	hash, err := contracts.HashObject("MessageEnvelope", message)
	return contracts.ObjectReference{ObjectType: "message", ObjectID: message.MessageID, ObjectVersion: 1, SourceVersion: message.SourceVersion, Hash: hash}, err
}
